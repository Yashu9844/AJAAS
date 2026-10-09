package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// prevMonth is the payroll period under test: the month before today (never in the future, FR-RN001).
func prevMonth() (year, month, days int) {
	first := time.Now().UTC().AddDate(0, -1, 0)
	y, m := first.Year(), int(first.Month())
	return y, m, time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// lopWindow finds a Mon–Tue inside the previous month, within Module 4's 30-day backdating window and the current
// leave year; ok=false when the calendar makes that impossible (e.g. early January).
func lopWindow() (mon time.Time, ok bool) {
	y, m, days := prevMonth()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for d := 1; d < days; d++ {
		c := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		if c.Weekday() == time.Monday && !c.Before(today.AddDate(0, 0, -29)) && c.Year() == today.Year() {
			return c, true
		}
	}
	return time.Time{}, false
}

func (tc *timeCtx) payrollStructure(t *testing.T) string {
	t.Helper()
	r := tc.do(t, "POST", "/payroll/structures", body(map[string]any{"name": "Standard " + uniq(), "components": []map[string]any{
		{"code": "BASIC", "name": "Basic", "kind": "earning", "calc": "percent_of_ctc", "value": 40},
		{"code": "HRA", "name": "HRA", "kind": "earning", "calc": "percent_of_basic", "value": 50},
		{"code": "SPECIAL", "name": "Special", "kind": "earning", "calc": "balance"},
	}}))
	expect(t, r, 201, "create structure")
	return r.str("data.id")
}

// TestPayrollLifecycle — G10 (real Module 4 LOP), G11 maker-checker, G13 self visibility, G14 privacy, CSV.
func TestPayrollLifecycle(t *testing.T) {
	tc := newTimeTenant(t)
	sid := tc.payrollStructure(t)
	for _, emp := range []string{tc.adminEmp, tc.memberEmp} {
		expect(t, tc.do(t, "POST", "/payroll/assignments", body(map[string]any{"employee_id": emp, "structure_id": sid,
			"annual_ctc": 1200000, "effective_from": "2026-01-15"})), 201, "assign")
	}
	lopDays := 0.0
	if mon, ok := lopWindow(); ok {
		typ := tc.leaveType(t, "LOP", 0)
		expect(t, tc.do(t, "PATCH", "/leave/types/"+typ, body(map[string]any{"is_paid": false})), 200, "make LOP unpaid")
		req := tc.apply(t, tc.memberTok, typ, mon, mon.AddDate(0, 0, 1))
		expect(t, req, 201, "member unpaid leave")
		expect(t, tc.do(t, "POST", "/leave/requests/"+req.str("data.id")+"/approve"), 200, "approve unpaid leave")
		lopDays = 2
	}
	y, m, days := prevMonth()
	run := tc.do(t, "POST", "/payroll/runs", body(map[string]int{"month": m, "year": y}))
	expect(t, run, 201, "create run")
	rid := run.str("data.id")
	expectErr(t, tc.do(t, "POST", "/payroll/runs", body(map[string]int{"month": m, "year": y})), 409, "RUN_EXISTS", "duplicate period")
	calc := tc.do(t, "POST", "/payroll/runs/"+rid+"/calculate")
	if expect(t, calc, 200, "calculate"); num(calc, "data.employee_count") != 2 {
		t.Fatalf("both employees paid: %s", calc.Raw)
	}
	if mine := tc.as(t, tc.memberTok, "GET", "/payroll/payslips/me"); len(mine.list("data")) != 0 {
		t.Fatalf("G13 nothing visible before finalize: %s", mine.Raw)
	}
	expectErr(t, tc.do(t, "POST", "/payroll/runs/"+rid+"/approve"), 403, "SELF_APPROVAL_FORBIDDEN", "G11 maker-checker")
	checker, _, _ := tc.limitedUser(t, [2]string{"payroll", "approve"})
	expectErr(t, tc.as(t, checker, "POST", "/payroll/runs/"+rid+"/finalize"), 409, "RUN_STATE", "G12 finalize before approve")
	expect(t, tc.as(t, checker, "POST", "/payroll/runs/"+rid+"/approve"), 200, "checker approves")
	expect(t, tc.as(t, checker, "POST", "/payroll/runs/"+rid+"/finalize"), 200, "finalize")

	mine := tc.as(t, tc.memberTok, "GET", "/payroll/payslips/me")
	slips := mine.list("data")
	if len(slips) != 1 {
		t.Fatalf("G13 one payslip after finalize: %s", mine.Raw)
	}
	slip := slips[0].(map[string]any)
	if slip["lop_days"].(float64) != lopDays || slip["payable_days"].(float64) != float64(days)-lopDays {
		t.Fatalf("G10 LOP from Module 4: lop %v payable %v (days %d)", slip["lop_days"], slip["payable_days"], days)
	}
	wantGross := 100000.0 * (float64(days) - lopDays) / float64(days)
	if g := slip["gross"].(float64); g < wantGross-0.02 || g > wantGross+0.02 {
		t.Fatalf("gross %v, want ≈ %.2f", g, wantGross)
	}
	adminSlip := ""
	for _, it := range tc.do(t, "GET", "/payroll/runs/"+rid+"/payslips").list("data") {
		if s := it.(map[string]any); s["employee_id"] == tc.adminEmp {
			adminSlip = s["id"].(string)
		}
	}
	expectErr(t, tc.as(t, tc.memberTok, "GET", "/payroll/payslips/me/"+adminSlip), 404, "NOT_FOUND", "G13 someone else's payslip")
	csv := tc.do(t, "GET", "/payroll/runs/"+rid+"/payout.csv")
	if expect(t, csv, 200, "payout csv"); !strings.HasPrefix(csv.Raw, "employee_code,employee_name,net_pay") || strings.Count(csv.Raw, "\n") != 3 {
		t.Fatalf("csv: %q", csv.Raw)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'payroll.%' AND (metadata::text LIKE '%1200000%' OR metadata::text LIKE '%ctc%' OR metadata::text LIKE '%net%')", tc.id); n != "0" {
		t.Fatalf("G14 audit leaks salary (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM payroll_events_outbox WHERE tenant_id = ? AND (payload::text LIKE ? OR payload::text LIKE ?)", tc.id, "%"+tc.memberEmp+"%", "%"+tc.adminEmp+"%"); n != "0" {
		t.Fatalf("G14 events name employees (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(DISTINCT routing_key) FROM payroll_events_outbox WHERE tenant_id = ?", tc.id); n != "4" {
		t.Fatalf("4 run events expected, got %s", n)
	}
}

// TestPayrollRBACAndIsolation — G1 + exact-grant RBAC + G16.
func TestPayrollRBACAndIsolation(t *testing.T) {
	a, b := newTimeTenant(t), newTimeTenant(t)
	sid := a.payrollStructure(t)
	y, m, _ := prevMonth()
	run := a.do(t, "POST", "/payroll/runs", body(map[string]int{"month": m, "year": y}))
	expect(t, run, 201, "A run")
	for _, p := range [][2]string{{"GET", "/payroll/structures"}, {"POST", "/payroll/runs"}, {"GET", "/payroll/runs"}, {"POST", "/payroll/assignments/preview"}} {
		expectErr(t, a.as(t, a.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "member "+p[0]+" "+p[1])
	}
	expectErr(t, a.as(t, a.memberTok, "GET", "/payroll/assignments/me"), 404, "NOT_FOUND", "member without assignment")
	reader, _, _ := a.limitedUser(t, [2]string{"payroll", "read"})
	expect(t, a.as(t, reader, "GET", "/payroll/runs/"+run.str("data.id")), 200, "payroll:read reads")
	expectErr(t, a.as(t, reader, "POST", "/payroll/runs/"+run.str("data.id")+"/calculate"), 403, "FORBIDDEN", "read cannot calculate")
	for _, p := range []string{"/payroll/structures/" + sid, "/payroll/runs/" + run.str("data.id")} {
		expectErr(t, b.do(t, "GET", p), 404, "NOT_FOUND", "G1 tenant B "+p)
	}
	expectErr(t, b.do(t, "POST", "/payroll/assignments", body(map[string]any{"employee_id": b.memberEmp, "structure_id": sid,
		"annual_ctc": 600000, "effective_from": "2026-02-01"})), 404, "NOT_FOUND", "G1 foreign structure")
	expectErr(t, call(t, "GET", "/payroll/runs", host(b.slug), token(a.token)), 403, "", "G1 A token on B host")
	for _, q := range [][]string{{"per_page", "0"}, {"page", "-1", "per_page", "abc"}, {"per_page", "100000"}} {
		r := a.do(t, "GET", "/payroll/runs", query(q...))
		if expect(t, r, 200, fmt.Sprint("G16 ", q)); num(r, "meta.per_page") < 1 || num(r, "meta.per_page") > 100 {
			t.Fatalf("G16 %v: %s", q, r.Raw)
		}
	}
	prev := a.do(t, "POST", "/payroll/assignments/preview", body(map[string]any{"structure_id": sid, "annual_ctc": 240000}))
	if expect(t, prev, 200, "preview"); num(prev, "data.gross") != 20000 {
		t.Fatalf("preview: %s", prev.Raw)
	}
}
