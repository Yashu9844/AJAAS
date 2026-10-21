package integration

import (
	"testing"
)

// TestCompensationLifecycle — CO-C01..CO-C26 end to end: ranges, auto and approved off-cycle changes synced to
// payroll, worker views, and a merit cycle from draft to applied with guideline, budget and maker-checker rules.
func TestCompensationLifecycle(t *testing.T) {
	cc := newCoreTenant(t)
	alice := cc.memberEmp
	cc.complete(t, "hire", alice, cc.day(-200), map[string]any{"position_id": cc.p1, "manager_employee_id": cc.adminEmp,
		"worker_type": "employee", "employment_type": "full_time"})
	approver, _, _ := cc.limitedUser(t, [2]string{"compensation", "approve"}, [2]string{"compensation", "read"})
	sid := cc.payrollStructure(t)
	expect(t, cc.do(t, "POST", "/hr/payroll/assignments", body(map[string]any{"employee_id": alice, "structure_id": sid,
		"annual_ctc": 1200000, "effective_from": cc.day(-260)})), 201, "payroll assignment")
	const base = "/hr/compensation"

	// CO-C01 ranges.
	rng := map[string]any{"grade_id": cc.g1, "min_amount": 800000, "mid_amount": 1000000, "max_amount": 1200000, "effective_from": cc.day(-300)}
	expect(t, cc.do(t, "POST", base+"/pay-ranges", body(rng)), 201, "CO-C01 range")
	expectErr(t, cc.do(t, "POST", base+"/pay-ranges", body(rng)), 409, "BACKDATED", "CO-C01 same start")
	if n := len(cc.do(t, "GET", base+"/pay-ranges?grade_id="+cc.g1).list("data")); n != 1 {
		t.Fatalf("ranges: %d", n)
	}

	// CO-C10/C11 first record applies at once and syncs payroll.
	first := cc.do(t, "POST", base+"/changes", body(map[string]any{"employee_id": alice, "annual_base": 1000000, "effective_from": cc.day(-200), "reason": "hire"}))
	if expect(t, first, 201, "first change"); first.str("data.status") != "approved" {
		t.Fatalf("first change: %s", first.Raw)
	}
	if s := sqlStr(t, "SELECT payroll_synced::text FROM comp_records WHERE employee_profile_id = ?", alice); s != "true" {
		t.Fatalf("CO-C11 payroll sync: %s", s)
	}
	if c := sqlStr(t, "SELECT annual_ctc::text FROM payroll_assignments WHERE employee_profile_id = ? AND effective_to IS NULL", alice); c != "1000000.00" {
		t.Fatalf("CO-C11 payroll CTC: %s", c)
	}

	// CO-C13 a raise over 10% needs an approver who is not the requester.
	big := cc.do(t, "POST", base+"/changes", body(map[string]any{"employee_id": alice, "annual_base": 1300000, "effective_from": cc.day(-100),
		"reason": "promotion", "comment": "SECRET-COMP-NOTE"}))
	if expect(t, big, 201, "big change"); big.str("data.status") != "pending" {
		t.Fatalf("big change: %s", big.Raw)
	}
	bid := big.str("data.id")
	expectErr(t, cc.do(t, "POST", base+"/changes", body(map[string]any{"employee_id": alice, "annual_base": 900000, "effective_from": cc.day(-90),
		"reason": "correction"})), 409, "CONFLICT", "CO-C13 second pending")
	expectErr(t, cc.do(t, "POST", base+"/changes/"+bid+"/approve"), 403, "COMP_SELF_APPROVAL", "CO-C13 requester approves")
	expect(t, cc.as(t, approver, "POST", base+"/changes/"+bid+"/approve", body(map[string]any{"comment": "ok"})), 200, "approve change")
	expectErr(t, cc.as(t, approver, "POST", base+"/changes/"+bid+"/reject"), 409, "CHANGE_STATE", "decided twice")
	if to := sqlStr(t, "SELECT effective_to::text FROM comp_records WHERE employee_profile_id = ? AND annual_base = 1000000", alice); to != cc.day(-100) {
		t.Fatalf("CO-C11 previous record closed: %s", to)
	}

	// CO-C12 / CO-C02 worker views.
	w := cc.do(t, "GET", base+"/workers/"+alice)
	if expect(t, w, 200, "worker"); w.get("data.current.annual_base") != 1300000.0 || w.get("data.position.compa_ratio") != 1.3 ||
		w.str("data.position.compa_band") != "above" || len(w.list("data.history")) != 2 {
		t.Fatalf("CO-C12 worker: %s", w.Raw)
	}
	if me := cc.as(t, cc.memberTok, "GET", base+"/me"); me.get("data.current.annual_base") != 1300000.0 {
		t.Fatalf("CO-C12 me: %s", me.Raw)
	}
	expectErr(t, cc.as(t, cc.memberTok, "GET", base+"/workers/"+alice), 403, "", "member reads others")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/changes", body(map[string]any{})), 403, "", "member changes pay")

	// CO-C20..CO-C26 merit cycle.
	cy := cc.do(t, "POST", base+"/merit-cycles", body(map[string]any{"name": "FY merit", "effective_date": cc.day(10),
		"performance_cycle_id": cc.g1, "guidelines": []map[string]any{{"rating": "unrated", "compa": "above", "min_pct": 0, "max_pct": 3}}}))
	expect(t, cy, 201, "cycle")
	cid := cy.str("data.id")
	expect(t, cc.do(t, "PUT", base+"/merit-cycles/"+cid+"/budgets", body([]map[string]any{{"department_id": cc.dept1, "amount": 50000}})), 200, "budgets")
	expectErr(t, cc.do(t, "POST", base+"/merit-cycles/"+cid+"/open"), 404, "", "CO-C22 unknown performance cycle")
	expect(t, cc.do(t, "PATCH", base+"/merit-cycles/"+cid, body(map[string]any{"performance_cycle_id": ""})), 200, "clear performance cycle")
	expect(t, cc.do(t, "POST", base+"/merit-cycles/"+cid+"/open"), 200, "open")
	props := cc.do(t, "GET", base+"/merit-cycles/"+cid+"/proposals")
	if len(props.list("data")) != 1 || props.str("data.0.rating_band") != "unrated" || props.str("data.0.compa_band") != "above" {
		t.Fatalf("CO-C22 snapshot: %s", props.Raw)
	}
	pid := props.str("data.0.id")
	if mine := cc.do(t, "GET", base+"/merit-cycles/"+cid+"/proposals/mine"); len(mine.list("data")) != 1 {
		t.Fatalf("CO-C26 manager view: %s", mine.Raw)
	}
	if mine := cc.as(t, cc.memberTok, "GET", base+"/merit-cycles/"+cid+"/proposals"); len(mine.list("data")) != 0 {
		t.Fatalf("CO-C26 worker sees no proposals: %s", mine.Raw)
	}
	expectErr(t, cc.do(t, "PATCH", base+"/proposals/"+pid, body(map[string]any{"increase_pct": 5})), 400, "VALIDATION_ERROR", "CO-C23 outside guideline")
	expectErr(t, cc.do(t, "PATCH", base+"/proposals/"+pid, body(map[string]any{"increase_pct": 4, "justification": "star"})), 409, "BUDGET_EXCEEDED", "CO-C24 budget")
	if p := cc.do(t, "PATCH", base+"/proposals/"+pid, body(map[string]any{"increase_pct": 3})); p.get("data.amount") != 39000.0 {
		t.Fatalf("CO-C23 amount: %s", p.Raw)
	}
	if u := cc.do(t, "GET", base+"/merit-cycles/"+cid+"/usage"); u.get("data.0.used") != 39000.0 || u.get("data.0.remaining") != 11000.0 {
		t.Fatalf("CO-C24 usage: %s", u.Raw)
	}
	expect(t, cc.do(t, "POST", base+"/merit-cycles/"+cid+"/close"), 200, "close")
	expectErr(t, cc.do(t, "POST", base+"/merit-cycles/"+cid+"/approve"), 403, "COMP_SELF_APPROVAL", "CO-C25 opener approves")
	expect(t, cc.as(t, approver, "POST", base+"/merit-cycles/"+cid+"/approve"), 200, "approve cycle")
	expect(t, cc.do(t, "POST", base+"/merit-cycles/"+cid+"/apply"), 200, "apply")
	expectErr(t, cc.do(t, "POST", base+"/merit-cycles/"+cid+"/apply"), 409, "CYCLE_STATE", "CO-C25 apply twice")
	if b := sqlStr(t, "SELECT annual_base::text || '/' || reason FROM comp_records WHERE employee_profile_id = ? AND effective_to IS NULL", alice); b != "1339000.00/merit" {
		t.Fatalf("CO-C25 merit record: %s", b)
	}

	// Events carry ids only; isolation.
	if n := sqlStr(t, "SELECT count(*)::text FROM comp_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%1300000%' OR payload::text LIKE '%SECRET%')", cc.id); n != "0" {
		t.Fatalf("event privacy: %s", n)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM comp_events_outbox WHERE tenant_id = ?", cc.id); n != "4" {
		t.Fatalf("events: %s", n)
	}
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", base+"/changes/"+bid), 404, "NOT_FOUND", "foreign change")
}
