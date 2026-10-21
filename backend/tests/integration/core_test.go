package integration

import (
	"strings"
	"testing"
	"time"
)

// coreCtx is a tenant with job architecture, two departments, positions, a worker (alice = the member), a manager
// (the admin) and an approver holding only core:approve.
type coreCtx struct {
	*timeCtx
	approver        string
	dept1, dept2    string
	g1, g2, profile string
	p1, shared      string
	today           time.Time
}

func newCoreTenant(t *testing.T) *coreCtx {
	t.Helper()
	cc := &coreCtx{timeCtx: newTimeTenant(t), today: time.Now().UTC().Truncate(24 * time.Hour)}
	cc.approver, _, _ = cc.limitedUser(t, [2]string{"core", "approve"}, [2]string{"core", "read"})
	cc.dept1, cc.dept2 = cc.mkDept(t, "Core A"), cc.mkDept(t, "Core B")
	mk := func(path string, b map[string]any) string {
		r := cc.do(t, "POST", "/hr/core/"+path, body(b))
		expect(t, r, 201, "create "+path)
		return r.str("data.id")
	}
	fam := mk("job-families", map[string]any{"code": "eng" + uniq(), "name": "Engineering " + uniq()})
	cc.g1 = mk("grades", map[string]any{"code": "L1" + uniq(), "name": "Level 1", "rank": 1})
	cc.g2 = mk("grades", map[string]any{"code": "L2" + uniq(), "name": "Level 2", "rank": 2})
	cc.profile = mk("job-profiles", map[string]any{"code": "SWE" + uniq(), "title": "Software Engineer", "family_id": fam, "default_grade_id": cc.g1,
		"category": "staff", "probation_days": 90})
	cc.p1 = mk("positions", map[string]any{"code": "P1" + uniq(), "title": "Backend seat", "job_profile_id": cc.profile, "department_id": cc.dept1})
	cc.shared = mk("positions", map[string]any{"code": "PS" + uniq(), "title": "Shared seat", "job_profile_id": cc.profile, "department_id": cc.dept1,
		"shared": true, "budgeted_fte": "1.00"})
	return cc
}

func (cc *coreCtx) day(offset int) string { return ymd(cc.today.AddDate(0, 0, offset)) }

// action creates and submits an action as the admin; it returns the response of the submit.
func (cc *coreCtx) action(t *testing.T, typ, emp, date string, changes map[string]any) resp {
	t.Helper()
	c := cc.do(t, "POST", "/hr/core/actions", body(map[string]any{"type": typ, "employee_id": emp, "effective_date": date, "reason_code": "test",
		"comment": "SECRET-COMMENT", "changes": changes}))
	expect(t, c, 201, "create "+typ)
	return cc.do(t, "POST", "/hr/core/actions/"+c.str("data.id")+"/submit")
}

// approve approves a submitted action as the approver.
func (cc *coreCtx) approve(t *testing.T, id string) resp {
	t.Helper()
	return cc.as(t, cc.approver, "POST", "/hr/core/actions/"+id+"/approve")
}

func (cc *coreCtx) complete(t *testing.T, typ, emp, date string, changes map[string]any) resp {
	t.Helper()
	s := cc.action(t, typ, emp, date, changes)
	expect(t, s, 200, "submit "+typ)
	a := cc.approve(t, s.str("data.id"))
	expect(t, a, 200, "approve "+typ)
	return a
}

func (cc *coreCtx) worker(t *testing.T, emp, asOf string) resp {
	t.Helper()
	q := ""
	if asOf != "" {
		q = "?as_of=" + asOf
	}
	r := cc.do(t, "GET", "/hr/core/workers/"+emp+q)
	expect(t, r, 200, "worker view")
	return r
}

// TestCoreLifecycle — CG1, CG3–CG12 end to end.
func TestCoreLifecycle(t *testing.T) {
	cc := newCoreTenant(t)
	alice := cc.memberEmp
	hireChanges := map[string]any{"position_id": cc.p1, "manager_employee_id": cc.adminEmp, "worker_type": "employee", "employment_type": "full_time"}

	// CG5 self-approval, then hire.
	s := cc.action(t, "hire", alice, cc.day(-200), hireChanges)
	expect(t, s, 200, "submit hire")
	expectErr(t, cc.do(t, "POST", "/hr/core/actions/"+s.str("data.id")+"/approve"), 403, "CORE_SELF_APPROVAL", "CG5 initiator approves")
	h := cc.approve(t, s.str("data.id"))
	if expect(t, h, 200, "approve hire"); h.str("data.status") != "completed" || h.get("data.effects_applied") != true {
		t.Fatalf("hire: %s", h.Raw)
	}
	if st := sqlStr(t, "SELECT status FROM employee_profiles WHERE id = ?", alice); st != "active" {
		t.Fatalf("CG12 hire projected status (probation over): %s", st)
	}
	if m := sqlStr(t, "SELECT department_id::text || '/' || manager_user_id::text FROM mappings WHERE user_id = ? AND is_primary AND status = 'active'", cc.memberUser); m != cc.dept1+"/"+cc.adminID {
		t.Fatalf("CG12 mapping projection: %s", m)
	}

	// CG7 position never double-filled.
	bobUser, _ := cc.newUser(t, "bob")
	bob := cc.do(t, "POST", "/hr/employees", body(empBody(bobUser, "BOB-"+strings.ToUpper(uniq()), nil))).str("data.id")
	sb := cc.action(t, "hire", bob, cc.day(-100), map[string]any{"position_id": cc.p1, "worker_type": "employee", "employment_type": "full_time"})
	expectErr(t, sb, 409, "POSITION_FILLED", "CG7 second incumbent")

	// CG6 + CG4/CG11: promotion, grade rule, as-of history.
	cc.complete(t, "promote", alice, cc.day(-60), map[string]any{"grade_id": cc.g2})
	expectErr(t, cc.action(t, "promote", alice, cc.day(-50), map[string]any{"grade_id": cc.g1}), 409, "GRADE_RULE", "CG6 promote to a lower rank")
	expectErr(t, cc.action(t, "transfer", alice, cc.day(-50), map[string]any{"department_id": cc.dept1}), 409, "NO_CHANGE", "CG6 transfer without change")
	if g := cc.worker(t, alice, cc.day(-61)).str("data.current.grade_id"); g != cc.g1 {
		t.Fatalf("CG11 before promotion: %s", g)
	}
	if g := cc.worker(t, alice, cc.day(-59)).str("data.current.grade_id"); g != cc.g2 {
		t.Fatalf("CG11 after promotion: %s", g)
	}

	// CG4 future-dated transfer (clearing the dept-1 seat) is visible only from its date; earlier changes are refused.
	ft := cc.complete(t, "transfer", alice, cc.day(30), map[string]any{"department_id": cc.dept2, "clear_position": true})
	if ft.get("data.effects_applied") != false {
		t.Fatalf("CG4 future effects must wait: %s", ft.Raw)
	}
	if d := cc.worker(t, alice, "").str("data.current.department_id"); d != cc.dept1 {
		t.Fatalf("CG4 today still dept 1: %s", d)
	}
	if d := cc.worker(t, alice, cc.day(31)).str("data.current.department_id"); d != cc.dept2 {
		t.Fatalf("CG4 future view: %s", d)
	}
	expectErr(t, cc.action(t, "change_manager", alice, cc.day(10), map[string]any{"manager_employee_id": bob}), 409, "LATER_CHANGE_EXISTS", "CG4 earlier change after a later one")
	if r := cc.do(t, "POST", "/hr/core/effects/run"); r.get("data.processed") != 0.0 {
		t.Fatalf("CG4 nothing due: %s", r.Raw)
	}
	expect(t, cc.as(t, cc.approver, "POST", "/hr/core/actions/"+ft.str("data.id")+"/rescind", body(map[string]any{"reason": "plan changed"})), 200, "rescind future transfer")

	// CG8 termination effects, CG9 rescinded termination restores everything.
	term := cc.complete(t, "terminate", alice, cc.day(-3), map[string]any{"termination_type": "voluntary", "last_working_day": cc.day(-1)})
	if st := sqlStr(t, "SELECT status FROM employee_profiles WHERE id = ?", alice); st != "terminated" {
		t.Fatalf("CG8 status: %s", st)
	}
	if st := sqlStr(t, "SELECT status FROM users WHERE id = ?", cc.memberUser); st != "inactive" {
		t.Fatalf("CG8 login: %s", st)
	}
	if p := cc.do(t, "GET", "/hr/core/positions/"+cc.p1); p.get("data.vacant") != true {
		t.Fatalf("CG8 position freed: %s", p.Raw)
	}
	if p := cc.do(t, "GET", "/hr/core/positions/"+cc.p1+"?as_of="+cc.day(-2)); p.get("data.vacant") != false {
		t.Fatalf("CG8 occupied on the last days: %s", p.Raw)
	}
	expect(t, cc.as(t, cc.approver, "POST", "/hr/core/actions/"+term.str("data.id")+"/rescind", body(map[string]any{"reason": "withdrawn resignation"})), 200, "CG9 rescind")
	if st := sqlStr(t, "SELECT ep.status || '/' || u.status FROM employee_profiles ep JOIN users u ON u.id = ep.user_id WHERE ep.id = ?", alice); st != "active/active" {
		t.Fatalf("CG9 status/login restored: %s", st)
	}
	if n := sqlStr(t, "SELECT count(*) FROM mappings WHERE user_id = ? AND is_primary AND status = 'active'", cc.memberUser); n != "1" {
		t.Fatalf("CG9 mapping restored: %s", n)
	}
	if p := cc.do(t, "GET", "/hr/core/positions/"+cc.p1); p.get("data.vacant") != false {
		t.Fatalf("CG9 seat re-occupied: %s", p.Raw)
	}

	// CG10 rehire after a real termination keeps the worker id and history.
	cc.complete(t, "terminate", alice, cc.day(-2), map[string]any{"termination_type": "involuntary", "last_working_day": cc.day(-1)})
	cc.complete(t, "rehire", alice, cc.day(0), map[string]any{"position_id": cc.shared, "fte": "0.50", "worker_type": "employee", "employment_type": "part_time"})
	w := cc.worker(t, alice, "")
	if w.str("data.current.position_id") != cc.shared || len(w.list("data.history")) < 4 {
		t.Fatalf("CG10 rehire: %s", w.Raw)
	}
	if st := sqlStr(t, "SELECT ep.status || '/' || u.status FROM employee_profiles ep JOIN users u ON u.id = ep.user_id WHERE ep.id = ?", alice); st != "probation/active" {
		t.Fatalf("CG10 status/login after rehire: %s", st)
	}
	// Shared seat: 0.50 + 0.50 fits, a further 0.25 does not.
	cc.complete(t, "hire", bob, cc.day(0), map[string]any{"position_id": cc.shared, "fte": "0.50", "worker_type": "contractor", "employment_type": "contract"})
	carolUser, _ := cc.newUser(t, "carol")
	carol := cc.do(t, "POST", "/hr/employees", body(empBody(carolUser, "CAR-"+strings.ToUpper(uniq()), nil))).str("data.id")
	expectErr(t, cc.action(t, "hire", carol, cc.day(0), map[string]any{"position_id": cc.shared, "fte": "0.25", "worker_type": "intern", "employment_type": "intern"}),
		409, "POSITION_FILLED", "CG7 shared budget")

	// CG3 the database refuses an overlapping assignment even if the service were bypassed.
	if err := testDB.Exec(`INSERT INTO core_job_assignments (id, tenant_id, employee_id, action_id, worker_type, employment_type, fte, effective_from)
		SELECT gen_random_uuid(), tenant_id, employee_id, action_id, worker_type, employment_type, fte, effective_from FROM core_job_assignments
		WHERE employee_id = ? AND status = 'active' AND effective_to IS NULL`, alice).Error; err == nil || !strings.Contains(err.Error(), "ex_core_job_assignments_overlap") {
		t.Fatalf("CG3 exclusion constraint: %v", err)
	}

	// CG12 back-fill: carol gets one assignment from her mapping; a second run skips her.
	expect(t, cc.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": carolUser, "department_id": cc.dept2, "is_primary": true})), 201, "carol mapping")
	bf := cc.do(t, "POST", "/hr/core/backfill")
	if expect(t, bf, 200, "backfill"); bf.get("data.processed").(float64) < 1 {
		t.Fatalf("CG12 backfill: %s", bf.Raw)
	}
	if again := cc.do(t, "POST", "/hr/core/backfill"); again.get("data.processed") != 0.0 {
		t.Fatalf("CG12 idempotent: %s", again.Raw)
	}
	if d := cc.worker(t, carol, "").str("data.current.department_id"); d != cc.dept2 {
		t.Fatalf("CG12 backfilled placement: %s", d)
	}

	// Headcount, assignments as of, self view, RBAC, isolation, privacy.
	expect(t, cc.do(t, "POST", "/hr/core/headcount-plans", body(map[string]any{"department_id": cc.dept1, "period_start": cc.day(-10),
		"period_end": cc.day(10), "planned_fte": 3})), 201, "plan")
	expectErr(t, cc.do(t, "POST", "/hr/core/headcount-plans", body(map[string]any{"department_id": cc.dept1, "period_start": cc.day(5),
		"period_end": cc.day(20), "planned_fte": 3})), 409, "CONFLICT", "overlapping plan")
	hc := cc.do(t, "GET", "/hr/core/headcount?department_id="+cc.dept1)
	if row := hc.list("data.departments")[0].(map[string]any); row["planned_fte"] != 3.0 || row["filled_fte"] != 1.0 || row["workers"] != 2.0 {
		t.Fatalf("CO-051 headcount: %v", row)
	}
	if as := cc.do(t, "GET", "/hr/core/assignments?position_id="+cc.p1+"&as_of="+cc.day(-100)); len(as.list("data")) != 1 {
		t.Fatalf("CG11 as-of org view: %s", as.Raw)
	}
	// Termination revoked alice's sessions; reactivation does not revive them, so she signs in again.
	aliceTok := login(t, cc.slug, sqlStr(t, "SELECT email FROM users WHERE id = ?", cc.memberUser), password).str("data.access_token")
	if me := cc.as(t, aliceTok, "GET", "/hr/core/workers/me"); me.str("data.employee_id") != alice {
		t.Fatalf("self view: %s", me.Raw)
	}
	expectErr(t, cc.as(t, aliceTok, "POST", "/hr/core/actions", body(map[string]any{})), 403, "FORBIDDEN", "member cannot initiate")
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", "/hr/core/actions/"+term.str("data.id")), 404, "NOT_FOUND", "CG1 foreign id")
	expectErr(t, call(t, "GET", "/hr/core/positions", host(cc.slug), token(other.token)), 403, "", "CG1 foreign token")
	for _, q := range []string{
		"SELECT count(*) FROM core_events_outbox WHERE tenant_id = ? AND payload::text LIKE '%SECRET%'",
		"SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'core.%' AND metadata::text LIKE '%SECRET%'",
	} {
		if n := sqlStr(t, q, cc.id); n != "0" {
			t.Fatalf("HR-009 leak: %s rows for %s", n, q)
		}
	}
}
