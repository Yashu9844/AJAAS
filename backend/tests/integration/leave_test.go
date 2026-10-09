package integration

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func (tc *timeCtx) leaveType(t *testing.T, code string, allowance float64) string {
	t.Helper()
	r := tc.do(t, "POST", "/leave/types", body(map[string]any{"name": code + " " + uniq(), "code": code,
		"annual_allowance": allowance, "carry_forward_limit": 5}))
	expect(t, r, 201, "create leave type "+code)
	return r.str("data.id")
}

func (tc *timeCtx) apply(t *testing.T, tok, typ string, from, to time.Time) resp {
	t.Helper()
	return tc.as(t, tok, "POST", "/leave/requests", body(map[string]string{
		"leave_type_id": typ, "start_date": ymd(from), "end_date": ymd(to), "reason": "family wedding"}))
}

func (tc *timeCtx) balance(t *testing.T, tok, typ string) map[string]any {
	t.Helper()
	r := tc.as(t, tok, "GET", "/leave/balances/me")
	for _, it := range r.list("data") {
		if m := it.(map[string]any); m["leave_type_id"] == typ {
			return m
		}
	}
	t.Fatalf("no balance for %s: %s", typ, r.Raw)
	return nil
}

// TestLeaveLifecycle — G5 reserve, G8 approve → Module 3 on_leave, G13 cancel future, G12 ledger invariant, G11 privacy.
func TestLeaveLifecycle(t *testing.T) {
	tc := newTimeTenant(t)
	el := tc.leaveType(t, "EL", 12)
	mon, tue := monday(0), monday(0).AddDate(0, 0, 1)
	p := tc.as(t, tc.memberTok, "POST", "/leave/requests/preview", body(map[string]string{"leave_type_id": el, "start_date": ymd(mon), "end_date": ymd(tue), "reason": "x"}))
	if num(p, "data.total_days") != 2 || num(p, "data.available_after") != 10 {
		t.Fatalf("preview: %s", p.Raw)
	}
	a := tc.apply(t, tc.memberTok, el, mon, tue)
	expect(t, a, 201, "G5 apply")
	if b := tc.balance(t, tc.memberTok, el); b["reserved"] != 2.0 || b["available"] != 10.0 {
		t.Fatalf("G5 balance: %v", b)
	}
	expect(t, tc.do(t, "POST", "/leave/requests/"+a.str("data.id")+"/approve", body(map[string]string{"comment": "enjoy"})), 200, "G8 approve")
	if n := sqlStr(t, "SELECT count(*) FROM attendance_records WHERE employee_profile_id = ? AND status = 'on_leave' AND source = 'leave' AND attendance_date IN (?::date, ?::date)", tc.memberEmp, ymd(mon), ymd(tue)); n != "2" {
		t.Fatalf("G8: Module 3 on_leave records = %s", n)
	}
	expect(t, tc.as(t, tc.memberTok, "POST", "/leave/requests/"+a.str("data.id")+"/cancel"), 200, "G13 cancel approved future")
	if n := sqlStr(t, "SELECT count(*) FROM attendance_records WHERE employee_profile_id = ?", tc.memberEmp); n != "0" {
		t.Fatalf("G13: leave-only records must be removed, %s left", n)
	}
	adj := tc.do(t, "POST", "/leave/balances/adjust", body(map[string]any{"employee_id": tc.memberEmp, "leave_type_id": el, "days": -1.5, "reason": "medical certificate correction"}))
	if expect(t, adj, 200, "adjust"); num(adj, "data.available") != 10.5 {
		t.Fatalf("adjust: %s", adj.Raw)
	}
	ok := sqlStr(t, `SELECT bool_and(ok) FROM (SELECT b.opening = COALESCE(SUM(l.days) FILTER (WHERE l.kind='carry_forward'),0)
		AND b.accrued = COALESCE(SUM(l.days) FILTER (WHERE l.kind='accrual'),0)
		AND b.adjusted = COALESCE(SUM(l.days) FILTER (WHERE l.kind='adjustment'),0)
		AND b.used = COALESCE(SUM(l.days) FILTER (WHERE l.kind IN ('consume','reversal')),0)
		AND b.reserved = COALESCE(SUM(l.days) FILTER (WHERE l.kind IN ('reserve','release')),0) AS ok
		FROM leave_balances b LEFT JOIN leave_ledger l ON l.tenant_id=b.tenant_id AND l.employee_profile_id=b.employee_profile_id
		AND l.leave_type_id=b.leave_type_id AND l.year=b.year WHERE b.employee_profile_id = ? GROUP BY b.id) s`, tc.memberEmp)
	if ok != "true" {
		t.Fatalf("G12 ledger invariant: %s", ok)
	}
	leak := "LIKE '%wedding%' OR %s::text LIKE '%medical%' OR %s::text LIKE '%enjoy%'"
	for table, col := range map[string]string{"audit_logs": "metadata", "leave_events_outbox": "payload"} {
		q := "SELECT count(*) FROM " + table + " WHERE tenant_id = ? AND (" + col + "::text " + strings.ReplaceAll(leak, "%s", col) + ")"
		if n := sqlStr(t, q, tc.id); n != "0" {
			t.Fatalf("G11: %s leaks free text (%s rows)", table, n)
		}
	}
}

// TestLeaveOverlapAndPagination — G7 + G15.
func TestLeaveOverlapAndPagination(t *testing.T) {
	tc := newTimeTenant(t)
	el := tc.leaveType(t, "EL", 20)
	mon := monday(1)
	expect(t, tc.apply(t, tc.memberTok, el, mon, mon.AddDate(0, 0, 2)), 201, "Mon–Wed")
	expectErr(t, tc.apply(t, tc.memberTok, el, mon.AddDate(0, 0, 1), mon.AddDate(0, 0, 1)), 409, "LEAVE_OVERLAP", "G7")
	thu := ymd(mon.AddDate(0, 0, 3))
	for _, half := range []string{"first_half", "second_half"} {
		expect(t, tc.as(t, tc.memberTok, "POST", "/leave/requests", body(map[string]string{"leave_type_id": el, "start_date": thu, "end_date": thu, "half_day": half, "reason": "x"})), 201, "G7 "+half)
	}
	for _, q := range [][]string{{"per_page", "0"}, {"page", "-1", "per_page", "abc"}, {"per_page", "100000"}} {
		r := tc.do(t, "GET", "/leave/requests", query(q...))
		if expect(t, r, 200, "G15"); num(r, "meta.per_page") < 1 || num(r, "meta.per_page") > 100 {
			t.Fatalf("G15 %v: %s", q, r.Raw)
		}
	}
}

// TestLeaveRBACAndSelfApproval — G9 (exact grants) + G10.
func TestLeaveRBACAndSelfApproval(t *testing.T) {
	tc := newTimeTenant(t)
	el := tc.leaveType(t, "EL", 10)
	for _, p := range [][2]string{{"POST", "/leave/types"}, {"GET", "/leave/requests"}, {"GET", "/leave/ledger"}, {"POST", "/leave/balances/adjust"}} {
		expectErr(t, tc.as(t, tc.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "member "+p[0]+" "+p[1])
	}
	for _, path := range []string{"/leave/types", "/leave/holidays", "/leave/balances/me", "/leave/requests/me"} {
		expect(t, tc.as(t, tc.memberTok, "GET", path), 200, "member "+path)
	}
	m := tc.apply(t, tc.memberTok, el, monday(2), monday(2))
	expect(t, m, 201, "member apply")
	expectErr(t, tc.as(t, tc.memberTok, "POST", "/leave/requests/"+m.str("data.id")+"/approve"), 403, "FORBIDDEN", "member approve")
	approver, _, _ := tc.limitedUser(t, [2]string{"leave", "approve"})
	expect(t, tc.as(t, approver, "POST", "/leave/requests/"+m.str("data.id")+"/approve"), 200, "leave:approve approves")
	own := tc.apply(t, tc.token, el, monday(3), monday(3))
	expect(t, own, 201, "admin apply")
	expectErr(t, tc.do(t, "POST", "/leave/requests/"+own.str("data.id")+"/approve"), 403, "SELF_APPROVAL_FORBIDDEN", "G10")
}

// TestLeaveIsolation — G1.
func TestLeaveIsolation(t *testing.T) {
	a, b := newTimeTenant(t), newTimeTenant(t)
	typ := a.leaveType(t, "EL", 10)
	r := a.apply(t, a.memberTok, typ, monday(3), monday(3))
	expect(t, r, 201, "A apply")
	h := a.do(t, "POST", "/leave/holidays", body(map[string]string{"date": ymd(monday(4)), "name": "A day"}))
	expect(t, h, 201, "A holiday")
	for _, p := range [][2]string{{"GET", "/leave/types/" + typ}, {"GET", "/leave/requests/" + r.str("data.id")},
		{"POST", "/leave/requests/" + r.str("data.id") + "/approve"}, {"DELETE", "/leave/holidays/" + h.str("data.id")}} {
		expectErr(t, b.do(t, p[0], p[1]), 404, "", "G1 B "+p[0]+" "+p[1])
	}
	expectErr(t, b.apply(t, b.memberTok, typ, monday(3), monday(3)), 404, "NOT_FOUND", "G1 foreign type")
	expectErr(t, b.do(t, "GET", "/leave/balances", query("employee_id", a.memberEmp)), 404, "EMPLOYEE_NOT_FOUND", "G1 foreign employee")
	expectErr(t, call(t, "GET", "/leave/requests", host(b.slug), token(a.token)), 403, "", "G1 A token on B host")
}

// TestLeaveConcurrency — G14: parallel applies cannot overdraw (LV-014).
func TestLeaveConcurrency(t *testing.T) {
	tc := newTimeTenant(t)
	typ := tc.leaveType(t, "CL", 5)
	codes := make([]int, 5)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mon := monday(i + 1)
			codes[i] = tc.apply(t, tc.memberTok, typ, mon, mon.AddDate(0, 0, 1)).Status
		}(i)
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		if c == 201 {
			created++
		}
	}
	if b := tc.balance(t, tc.memberTok, typ); created != 2 || b["available"] != 1.0 {
		t.Fatalf("G14: codes %v balance %v", codes, b)
	}
}
