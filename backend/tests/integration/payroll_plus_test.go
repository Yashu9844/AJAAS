package integration

import (
	"testing"
	"time"
)

// TestPayrollParity â€” PG1..PG6: employer contributions in a run, state PT slabs, arrears from a backdated change and a
// final settlement for a leaver (gratuity, clearance gate, maker-checker, payout).
func TestPayrollParity(t *testing.T) {
	cc := newCoreTenant(t)
	const base = "/hr/payroll"
	now := time.Now().UTC()

	// A structure with PF and PT on: BASIC 40% of CTC, SPECIAL the balance.
	st := cc.do(t, "POST", base+"/structures", body(map[string]any{"name": "Parity " + uniq(), "pf_enabled": true, "pt_enabled": true, "components": []map[string]any{
		{"code": "BASIC", "name": "Basic", "kind": "earning", "calc": "percent_of_ctc", "value": 40, "taxable": true},
		{"code": "SPECIAL", "name": "Special", "kind": "earning", "calc": "balance", "taxable": true}}}))
	expect(t, st, 201, "structure")
	sid := st.str("data.id")

	// PG1 employer contributions: a run of last month for the member (CTC 1,248,000 â†’ BASIC 41,600 â†’ PF wage capped at 15,000).
	expect(t, cc.do(t, "POST", base+"/assignments", body(map[string]any{"employee_id": cc.memberEmp, "structure_id": sid, "annual_ctc": 1248000, "effective_from": "2026-01-01"})), 201, "assign member")
	last := now.AddDate(0, -1, 0)
	run := cc.do(t, "POST", base+"/runs", body(map[string]any{"month": int(last.Month()), "year": last.Year()}))
	expect(t, run, 201, "run")
	rid := run.str("data.id")
	calc := cc.do(t, "POST", base+"/runs/"+rid+"/calculate")
	expect(t, calc, 200, "calculate")
	slips := cc.do(t, "GET", base+"/runs/"+rid+"/payslips")
	var memberSlip string
	for _, it := range slips.list("data") {
		if m := it.(map[string]any); m["employee_id"] == cc.memberEmp {
			memberSlip = m["id"].(string)
		}
	}
	slip := cc.do(t, "GET", base+"/payslips/"+memberSlip)
	if expect(t, slip, 200, "payslip"); num(slip, "data.employer_cost") != 1950 || !slip.has("data.lines", "code", "EPF_ER") || !slip.has("data.lines", "kind", "employer") {
		t.Fatalf("PG1 employer lines: %s", slip.Raw)
	}
	if num(calc, "data.employer_total") < 1950 {
		t.Fatalf("PG1 run employer total: %s", calc.Raw)
	}

	// PG2 state PT slabs: management and validation.
	slabs := map[string]any{"slabs": []map[string]any{{"min_gross": 0, "max_gross": 9999, "amount": 0}, {"min_gross": 10000, "amount": 150}, {"min_gross": 10000, "amount": 250, "month": 2}}}
	expect(t, cc.do(t, "PUT", base+"/pt-slabs/KA", body(slabs)), 200, "slabs")
	expectErr(t, cc.do(t, "PUT", base+"/pt-slabs/KA", body(map[string]any{"slabs": []map[string]any{{"min_gross": 500, "amount": 1}}})), 400, "INVALID_SLABS", "must start at 0")
	expectErr(t, cc.do(t, "PUT", base+"/pt-slabs/KAR", body(slabs)), 400, "VALIDATION_ERROR", "state code")
	if l := cc.do(t, "GET", base+"/pt-slabs"); len(l.list("data.tenant.KA")) != 3 || l.get("data.defaults.MH") == nil {
		t.Fatalf("PG2 slab list: %s", l.Raw)
	}
	expect(t, cc.do(t, "DELETE", base+"/pt-slabs/KA"), 204, "delete slabs")
	expectErr(t, cc.do(t, "DELETE", base+"/pt-slabs/KA"), 404, "NOT_FOUND", "delete again")

	// PG6 settlement for a leaver: joined 2019, exit 2030-06-30 â†’ 11 years 6 months â†’ 12 years of gratuity.
	leaver, leaverUser := cc.mkEmployeeFor(t, "lv", nil, map[string]any{"joining_date": "2019-01-01T00:00:00Z"})
	_ = leaverUser
	expect(t, cc.do(t, "POST", base+"/assignments", body(map[string]any{"employee_id": leaver, "structure_id": sid, "annual_ctc": 1248000, "effective_from": "2019-01-01"})), 201, "assign leaver")
	expectErr(t, cc.do(t, "POST", base+"/settlements", body(map[string]any{"employee_id": leaver})), 409, "NOT_A_LEAVER", "active worker")
	expect(t, cc.do(t, "POST", "/hr/employees/"+leaver+"/status", body(map[string]any{"status": "notice", "notes": "leaving"})), 200, "notice")
	expect(t, cc.do(t, "POST", "/hr/employees/"+leaver+"/status", body(map[string]any{"status": "resigned", "exit_date": "2030-06-30T00:00:00Z", "exit_reason": "relocation"})), 200, "resign")
	set := cc.do(t, "POST", base+"/settlements", body(map[string]any{"employee_id": leaver}))
	expect(t, set, 201, "settlement")
	setID := set.str("data.id")
	// BASIC 41,600 Ã— 15 / 26 = 24,000 a year Ã— 12 years.
	if num(set, "data.gratuity") != 288000 || num(set, "data.net") != 288000 || set.str("data.status") != "draft" || !set.has("data.lines", "code", "GRATUITY") {
		t.Fatalf("PG5 settlement: %s", set.Raw)
	}
	expect(t, cc.do(t, "POST", base+"/settlements", body(map[string]any{"employee_id": leaver})), 201, "calculating again refreshes the draft")
	approver, _, _ := cc.limitedUser(t, [2]string{"payroll", "approve"}, [2]string{"payroll", "read"})
	expectErr(t, cc.do(t, "POST", base+"/settlements/"+setID+"/approve"), 403, "PAY_SELF_APPROVAL", "calculator approves")
	expectErr(t, cc.as(t, approver, "POST", base+"/settlements/"+setID+"/approve"), 409, "CLEARANCE_PENDING", "no clearance yet")
	// Exit clearance through onboarding: start an offboarding checklist and sign all four areas.
	ob := cc.do(t, "POST", "/hr/onboarding/checklists", body(map[string]any{"employee_id": leaver, "kind": "offboarding", "anchor_date": ymd(now)}))
	expect(t, ob, 201, "offboarding checklist")
	for _, area := range []string{"manager", "hr", "it", "finance"} {
		expect(t, cc.do(t, "POST", "/hr/onboarding/checklists/"+ob.str("data.id")+"/clearances/"+area+"/clear"), 200, "clear "+area)
	}
	expectErr(t, cc.do(t, "POST", base+"/settlements/"+setID+"/pay"), 409, "SETTLEMENT_STATE", "pay before approval")
	appr := cc.as(t, approver, "POST", base+"/settlements/"+setID+"/approve")
	if expect(t, appr, 200, "approve"); appr.str("data.status") != "approved" {
		t.Fatalf("approve: %s", appr.Raw)
	}
	expectErr(t, cc.do(t, "POST", base+"/settlements", body(map[string]any{"employee_id": leaver})), 409, "CONFLICT", "open settlement")
	paid := cc.do(t, "POST", base+"/settlements/"+setID+"/pay")
	if expect(t, paid, 200, "pay"); paid.str("data.status") != "paid" || paid.str("data.paid_at") == "" {
		t.Fatalf("pay: %s", paid.Raw)
	}
	expectErr(t, cc.do(t, "POST", base+"/settlements/"+setID+"/pay"), 409, "SETTLEMENT_STATE", "pay twice")
	if l := cc.do(t, "GET", base+"/settlements", query("employee_id", leaver, "status", "paid")); len(l.list("data")) != 1 {
		t.Fatalf("settlement list: %s", l.Raw)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM payroll_events_outbox WHERE tenant_id = ? AND routing_key LIKE 'payroll.settlement.%' AND payload::text LIKE '%288000%'", cc.id); n != "0" {
		t.Fatalf("PP-08 events carry no amounts: %s", n)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM payroll_events_outbox WHERE tenant_id = ? AND routing_key IN ('payroll.settlement.approved', 'payroll.settlement.paid')", cc.id); n != "2" {
		t.Fatalf("settlement events: %s", n)
	}

	// PG4 arrears: finalize last month, then a backdated raise for the member is settled once in the next run.
	expect(t, cc.as(t, approver, "POST", base+"/runs/"+rid+"/approve"), 200, "approve run")
	expect(t, cc.as(t, approver, "POST", base+"/runs/"+rid+"/finalize"), 200, "finalize run")
	monthStart := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC)
	expect(t, cc.do(t, "POST", base+"/assignments", body(map[string]any{"employee_id": cc.memberEmp, "structure_id": sid, "annual_ctc": 1368000, "effective_from": ymd(monthStart)})), 201, "backdated raise")
	next := cc.do(t, "POST", base+"/runs", body(map[string]any{"month": int(now.Month()), "year": now.Year()}))
	expect(t, next, 201, "next run")
	cc.do(t, "POST", base+"/runs/"+next.str("data.id")+"/calculate")
	nslips := cc.do(t, "GET", base+"/runs/"+next.str("data.id")+"/payslips")
	var nextSlip string
	for _, it := range nslips.list("data") {
		if m := it.(map[string]any); m["employee_id"] == cc.memberEmp {
			nextSlip = m["id"].(string)
		}
	}
	ns := cc.do(t, "GET", base+"/payslips/"+nextSlip)
	// Raise: 1,368,000 âˆ’ 1,248,000 = 120,000 a year â†’ 10,000 a month for the whole of last month.
	if !ns.has("data.lines", "code", "ARREARS") {
		t.Fatalf("PG4 arrears line: %s", ns.Raw)
	}
	for _, it := range ns.list("data.lines") {
		if m := it.(map[string]any); m["code"] == "ARREARS" && m["amount"] != 10000.0 {
			t.Fatalf("PG4 arrears amount: %v", m["amount"])
		}
	}
	cc.do(t, "POST", base+"/runs/"+next.str("data.id")+"/calculate")
	if n := sqlStr(t, "SELECT count(*)::text FROM payroll_arrears WHERE tenant_id = ?", cc.id); n != "1" {
		t.Fatalf("PG4 recalculation must not double arrears: %s rows", n)
	}

	// RBAC and isolation.
	for _, p := range [][2]string{{"PUT", base + "/pt-slabs/KA"}, {"POST", base + "/settlements"}, {"POST", base + "/settlements/" + setID + "/pay"}} {
		expectErr(t, cc.as(t, cc.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "member "+p[0]+" "+p[1])
	}
	if me := cc.as(t, cc.memberTok, "GET", base+"/settlements/me"); me.Status != 200 {
		t.Fatalf("settlements/me: %s", me.Raw)
	}
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", base+"/settlements/"+setID), 404, "NOT_FOUND", "foreign settlement")
}
