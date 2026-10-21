package services

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
)

// CG12 + CO-033: hire projects status, mapping and probation; a future hire waits for the sweep.
func TestGolden_HireAndEffects(t *testing.T) {
	e := newEnv(t)
	h := e.complete("hire", e.alice.ID, e.day(-200), e.hireChanges(e.p1))
	if h.Status != "completed" || !h.EffectsApplied {
		t.Fatalf("hire: %+v", h)
	}
	cur := e.current(e.alice.ID, "")
	if *cur.PositionID != e.p1 || *cur.DepartmentID != e.dept1 || *cur.LocationID != e.loc || *cur.GradeID != e.g1 || *cur.JobProfileID != e.profile {
		t.Fatalf("assignment from position and profile defaults: %+v", cur)
	}
	if e.w.emps[e.alice.ID].Status != "active" {
		t.Fatalf("probation over → active: %s", e.w.emps[e.alice.ID].Status)
	}
	if m := e.w.mappings[e.alice.UserID]; *m[0] != e.dept1 || *m[2] != e.mgr.UserID {
		t.Fatalf("mapping projection: %v", m)
	}
	// Future hire: completed now, effects later (DC-06).
	f := e.complete("hire", e.bob.ID, e.day(10), models.Changes{JobProfileID: &e.profile, DepartmentID: &e.dept2, WorkerType: sp("intern"), EmploymentType: sp("intern")})
	if f.EffectsApplied {
		t.Fatal("future hire must not apply effects yet")
	}
	if res, err := e.effects.Sweep(ctx, &e.tenant); err != nil || res.Processed != 0 {
		t.Fatalf("nothing due: %+v %v", res, err)
	}
	e.now = e.now.AddDate(0, 0, 10)
	if res, err := e.effects.Sweep(ctx, nil); err != nil || res.Processed != 1 {
		t.Fatalf("due now: %+v %v", res, err)
	}
	if e.w.emps[e.bob.ID].Status != "probation" {
		t.Fatalf("starts in probation: %s", e.w.emps[e.bob.ID].Status)
	}
	if res, _ := e.effects.Sweep(ctx, nil); res.Processed != 0 {
		t.Fatal("sweep is idempotent")
	}
	if _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.shared)); code_(err) != "ALREADY_ASSIGNED" {
		t.Fatalf("second hire: %v", err)
	}
}

// CG2/CG5: the action state graph at the service edge and self-approval.
func TestGolden_StateGraphAndSelfApproval(t *testing.T) {
	e := newEnv(t)
	d, err := e.draft("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1))
	must(t, err)
	_, err = e.acts.Approve(ctx, e.checker, d.ID, dto.DecisionRequest{})
	wantCode(t, err, "INVALID_STATE")
	upd, err := e.acts.Update(ctx, e.maker, d.ID, dto.UpdateActionRequest{EffectiveDate: sp(e.day(-1)), ReasonCode: sp("new_hire"), Comment: sp("x")})
	if must(t, err); upd.EffectiveDate != e.day(-1) || upd.ReasonCode != "new_hire" {
		t.Fatalf("draft edit: %+v", upd)
	}
	_, err = e.acts.Submit(ctx, e.maker, d.ID)
	must(t, err)
	_, err = e.acts.Update(ctx, e.maker, d.ID, dto.UpdateActionRequest{Comment: sp("late")})
	wantCode(t, err, "INVALID_STATE")
	_, err = e.acts.Approve(ctx, e.maker, d.ID, dto.DecisionRequest{})
	wantCode(t, err, "CORE_SELF_APPROVAL")
	_, err = e.acts.Reject(ctx, e.maker, d.ID, dto.DecisionRequest{})
	wantCode(t, err, "CORE_SELF_APPROVAL")
	r, err := e.acts.Reject(ctx, e.checker, d.ID, dto.DecisionRequest{Comment: sp("not budgeted")})
	if must(t, err); r.Status != "rejected" || *r.DecidedByUserID != e.checker.UserID {
		t.Fatalf("rejected: %+v", r)
	}
	_, err = e.acts.Cancel(ctx, e.maker, d.ID)
	wantCode(t, err, "INVALID_STATE")
	d2, err := e.draft("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1))
	must(t, err)
	c, err := e.acts.Cancel(ctx, e.maker, d2.ID)
	if must(t, err); c.Status != "cancelled" {
		t.Fatalf("cancel: %+v", c)
	}
	_, err = e.acts.Rescind(ctx, e.checker, d2.ID, dto.DecisionRequest{Reason: sp("x")})
	wantCode(t, err, "INVALID_STATE")
}

// CG6 + CO-034/CO-036: type rules, sequencing and probation.
func TestGolden_ChangeRules(t *testing.T) {
	e := newEnv(t)
	e.complete("hire", e.alice.ID, e.day(-100), e.hireChanges(e.p1))
	cases := []struct {
		typ, date string
		ch        models.Changes
		want      string
	}{
		{"promote", e.day(-100), models.Changes{GradeID: &e.g2}, "LATER_CHANGE_EXISTS"},
		{"promote", e.day(-50), models.Changes{}, "GRADE_RULE"},
		{"promote", e.day(-50), models.Changes{GradeID: &e.g1}, "GRADE_RULE"},
		{"demote", e.day(-50), models.Changes{GradeID: &e.g2}, "GRADE_RULE"},
		{"transfer", e.day(-50), models.Changes{DepartmentID: &e.dept1}, "NO_CHANGE"},
		{"change_job", e.day(-50), models.Changes{}, "NO_CHANGE"},
		{"change_manager", e.day(-50), models.Changes{ManagerEmployeeID: &e.mgr.ID}, "NO_CHANGE"},
		{"change_location", e.day(-50), models.Changes{LocationID: &e.loc}, "NO_CHANGE"},
		{"change_manager", e.day(-50), models.Changes{ManagerEmployeeID: &e.alice.ID}, "VALIDATION_ERROR"},
		{"transfer", e.day(-50), models.Changes{DepartmentID: &e.deptOff}, "INACTIVE_REFERENCE"},
		{"transfer", e.day(-50), models.Changes{DepartmentID: idp(uuid.New())}, "NOT_FOUND"},
		{"transfer", e.day(-50), models.Changes{FTE: fxp("1.5"), DepartmentID: &e.dept2}, "VALIDATION_ERROR"},
		{"transfer", e.day(-50), models.Changes{WorkerType: sp("robot"), DepartmentID: &e.dept2}, "VALIDATION_ERROR"},
		{"transfer", e.day(-50), models.Changes{EmploymentType: sp("gig"), DepartmentID: &e.dept2}, "VALIDATION_ERROR"},
		{"change_job", e.day(-50), models.Changes{PositionID: &e.shared, DepartmentID: &e.dept2}, "VALIDATION_ERROR"},
		{"probation_confirm", e.day(-50), models.Changes{}, "VALIDATION_ERROR"},
		{"probation_confirm", e.day(-50), models.Changes{ProbationOutcome: sp("extend")}, "VALIDATION_ERROR"},
		{"probation_confirm", e.day(-50), models.Changes{ProbationOutcome: sp("extend"), ProbationEndDate: sp(e.day(-60))}, "VALIDATION_ERROR"},
		{"probation_confirm", e.day(-50), models.Changes{ProbationOutcome: sp("extend"), ProbationEndDate: sp("x")}, "VALIDATION_ERROR"},
		{"terminate", e.day(-50), models.Changes{}, "VALIDATION_ERROR"},
		{"terminate", e.day(-50), models.Changes{TerminationType: sp("voluntary")}, "VALIDATION_ERROR"},
		{"terminate", e.day(-50), models.Changes{TerminationType: sp("voluntary"), LastWorkingDay: sp(e.day(-51))}, "VALIDATION_ERROR"},
		{"terminate", e.day(-50), models.Changes{TerminationType: sp("voluntary"), LastWorkingDay: sp(e.day(-40)), NoticeDate: sp("x")}, "VALIDATION_ERROR"},
		{"rehire", e.day(-50), e.hireChanges(e.shared), "NOT_TERMINATED"},
	}
	for _, c := range cases {
		_, err := e.run(c.typ, e.alice.ID, c.date, c.ch)
		if code_(err) != c.want {
			t.Errorf("%s %+v: want %s, got %v", c.typ, c.ch, c.want, err)
		}
	}
	e.complete("promote", e.alice.ID, e.day(-60), models.Changes{GradeID: &e.g2})
	e.complete("demote", e.alice.ID, e.day(-55), models.Changes{GradeID: &e.g1, GradeOverrideReason: sp("restructure")})
	e.complete("change_job", e.alice.ID, e.day(-50), models.Changes{PositionID: &e.shared, FTE: fxp("0.5")})
	e.complete("change_location", e.alice.ID, e.day(-45), models.Changes{LocationID: idp(e.addLocation())})
	e.complete("transfer", e.alice.ID, e.day(-40), models.Changes{ClearPosition: true, DepartmentID: &e.dept2})
	e.complete("change_manager", e.alice.ID, e.day(-35), models.Changes{ManagerEmployeeID: &e.bob.ID})
	ext := e.complete("probation_confirm", e.alice.ID, e.day(-30), models.Changes{ProbationOutcome: sp("extend"), ProbationEndDate: sp(e.day(30))})
	if c := e.current(e.alice.ID, ""); *c.ProbationEndDate != e.day(30) || e.w.emps[e.alice.ID].Status != "active" {
		t.Fatalf("extend keeps status, moves the end: %+v %s (%+v)", c, e.w.emps[e.alice.ID].Status, ext)
	}
	e.complete("probation_confirm", e.alice.ID, e.day(-20), models.Changes{ProbationOutcome: sp("confirm")})
	if c := e.current(e.alice.ID, ""); c.ProbationEndDate != nil || e.w.emps[e.alice.ID].Status != "active" {
		t.Fatalf("confirmed: %+v", c)
	}
	_, err := e.run("probation_confirm", e.alice.ID, e.day(-10), models.Changes{ProbationOutcome: sp("confirm")})
	wantCode(t, err, "PROBATION_STATE")
	_, err = e.run("transfer", e.carol.ID, e.day(-10), models.Changes{DepartmentID: &e.dept2})
	wantCode(t, err, "NO_ASSIGNMENT")
	w, err := e.workers.Worker(ctx, e.tenant, e.alice.ID, e.day(-56))
	if must(t, err); *w.Current.GradeID != e.g2 || len(w.History) != 9 {
		t.Fatalf("CG11 as-of and history: %+v (%d)", w.Current, len(w.History))
	}
}

func (e *env) addLocation() uuid.UUID {
	id := uuid.New()
	e.w.locs[id] = true
	return id
}

// CG7 + CO-012..CO-014: seats, budgets, freeze and close.
func TestGolden_Positions(t *testing.T) {
	e := newEnv(t)
	e.complete("hire", e.alice.ID, e.day(-30), e.hireChanges(e.p1))
	_, err := e.run("hire", e.bob.ID, e.day(-10), e.hireChanges(e.p1))
	wantCode(t, err, "POSITION_FILLED")
	e.complete("hire", e.bob.ID, e.day(-10), models.Changes{PositionID: &e.shared, FTE: fxp("0.6"), WorkerType: sp("employee"), EmploymentType: sp("full_time")})
	_, err = e.run("hire", e.carol.ID, e.day(-5), models.Changes{PositionID: &e.shared, FTE: fxp("0.5"), WorkerType: sp("employee"), EmploymentType: sp("part_time")})
	wantCode(t, err, "POSITION_FILLED")
	e.complete("hire", e.carol.ID, e.day(-5), models.Changes{PositionID: &e.shared, FTE: fxp("0.4"), WorkerType: sp("employee"), EmploymentType: sp("part_time")})
	_, err = e.pos.Update(ctx, e.maker, e.shared, dto.UpdatePositionRequest{BudgetedFTE: fxp("0.9")})
	wantCode(t, err, "POSITION_OVERFILLED")
	_, err = e.pos.Update(ctx, e.maker, e.shared, dto.UpdatePositionRequest{Shared: boolp(false)})
	wantCode(t, err, "POSITION_OVERFILLED")
	upd, err := e.pos.Update(ctx, e.maker, e.shared, dto.UpdatePositionRequest{Title: sp("Pool"), BudgetedFTE: fxp("2"), DepartmentID: sp(e.dept1.String()),
		JobProfileID: sp(e.profile.String()), LocationID: sp(e.loc.String())})
	if must(t, err); upd.FilledFTE != fx("1") || upd.OpenFTE != fx("1") || len(upd.Incumbents) != 2 || upd.Vacant {
		t.Fatalf("occupancy: %+v", upd)
	}
	for _, bad := range []dto.UpdatePositionRequest{{Title: sp(" ")}, {BudgetedFTE: fxp("0")}, {JobProfileID: sp("x")}, {DepartmentID: sp("x")}, {LocationID: sp("x")}} {
		_, err = e.pos.Update(ctx, e.maker, e.shared, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err = e.pos.SetStatus(ctx, e.maker, e.p1, "closed")
	wantCode(t, err, "POSITION_OCCUPIED")
	fr, err := e.pos.SetStatus(ctx, e.maker, e.p1, "frozen")
	if must(t, err); fr.Status != "frozen" {
		t.Fatalf("frozen: %+v", fr)
	}
	_, err = e.pos.SetStatus(ctx, e.maker, e.p1, "frozen")
	wantCode(t, err, "POSITION_STATE")
	p3 := e.position("P3", false, "1")
	_, err = e.pos.SetStatus(ctx, e.maker, p3, "frozen")
	must(t, err)
	_, err = e.run("hire", e.mgr.ID, e.day(0), models.Changes{PositionID: &p3, WorkerType: sp("employee"), EmploymentType: sp("full_time")})
	wantCode(t, err, "POSITION_STATE")
	cl, err := e.pos.SetStatus(ctx, e.maker, p3, "closed")
	if must(t, err); cl.Status != "closed" {
		t.Fatalf("closed: %+v", cl)
	}
	vac := true
	rows, meta, err := e.pos.List(ctx, e.tenant, repositories.PositionFilter{}, &vac, "", dto.ParsePage("", ""))
	if must(t, err); meta.TotalItems != 1 || rows[0].ID != p3 {
		t.Fatalf("vacant filter: %+v", rows)
	}
	old, err := e.pos.Get(ctx, e.tenant, e.p1, e.day(-31))
	if must(t, err); !old.Vacant {
		t.Fatalf("as-of before the hire: %+v", old)
	}
	all, meta, err := e.pos.List(ctx, e.tenant, repositories.PositionFilter{DepartmentID: &e.dept1}, nil, e.day(0), dto.ParsePage("1", "2"))
	if must(t, err); len(all) != 2 || meta.TotalItems != 3 {
		t.Fatalf("paged list: %d %+v", len(all), meta)
	}
	_, _, err = e.pos.List(ctx, e.tenant, repositories.PositionFilter{Status: "open"}, nil, "", dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
	_, _, err = e.pos.List(ctx, e.tenant, repositories.PositionFilter{}, nil, "bad", dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.pos.Get(ctx, e.tenant, uuid.New(), "")
	wantCode(t, err, "NOT_FOUND")
}

func boolp(b bool) *bool { return &b }

// CG8–CG10: terminate, rescind, rehire with every projection.
func TestGolden_TerminateRescindRehire(t *testing.T) {
	e := newEnv(t)
	e.complete("hire", e.alice.ID, e.day(-100), e.hireChanges(e.p1))
	term := e.complete("terminate", e.alice.ID, e.day(-5), models.Changes{TerminationType: sp("voluntary"), LastWorkingDay: sp(e.day(-1)), EligibleForRehire: boolp(true)})
	if !term.EffectsApplied || e.w.emps[e.alice.ID].Status != "terminated" || e.w.logins[e.alice.UserID] != "inactive" {
		t.Fatalf("CG8: %+v %s %s", term, e.w.emps[e.alice.ID].Status, e.w.logins[e.alice.UserID])
	}
	if p, _ := e.pos.Get(ctx, e.tenant, e.p1, ""); !p.Vacant {
		t.Fatal("CG8 seat freed after the last working day")
	}
	_, err := e.acts.Rescind(ctx, e.checker, term.ID, dto.DecisionRequest{})
	wantCode(t, err, "VALIDATION_ERROR")
	r, err := e.acts.Rescind(ctx, e.checker, term.ID, dto.DecisionRequest{Reason: sp("resignation withdrawn")})
	if must(t, err); r.Status != "rescinded" || e.w.emps[e.alice.ID].Status != "active" || e.w.logins[e.alice.UserID] != "active" {
		t.Fatalf("CG9 restored: %+v %s %s", r, e.w.emps[e.alice.ID].Status, e.w.logins[e.alice.UserID])
	}
	if c := e.current(e.alice.ID, ""); c == nil || c.EffectiveTo != nil {
		t.Fatalf("CG9 assignment open again: %+v", c)
	}
	// A seat taken after the termination blocks the rescind of the next termination (CO-014).
	term2 := e.complete("terminate", e.alice.ID, e.day(-3), models.Changes{TerminationType: sp("involuntary"), LastWorkingDay: sp(e.day(-2))})
	e.complete("hire", e.bob.ID, e.day(0), e.hireChanges(e.p1))
	_, err = e.acts.Rescind(ctx, e.checker, term2.ID, dto.DecisionRequest{Reason: sp("mistake")})
	wantCode(t, err, "POSITION_FILLED")
	reh := e.complete("rehire", e.alice.ID, e.day(0), models.Changes{JobProfileID: &e.profile, DepartmentID: &e.dept2, WorkerType: sp("employee"), EmploymentType: sp("full_time")})
	if e.w.emps[e.alice.ID].Status != "probation" || e.w.logins[e.alice.UserID] != "active" || !reh.EffectsApplied {
		t.Fatalf("CG10: %s %s", e.w.emps[e.alice.ID].Status, e.w.logins[e.alice.UserID])
	}
	_, err = e.acts.Rescind(ctx, e.checker, term2.ID, dto.DecisionRequest{Reason: sp("old")})
	wantCode(t, err, "NOT_LATEST")
	_, err = e.acts.Rescind(ctx, e.checker, reh.ID, dto.DecisionRequest{Reason: sp("rehire cancelled")})
	must(t, err)
	if e.w.emps[e.alice.ID].Status != "terminated" || e.w.logins[e.alice.UserID] != "inactive" {
		t.Fatalf("rescinded rehire: %s %s", e.w.emps[e.alice.ID].Status, e.w.logins[e.alice.UserID])
	}
	// A future termination completes now and waits for its day.
	e.complete("hire", e.carol.ID, e.day(-10), e.hireChanges(e.shared))
	ft := e.complete("terminate", e.carol.ID, e.day(0), models.Changes{TerminationType: sp("voluntary"), LastWorkingDay: sp(e.day(5))})
	if ft.EffectsApplied || e.w.emps[e.carol.ID].Status == "terminated" {
		t.Fatal("future termination must wait")
	}
	if res, _ := e.effects.Sweep(ctx, &e.tenant); res.Skipped == 0 || res.Processed != 0 {
		t.Fatalf("skipped until due: %+v", res)
	}
	e.now = e.now.AddDate(0, 0, 6)
	if res, _ := e.effects.Sweep(ctx, &e.tenant); res.Processed != 1 || e.w.emps[e.carol.ID].Status != "terminated" {
		t.Fatalf("applied when due: %+v", res)
	}
	// Rescinding a future change whose effects never ran restores only the assignment.
	e.complete("hire", e.mgr.ID, e.day(-1), models.Changes{JobProfileID: &e.profile, DepartmentID: &e.dept1, WorkerType: sp("employee"), EmploymentType: sp("full_time")})
	fut := e.complete("transfer", e.mgr.ID, e.day(20), models.Changes{DepartmentID: &e.dept2})
	_, err = e.acts.Rescind(ctx, e.checker, fut.ID, dto.DecisionRequest{Reason: sp("plan changed")})
	must(t, err)
	if c := e.current(e.mgr.ID, e.day(25)); *c.DepartmentID != e.dept1 {
		t.Fatalf("future change rescinded: %+v", c)
	}
}

// CO-001..CO-003: the job architecture.
func TestArchitecture(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []dto.CreateFamilyRequest{{Code: "x", Name: "N"}, {Code: "FAM", Name: " "}, {Code: "FAM", Name: "N", Description: sp(strings.Repeat("d", 2001))}} {
		_, err := e.arch.CreateFamily(ctx, e.maker, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err := e.arch.CreateFamily(ctx, e.maker, dto.CreateFamilyRequest{Code: "Eng", Name: "Dup"})
	wantCode(t, err, "CONFLICT")
	_, err = e.arch.CreateGrade(ctx, e.maker, dto.CreateGradeRequest{Code: "L9", Name: "N", Rank: 1})
	wantCode(t, err, "CONFLICT")
	for _, bad := range []dto.CreateGradeRequest{{Code: "-", Name: "N", Rank: 3}, {Code: "L3", Name: "", Rank: 3}, {Code: "L3", Name: "N", Rank: 0}} {
		_, err = e.arch.CreateGrade(ctx, e.maker, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	fam2, err := e.arch.UpdateFamily(ctx, e.maker, e.family, dto.UpdateArchitectureRequest{Name: sp("Eng & Data"), Description: sp("all"), Status: sp("inactive")})
	if must(t, err); fam2.Status != "inactive" || fam2.Name != "Eng & Data" {
		t.Fatalf("family update: %+v", fam2)
	}
	_, err = e.arch.CreateProfile(ctx, e.maker, dto.CreateProfileRequest{Code: "SRE", Title: "SRE", FamilyID: e.family.String(), Category: "staff"})
	wantCode(t, err, "INACTIVE_REFERENCE")
	_, err = e.arch.UpdateFamily(ctx, e.maker, e.family, dto.UpdateArchitectureRequest{Status: sp("active")})
	must(t, err)
	for _, bad := range []dto.CreateProfileRequest{
		{Code: "x", Title: "T", FamilyID: e.family.String(), Category: "staff"},
		{Code: "SRE", Title: "", FamilyID: e.family.String(), Category: "staff"},
		{Code: "SRE", Title: "T", FamilyID: "x", Category: "staff"},
		{Code: "SRE", Title: "T", FamilyID: e.family.String(), Category: "boss"},
		{Code: "SRE", Title: "T", FamilyID: e.family.String(), Category: "staff", ProbationDays: intp(400)},
		{Code: "SRE", Title: "T", FamilyID: e.family.String(), Category: "staff", DefaultGradeID: sp("x")},
		{Code: "SRE", Title: "T", FamilyID: e.family.String(), Category: "staff", Description: sp(strings.Repeat("d", 2001))},
	} {
		_, err = e.arch.CreateProfile(ctx, e.maker, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err = e.arch.CreateProfile(ctx, e.maker, dto.CreateProfileRequest{Code: "SRE", Title: "T", FamilyID: uuid.NewString(), Category: "staff"})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.arch.CreateProfile(ctx, e.maker, dto.CreateProfileRequest{Code: "SRE", Title: "T", FamilyID: e.family.String(), Category: "staff", DefaultGradeID: sp(uuid.NewString())})
	wantCode(t, err, "NOT_FOUND")
	g, err := e.arch.UpdateGrade(ctx, e.maker, e.g2, dto.UpdateArchitectureRequest{Name: sp("Senior"), Rank: intp(5), Status: sp("inactive")})
	if must(t, err); g.Rank != 5 || g.Status != "inactive" {
		t.Fatalf("grade update: %+v", g)
	}
	_, err = e.arch.UpdateGrade(ctx, e.maker, e.g2, dto.UpdateArchitectureRequest{Rank: intp(1)})
	wantCode(t, err, "CONFLICT")
	_, err = e.arch.CreateProfile(ctx, e.maker, dto.CreateProfileRequest{Code: "SRE", Title: "T", FamilyID: e.family.String(), Category: "staff", DefaultGradeID: sp(e.g2.String())})
	wantCode(t, err, "INACTIVE_REFERENCE")
	p, err := e.arch.UpdateProfile(ctx, e.maker, e.profile, dto.UpdateArchitectureRequest{Title: sp("Senior Engineer"), FamilyID: sp(e.family.String()),
		DefaultGradeID: sp(e.g1.String()), Category: sp("management"), ProbationDays: intp(0), Description: sp("d")})
	if must(t, err); p.ProbationDays != 0 || p.Category != "management" || p.Title != "Senior Engineer" {
		t.Fatalf("profile update: %+v", p)
	}
	for _, bad := range []dto.UpdateArchitectureRequest{{Title: sp(" ")}, {FamilyID: sp("x")}, {DefaultGradeID: sp("x")}, {Status: sp("gone")}, {Description: sp(strings.Repeat("d", 2001))}} {
		_, err = e.arch.UpdateProfile(ctx, e.maker, e.profile, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	for _, call := range []func() error{
		func() error {
			_, err := e.arch.UpdateFamily(ctx, e.maker, uuid.New(), dto.UpdateArchitectureRequest{})
			return err
		},
		func() error {
			_, err := e.arch.UpdateGrade(ctx, e.maker, uuid.New(), dto.UpdateArchitectureRequest{})
			return err
		},
		func() error {
			_, err := e.arch.UpdateProfile(ctx, e.maker, uuid.New(), dto.UpdateArchitectureRequest{})
			return err
		},
		func() error { _, err := e.arch.Profile(ctx, e.tenant, uuid.New()); return err },
	} {
		wantCode(t, call(), "NOT_FOUND")
	}
	_, err = e.arch.UpdateGrade(ctx, e.maker, e.g1, dto.UpdateArchitectureRequest{Name: sp("")})
	wantCode(t, err, "VALIDATION_ERROR")
	fams, _, err := e.arch.Families(ctx, e.tenant, "active", dto.ParsePage("", ""))
	if must(t, err); len(fams) != 1 {
		t.Fatalf("families: %d", len(fams))
	}
	grades, _, err := e.arch.Grades(ctx, e.tenant, "", dto.ParsePage("", ""))
	if must(t, err); len(grades) != 2 || grades[0].Rank != 1 {
		t.Fatalf("grades by rank: %+v", grades)
	}
	profs, _, err := e.arch.Profiles(ctx, e.tenant, repositories.ProfileFilter{FamilyID: &e.family, Category: "management"}, dto.ParsePage("", ""))
	if must(t, err); len(profs) != 1 {
		t.Fatalf("profiles: %d", len(profs))
	}
	got, err := e.arch.Profile(ctx, e.tenant, e.profile)
	if must(t, err); got.Code != "SWE" {
		t.Fatalf("profile: %+v", got)
	}
	for _, call := range []func() error{
		func() error { _, _, err := e.arch.Families(ctx, e.tenant, "x", dto.ParsePage("", "")); return err },
		func() error { _, _, err := e.arch.Grades(ctx, e.tenant, "x", dto.ParsePage("", "")); return err },
		func() error {
			_, _, err := e.arch.Profiles(ctx, e.tenant, repositories.ProfileFilter{Status: "x"}, dto.ParsePage("", ""))
			return err
		},
		func() error {
			_, _, err := e.arch.Profiles(ctx, e.tenant, repositories.ProfileFilter{Category: "x"}, dto.ParsePage("", ""))
			return err
		},
	} {
		wantCode(t, call(), "VALIDATION_ERROR")
	}
}

func intp(n int) *int { return &n }
