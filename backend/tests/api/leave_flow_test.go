//go:build integration

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// monday returns the Monday k weeks after the next Monday (UTC), as YYYY-MM-DD.
func monday(k int) time.Time {
	d := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d.AddDate(0, 0, 7*k)
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

// leaveType creates a paid annual type as the tenant admin and returns its id.
func leaveType(t *testing.T, a *attTenant, code string, allowance float64) string {
	t.Helper()
	status, env := a.do(t, a.adminTok, http.MethodPost, "/api/v1/leave/types", map[string]interface{}{
		"name": code + " " + randomSuffix(), "code": code, "annual_allowance": allowance, "carry_forward_limit": 5,
	})
	requireStatus(t, "create leave type "+code, status, 201, env)
	return strField(t, env.Data, "id")
}

func applyLeave(t *testing.T, a *attTenant, tok, typ string, from, to time.Time) (int, envelope) {
	t.Helper()
	return a.do(t, tok, http.MethodPost, "/api/v1/leave/requests", map[string]string{
		"leave_type_id": typ, "start_date": ymd(from), "end_date": ymd(to), "reason": "family wedding",
	})
}

func balanceFor(t *testing.T, a *attTenant, tok, typ string) map[string]interface{} {
	t.Helper()
	status, env := a.do(t, tok, http.MethodGet, "/api/v1/leave/balances/me", nil)
	requireStatus(t, "balances/me", status, 200, env)
	for _, b := range listField(t, env.Data) {
		if b["leave_type_id"] == typ {
			return b
		}
	}
	t.Fatalf("no balance for type %s: %s", typ, env.Data)
	return nil
}

// TestLeaveLive_Lifecycle — live G5 (reserve), G8 (approve → Module 3 on_leave), G13 (cancel future),
// G11 (audit, no free text), G12 (ledger invariant), preview.
func TestLeaveLive_Lifecycle(t *testing.T) {
	a := newAttTenant(t)
	el := leaveType(t, a, "EL", 12)
	mon, tue := monday(0), monday(0).AddDate(0, 0, 1)

	status, env := a.do(t, a.memberTok, http.MethodPost, "/api/v1/leave/requests/preview", map[string]string{
		"leave_type_id": el, "start_date": ymd(mon), "end_date": ymd(tue), "reason": "x"})
	requireStatus(t, "preview", status, 200, env)
	if p := objField(t, env.Data); p["total_days"].(float64) != 2 || p["available_after"].(float64) != 10 {
		t.Fatalf("preview: %s", env.Data)
	}

	status, env = applyLeave(t, a, a.memberTok, el, mon, tue)
	requireStatus(t, "G5 apply", status, 201, env)
	reqID := strField(t, env.Data, "id")
	if b := balanceFor(t, a, a.memberTok, el); b["reserved"].(float64) != 2 || b["available"].(float64) != 10 || b["accrued"].(float64) != 12 {
		t.Fatalf("G5 balance: %+v", b)
	}

	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/leave/requests/"+reqID+"/approve", map[string]string{"comment": "enjoy"})
	requireStatus(t, "G8 approve", status, 200, env)
	marked := psql(t, "SELECT count(*) FROM attendance_records WHERE employee_profile_id = '"+a.memberEmp+
		"' AND status = 'on_leave' AND source = 'leave' AND attendance_date IN ('"+ymd(mon)+"','"+ymd(tue)+"')")
	if marked != "2" {
		t.Fatalf("G8: Module 3 must hold 2 on_leave records, got %s", marked)
	}
	if b := balanceFor(t, a, a.memberTok, el); b["used"].(float64) != 2 || b["reserved"].(float64) != 0 {
		t.Fatalf("G8 balance: %+v", b)
	}

	status, env = a.do(t, a.memberTok, http.MethodPost, "/api/v1/leave/requests/"+reqID+"/cancel", nil)
	requireStatus(t, "G13 cancel approved future", status, 200, env)
	if left := psql(t, "SELECT count(*) FROM attendance_records WHERE employee_profile_id = '"+a.memberEmp+"'"); left != "0" {
		t.Fatalf("G13: leave-only attendance records must be removed, %s left", left)
	}
	if b := balanceFor(t, a, a.memberTok, el); b["used"].(float64) != 0 || b["available"].(float64) != 12 {
		t.Fatalf("G13 balance: %+v", b)
	}

	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/leave/balances/adjust", map[string]interface{}{
		"employee_id": a.memberEmp, "leave_type_id": el, "days": -1.5, "reason": "medical certificate correction"})
	requireStatus(t, "adjust", status, 200, env)
	if objField(t, env.Data)["available"].(float64) != 10.5 {
		t.Fatalf("adjust: %s", env.Data)
	}

	// G12: projection == ledger sums per kind.
	ok := psql(t, `SELECT bool_and(ok) FROM (
		SELECT b.opening = COALESCE(SUM(l.days) FILTER (WHERE l.kind = 'carry_forward'), 0)
		   AND b.accrued = COALESCE(SUM(l.days) FILTER (WHERE l.kind = 'accrual'), 0)
		   AND b.adjusted = COALESCE(SUM(l.days) FILTER (WHERE l.kind = 'adjustment'), 0)
		   AND b.used = COALESCE(SUM(l.days) FILTER (WHERE l.kind IN ('consume','reversal')), 0)
		   AND b.reserved = COALESCE(SUM(l.days) FILTER (WHERE l.kind IN ('reserve','release')), 0) AS ok
		FROM leave_balances b LEFT JOIN leave_ledger l ON l.tenant_id = b.tenant_id AND l.employee_profile_id = b.employee_profile_id
		 AND l.leave_type_id = b.leave_type_id AND l.year = b.year
		WHERE b.employee_profile_id = '`+a.memberEmp+`' GROUP BY b.id) s`)
	if ok != "t" {
		t.Fatalf("G12 ledger invariant broken (%s)", ok)
	}
	if n := psql(t, "SELECT count(*) FROM leave_ledger WHERE employee_profile_id = '"+a.memberEmp+"'"); n != "6" {
		t.Fatalf("G12: accrual, reserve, release, consume, reversal, adjustment = 6 rows, got %s", n)
	}

	// G11: audit rows and privacy.
	audits := psql(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = '"+a.tenantID+"' AND action IN ('leave.applied','leave.approved','leave.cancelled','leave.balance_adjusted','leave.type_created')")
	if audits != "5" {
		t.Fatalf("G11: expected 5 audit rows, got %s", audits)
	}
	leak := "metadata::text LIKE '%wedding%' OR metadata::text LIKE '%medical%' OR metadata::text LIKE '%enjoy%'"
	if n := psql(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = '"+a.tenantID+"' AND ("+leak+")"); n != "0" {
		t.Fatalf("G11: audit metadata leaks free text (%s rows)", n)
	}
	payloadLeak := strings.ReplaceAll(leak, "metadata", "payload")
	if n := psql(t, "SELECT count(*) FROM leave_events_outbox WHERE tenant_id = '"+a.tenantID+"' AND ("+payloadLeak+")"); n != "0" {
		t.Fatalf("FR-EV003: outbox payload leaks free text (%s rows)", n)
	}
	if n := psql(t, "SELECT count(DISTINCT routing_key) FROM leave_events_outbox WHERE tenant_id = '"+a.tenantID+"'"); n != "4" {
		t.Fatalf("expected applied, approved, cancelled, accrued events; got %s kinds", n)
	}
}

// TestLeaveLive_G7_Overlap_G15_Pagination — live G7 and G15.
func TestLeaveLive_G7_Overlap_G15_Pagination(t *testing.T) {
	a := newAttTenant(t)
	el := leaveType(t, a, "EL", 20)
	mon := monday(1)
	status, env := applyLeave(t, a, a.memberTok, el, mon, mon.AddDate(0, 0, 2))
	requireStatus(t, "apply Mon–Wed", status, 201, env)
	status, env = applyLeave(t, a, a.memberTok, el, mon.AddDate(0, 0, 1), mon.AddDate(0, 0, 1))
	requireCode(t, "G7 overlap", status, 409, env, "LEAVE_OVERLAP")
	thu := mon.AddDate(0, 0, 3)
	for _, half := range []string{"first_half", "second_half"} {
		status, env = a.do(t, a.memberTok, http.MethodPost, "/api/v1/leave/requests", map[string]string{
			"leave_type_id": el, "start_date": ymd(thu), "end_date": ymd(thu), "half_day": half, "reason": "x"})
		requireStatus(t, "G7 "+half, status, 201, env)
	}
	for _, q := range []string{"?per_page=0", "?page=-1&per_page=abc", "?per_page=100000"} {
		status, env = a.do(t, a.adminTok, http.MethodGet, "/api/v1/leave/requests"+q, nil)
		requireStatus(t, "G15 "+q, status, 200, env)
		if pp := objField(t, env.Meta)["per_page"].(float64); pp < 1 || pp > 100 {
			t.Fatalf("G15 %s per_page %v", q, pp)
		}
	}
	status, env = a.do(t, a.memberTok, http.MethodGet, "/api/v1/leave/requests/me?status=pending", nil)
	requireStatus(t, "list mine", status, 200, env)
	if rows := listField(t, env.Data); len(rows) != 3 || rows[0]["leave_type_code"] != "EL" {
		t.Fatalf("mine: %s", env.Data)
	}
}
