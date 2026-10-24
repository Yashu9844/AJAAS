package integration

import (
	"sync"
	"testing"
	"time"
)

// TestBenefits ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â BG1..BG5: enrolment outside a window is refused, one live election per worker and plan under
// concurrent requests, payroll deducts exactly the elected level, a termination stops the deduction and rescinding it
// brings it back, and dependants and life events never leak.
func TestBenefits(t *testing.T) {
	cc := newCoreTenant(t)
	const base = "/hr/benefits"
	now := time.Now().UTC()
	today := ymd(now)

	plan := cc.do(t, "POST", base+"/plans", body(map[string]any{"code": "med-" + uniq(), "name": "Group medical", "category": "health", "plan_year": now.Year(), "effective_from": ymd(now.AddDate(0, -9, 0)),
		"levels": []map[string]any{{"code": "SELF", "name": "Employee", "employee_cost": 500, "employer_cost": 1500, "max_dependants": 0},
			{"code": "FAMILY", "name": "Family", "employee_cost": 1200, "employer_cost": 2800, "max_dependants": 2}}}))
	expect(t, plan, 201, "plan")
	pid := plan.str("data.id")
	expectErr(t, cc.do(t, "POST", base+"/plans", body(map[string]any{"code": plan.str("data.code"), "name": "Dup", "category": "health", "plan_year": now.Year(), "effective_from": today,
		"levels": []map[string]any{{"code": "A", "name": "A"}}})), 409, "CONFLICT", "duplicate plan code")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/plans", body(map[string]any{})), 403, "FORBIDDEN", "a worker cannot create plans")
	expect(t, cc.do(t, "POST", base+"/plans/"+pid+"/activate"), 200, "activate")

	// BG1 outside any window an election is refused.
	if o := cc.as(t, cc.memberTok, "GET", base+"/options"); o.get("data.window.open") != false || len(o.list("data.plans")) != 1 {
		t.Fatalf("BG1 closed window, plan offered: %s", o.Raw)
	}
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/elections", body(map[string]any{"plan_id": pid, "level_code": "SELF"})), 409, "ENROLMENT_WINDOW_CLOSED", "BG1 no window")
	win := cc.do(t, "POST", base+"/windows", body(map[string]any{"name": "Annual", "starts_on": ymd(now.AddDate(0, 0, -1)), "ends_on": ymd(now.AddDate(0, 0, 10))}))
	expect(t, win, 201, "open enrolment window")
	if o := cc.as(t, cc.memberTok, "GET", base+"/options"); o.get("data.window.open") != true || o.str("data.window.reason") != "open_enrolment" {
		t.Fatalf("window open: %s", o.Raw)
	}

	// Dependants (sensitive) and an election covering one.
	dep := cc.as(t, cc.memberTok, "POST", base+"/dependants", body(map[string]any{"name": "Asha SecretName", "relationship": "child", "date_of_birth": "2018-04-02"}))
	expect(t, dep, 201, "dependant")
	did := dep.str("data.id")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/elections", body(map[string]any{"plan_id": pid, "level_code": "SELF", "dependant_ids": []string{did}})), 400, "VALIDATION_ERROR", "SELF covers no dependants")
	el := cc.as(t, cc.memberTok, "POST", base+"/elections", body(map[string]any{"plan_id": pid, "level_code": "FAMILY", "dependant_ids": []string{did}}))
	expect(t, el, 201, "elect")
	eid := el.str("data.id")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/dependants/"+did+"/remove"), 409, "DEPENDANT_IN_USE", "covered dependant")

	// BG2 concurrent requests for one worker and plan: exactly one election is created.
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = cc.do(t, "POST", base+"/elections", body(map[string]any{"plan_id": pid, "level_code": "SELF"})).Status
		}(i)
	}
	wg.Wait()
	created, conflicts := 0, 0
	for _, c := range codes {
		switch c {
		case 201:
			created++
		case 409:
			conflicts++
		}
	}
	if created != 1 || conflicts != len(codes)-1 {
		t.Fatalf("BG2 one winner among concurrent requests: %v", codes)
	}
	if n := sqlStr(t, "SELECT count(*) FROM ben_elections WHERE tenant_id = ? AND plan_id = ? AND effective_to IS NULL AND status = 'active'", cc.id, pid); n != "2" {
		t.Fatalf("BG2 one live election per worker (admin and member): %s", n)
	}

	// Waive, replace the waiver, end.
	other := cc.do(t, "POST", base+"/plans", body(map[string]any{"code": "life-" + uniq(), "name": "Group life", "category": "life", "plan_year": now.Year(), "effective_from": ymd(now.AddDate(0, -9, 0)),
		"levels": []map[string]any{{"code": "BASIC", "name": "Basic", "employee_cost": 100, "employer_cost": 300}}}))
	expect(t, other, 201, "second plan")
	oid := other.str("data.id")
	expect(t, cc.do(t, "POST", base+"/plans/"+oid+"/activate"), 200, "activate second")
	w := cc.as(t, cc.memberTok, "POST", base+"/elections/waive", body(map[string]any{"plan_id": oid}))
	expect(t, w, 201, "waive")
	if again := cc.as(t, cc.memberTok, "POST", base+"/elections/waive", body(map[string]any{"plan_id": oid})); again.str("data.id") != w.str("data.id") {
		t.Fatalf("waiving twice returns the waiver: %s", again.Raw)
	}
	back := cc.as(t, cc.memberTok, "POST", base+"/elections", body(map[string]any{"plan_id": oid, "level_code": "BASIC"}))
	expect(t, back, 201, "electing replaces the waiver")
	expect(t, cc.as(t, cc.memberTok, "POST", base+"/elections/"+back.str("data.id")+"/end"), 200, "end")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/elections/"+back.str("data.id")+"/end"), 409, "ELECTION_STATE", "ended twice")

	// BG5 payroll deducts exactly the elected level's employee cost (1,200 for FAMILY) for last month.
	if err := testDB.Exec("UPDATE ben_elections SET effective_from = ? WHERE id = ?", ymd(now.AddDate(0, -8, 0)), eid).Error; err != nil {
		t.Fatal(err)
	}
	st := cc.do(t, "POST", "/hr/payroll/structures", body(map[string]any{"name": "Ben " + uniq(), "components": []map[string]any{
		{"code": "BASIC", "name": "Basic", "kind": "earning", "calc": "percent_of_ctc", "value": 40, "taxable": true},
		{"code": "SPECIAL", "name": "Special", "kind": "earning", "calc": "balance", "taxable": true}}}))
	expect(t, st, 201, "structure")
	expect(t, cc.do(t, "POST", "/hr/payroll/assignments", body(map[string]any{"employee_id": cc.memberEmp, "structure_id": st.str("data.id"), "annual_ctc": 1200000, "effective_from": "2026-01-01"})), 201, "assign")
	last := now.AddDate(0, -1, 0)
	run := cc.do(t, "POST", "/hr/payroll/runs", body(map[string]any{"month": int(last.Month()), "year": last.Year()}))
	expect(t, run, 201, "run")
	expect(t, cc.do(t, "POST", "/hr/payroll/runs/"+run.str("data.id")+"/calculate"), 200, "calculate")
	var slipID string
	for _, it := range cc.do(t, "GET", "/hr/payroll/runs/"+run.str("data.id")+"/payslips").list("data") {
		if m := it.(map[string]any); m["employee_id"] == cc.memberEmp {
			slipID = m["id"].(string)
		}
	}
	slip := cc.do(t, "GET", "/hr/payroll/payslips/"+slipID)
	var benLine map[string]any
	for _, l := range slip.list("data.lines") {
		if m := l.(map[string]any); len(m["code"].(string)) > 4 && m["code"].(string)[:4] == "BEN_" {
			benLine = m
		}
	}
	if expect(t, slip, 200, "payslip"); benLine == nil || benLine["amount"] != 1200.0 || benLine["kind"] != "deduction" {
		t.Fatalf("BG5 the payslip carries the elected FAMILY cost: %s", slip.Raw)
	}
	if dd := cc.as(t, cc.memberTok, "GET", base+"/deductions/me?month="+last.Format("2006-01")); len(dd.list("data")) != 1 || dd.list("data")[0].(map[string]any)["amount"] != 1200.0 {
		t.Fatalf("the worker previews the same deduction: %s", dd.Raw)
	}

	// BG3 a termination ends the election, and rescinding it brings it back.
	cc.complete(t, "hire", cc.memberEmp, cc.day(-10), map[string]any{"position_id": cc.p1, "manager_employee_id": cc.adminEmp, "worker_type": "employee", "employment_type": "full_time"})
	term := cc.complete(t, "terminate", cc.memberEmp, cc.day(-3), map[string]any{"termination_type": "voluntary", "last_working_day": cc.day(-1)})
	if s := sqlStr(t, "SELECT status||'/'||effective_to||'/'||end_reason FROM ben_elections WHERE id = ?", eid); s != "ended/"+today+"/termination" {
		t.Fatalf("BG3 the election ends the day after the last working day: %s", s)
	}
	expect(t, cc.as(t, cc.approver, "POST", "/hr/core/actions/"+term.str("data.id")+"/rescind", body(map[string]any{"reason": "withdrawn resignation"})), 200, "rescind")
	if s := sqlStr(t, "SELECT status||'/'||coalesce(effective_to::text, 'open') FROM ben_elections WHERE id = ?", eid); s != "active/open" {
		t.Fatalf("BG3 the rescinded termination reinstates the election: %s", s)
	}

	// BG4 dependants and life events never leak: not to a worker without read_sensitive, not into events or audit.
	reader, _, _ := cc.limitedUser(t, [2]string{"benefits", "read"})
	expectErr(t, cc.as(t, reader, "GET", base+"/employees/"+cc.memberEmp+"/dependants"), 403, "FORBIDDEN", "BG4 read is not read_sensitive")
	expectErr(t, cc.as(t, reader, "GET", base+"/employees/"+cc.memberEmp+"/life-events"), 403, "FORBIDDEN", "BG4 life events")
	expectErr(t, cc.as(t, reader, "GET", base+"/elections"), 403, "FORBIDDEN", "BG4 elections")
	if l := cc.do(t, "GET", base+"/employees/"+cc.memberEmp+"/dependants"); len(l.list("data")) != 1 {
		t.Fatalf("HR with read_sensitive sees them: %s", l.Raw)
	}
	le := cc.do(t, "POST", base+"/life-events", body(map[string]any{"type": "birth", "event_date": today, "employee_id": cc.memberEmp, "evidence_ref": "docs/secret-certificate.pdf"}))
	expect(t, le, 201, "life event on behalf")
	expectErr(t, cc.as(t, reader, "POST", base+"/life-events", body(map[string]any{"type": "birth", "event_date": today, "employee_id": cc.adminEmp})), 403, "FORBIDDEN", "a worker cannot record for others")
	if n := sqlStr(t, "SELECT count(*) FROM ben_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%SecretName%' OR payload::text LIKE '%secret-certificate%' OR payload::text LIKE '%1200%' OR payload::text LIKE '%2018-04-02%')", cc.id); n != "0" {
		t.Fatalf("BG4 events leak content (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'benefits.%' AND (metadata::text LIKE '%SecretName%' OR metadata::text LIKE '%secret-certificate%' OR metadata::text LIKE '%2018-04-02%')", cc.id); n != "0" {
		t.Fatalf("BG4 audit leaks content (%s rows)", n)
	}

	// Plan costs change only through a new version; the report counts without naming anyone.
	expect(t, cc.do(t, "POST", base+"/plans/"+pid+"/versions", body(map[string]any{"effective_from": ymd(now.AddDate(0, 0, 30)),
		"levels": []map[string]any{{"code": "SELF", "name": "Employee", "employee_cost": 650, "employer_cost": 1500, "max_dependants": 0}, {"code": "FAMILY", "name": "Family", "employee_cost": 1400, "employer_cost": 2800, "max_dependants": 2}}})), 201, "new version")
	expectErr(t, cc.do(t, "PATCH", base+"/plans/"+pid, body(map[string]any{"levels": []map[string]any{{"code": "X", "name": "X"}}})), 400, "VALIDATION_ERROR", "levels are not patchable")
	rep := cc.do(t, "GET", base+"/reports/enrolment")
	if expect(t, rep, 200, "report"); len(rep.list("data")) < 1 {
		t.Fatalf("report: %s", rep.Raw)
	}
}
