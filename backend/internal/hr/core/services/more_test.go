package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
)

// CO-021, CO-022, CO-050, CO-051, CO-042: worker views, as-of org view, headcount and back-fill.
func TestWorkersHeadcountBackfill(t *testing.T) {
	e := newEnv(t)
	e.complete("hire", e.alice.ID, e.day(-30), e.hireChanges(e.p1))
	me, err := e.workers.Me(ctx, Actor{TenantID: e.tenant, UserID: e.alice.UserID}, "")
	if must(t, err); me.Current == nil || me.EmployeeID != e.alice.ID || len(me.Actions) != 1 {
		t.Fatalf("me: %+v", me)
	}
	_, err = e.workers.Me(ctx, Actor{TenantID: e.tenant, UserID: uuid.New()}, "")
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.workers.Worker(ctx, e.tenant, uuid.New(), "")
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.workers.Worker(ctx, e.tenant, e.alice.ID, "2026-13-01")
	wantCode(t, err, "VALIDATION_ERROR")
	rows, meta, err := e.workers.Assignments(ctx, e.tenant, e.day(-1), repositories.AssignmentFilter{PositionID: &e.p1}, dto.ParsePage("", ""))
	if must(t, err); meta.TotalItems != 1 || rows[0].EmployeeID != e.alice.ID {
		t.Fatalf("as-of view: %+v", rows)
	}
	if rows, _, _ = e.workers.Assignments(ctx, e.tenant, e.day(-31), repositories.AssignmentFilter{}, dto.ParsePage("", "")); len(rows) != 0 {
		t.Fatal("nobody before the hire")
	}
	_, _, err = e.workers.Assignments(ctx, e.tenant, "x", repositories.AssignmentFilter{}, dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")

	plan, err := e.workers.CreatePlan(ctx, e.maker, dto.CreatePlanRequest{DepartmentID: e.dept1.String(), PeriodStart: e.day(-10), PeriodEnd: e.day(10), PlannedFTE: fx("3")})
	if must(t, err); plan.PlannedFTE != fx("3") {
		t.Fatalf("plan: %+v", plan)
	}
	for _, bad := range []dto.CreatePlanRequest{
		{DepartmentID: "x", PeriodStart: e.day(0), PeriodEnd: e.day(1)},
		{DepartmentID: e.dept2.String(), PeriodStart: "x", PeriodEnd: e.day(1)},
		{DepartmentID: e.dept2.String(), PeriodStart: e.day(0), PeriodEnd: "x"},
		{DepartmentID: e.dept2.String(), PeriodStart: e.day(2), PeriodEnd: e.day(1)},
		{DepartmentID: e.dept2.String(), PeriodStart: e.day(0), PeriodEnd: e.day(1), PlannedFTE: fx("-1")},
	} {
		_, err = e.workers.CreatePlan(ctx, e.maker, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err = e.workers.CreatePlan(ctx, e.maker, dto.CreatePlanRequest{DepartmentID: e.dept1.String(), PeriodStart: e.day(5), PeriodEnd: e.day(20)})
	wantCode(t, err, "CONFLICT")
	_, err = e.workers.CreatePlan(ctx, e.maker, dto.CreatePlanRequest{DepartmentID: e.deptOff.String(), PeriodStart: e.day(0), PeriodEnd: e.day(1)})
	wantCode(t, err, "INACTIVE_REFERENCE")
	plans, _, err := e.workers.Plans(ctx, e.tenant, &e.dept1, dto.ParsePage("", ""))
	if must(t, err); len(plans) != 1 {
		t.Fatalf("plans: %d", len(plans))
	}
	hc, err := e.workers.Headcount(ctx, e.tenant, nil, "")
	must(t, err)
	if len(hc.Departments) != 1 || hc.Departments[0].Workers != 1 || hc.Departments[0].FilledFTE != fx("1") || *hc.Departments[0].PlannedFTE != fx("3") ||
		hc.Departments[0].OpenPositions != 1 || hc.Departments[0].BudgetedFTE != fx("2") {
		t.Fatalf("headcount: %+v", hc.Departments)
	}
	_, err = e.workers.Headcount(ctx, e.tenant, nil, "x")
	wantCode(t, err, "VALIDATION_ERROR")

	// Back-fill: bob has a mapping and a manager; carol has neither; mgr is terminated (skipped); alice is assigned.
	e.w.mappings[e.bob.UserID] = [3]*uuid.UUID{&e.dept2, &e.loc, &e.alice.UserID}
	e.w.emps[e.mgr.ID].Status = "terminated"
	res, err := e.workers.Backfill(ctx, e.maker)
	if must(t, err); res.Processed != 2 || res.Skipped != 2 {
		t.Fatalf("backfill: %+v", res)
	}
	b := e.current(e.bob.ID, "")
	if *b.DepartmentID != e.dept2 || *b.ManagerEmployeeID != e.alice.ID || b.EmploymentType != "part_time" || b.EffectiveFrom != "2025-02-01" {
		t.Fatalf("bob backfilled: %+v", b)
	}
	if res, _ = e.workers.Backfill(ctx, e.maker); res.Processed != 0 {
		t.Fatalf("idempotent: %+v", res)
	}
}

// Action input validation, list and get.
func TestActionsInputAndLists(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []dto.CreateActionRequest{
		{Type: "fire", EmployeeID: e.alice.ID.String(), EffectiveDate: e.day(0), ReasonCode: "x"},
		{Type: "hire", EmployeeID: "x", EffectiveDate: e.day(0), ReasonCode: "x"},
		{Type: "hire", EmployeeID: e.alice.ID.String(), EffectiveDate: "x", ReasonCode: "x"},
		{Type: "hire", EmployeeID: e.alice.ID.String(), EffectiveDate: e.day(0), ReasonCode: " "},
		{Type: "hire", EmployeeID: e.alice.ID.String(), EffectiveDate: e.day(0), ReasonCode: "x", Comment: sp(string(make([]rune, 2001)))},
	} {
		_, err := e.acts.Create(ctx, e.maker, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err := e.acts.Create(ctx, e.maker, dto.CreateActionRequest{Type: "hire", EmployeeID: uuid.NewString(), EffectiveDate: e.day(0), ReasonCode: "x"})
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	d, err := e.draft("hire", e.alice.ID, e.day(0), models.Changes{WorkerType: sp("employee")})
	must(t, err)
	_, err = e.acts.Submit(ctx, e.maker, d.ID)
	wantCode(t, err, "VALIDATION_ERROR") // missing profile / department
	_, err = e.acts.Update(ctx, e.maker, d.ID, dto.UpdateActionRequest{Changes: &models.Changes{JobProfileID: &e.profile, DepartmentID: &e.dept1,
		WorkerType: sp("employee"), EmploymentType: sp("full_time"), GradeID: &e.g2}})
	must(t, err)
	_, err = e.acts.Update(ctx, e.maker, d.ID, dto.UpdateActionRequest{EffectiveDate: sp("x")})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.acts.Update(ctx, e.maker, uuid.New(), dto.UpdateActionRequest{})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.acts.Approve(ctx, e.checker, d.ID, dto.DecisionRequest{Comment: sp(string(make([]rune, 1001)))})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.acts.Reject(ctx, e.checker, d.ID, dto.DecisionRequest{Comment: sp(string(make([]rune, 1001)))})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.acts.Rescind(ctx, e.checker, d.ID, dto.DecisionRequest{Reason: sp(" ")})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.acts.Submit(ctx, e.maker, d.ID)
	must(t, err)
	got, err := e.acts.Approve(ctx, e.checker, d.ID, dto.DecisionRequest{Comment: sp("ok")})
	if must(t, err); *e.current(e.alice.ID, "").GradeID != e.g2 || *got.DecisionComment != "ok" {
		t.Fatalf("explicit grade wins: %+v", got)
	}
	one, err := e.acts.Get(ctx, e.tenant, d.ID)
	if must(t, err); one.Status != "completed" {
		t.Fatalf("get: %+v", one)
	}
	_, err = e.acts.Get(ctx, e.tenant, uuid.New())
	wantCode(t, err, "NOT_FOUND")
	rows, meta, err := e.acts.List(ctx, e.tenant, repositories.ActionFilter{EmployeeID: &e.alice.ID, Status: "completed", Type: "hire"}, dto.ParsePage("", ""))
	if must(t, err); meta.TotalItems != 1 || rows[0].ID != d.ID {
		t.Fatalf("list: %+v", rows)
	}
	for _, f := range []repositories.ActionFilter{{Status: "x"}, {Type: "x"}} {
		_, _, err = e.acts.List(ctx, e.tenant, f, dto.ParsePage("", ""))
		wantCode(t, err, "VALIDATION_ERROR")
	}
	// Position create validation.
	for _, bad := range []dto.CreatePositionRequest{
		{Code: "x", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String()},
		{Code: "PX", Title: " ", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String()},
		{Code: "PX", Title: "T", JobProfileID: "x", DepartmentID: e.dept1.String()},
		{Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: "x"},
		{Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String(), LocationID: sp("x")},
		{Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String(), BudgetedFTE: fxp("100")},
	} {
		_, err = e.pos.Create(ctx, e.maker, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	for c, req := range map[string]dto.CreatePositionRequest{
		"CONFLICT":           {Code: "P1", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String()},
		"NOT_FOUND":          {Code: "PX", Title: "T", JobProfileID: uuid.NewString(), DepartmentID: e.dept1.String()},
		"INACTIVE_REFERENCE": {Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.deptOff.String()},
	} {
		_, err = e.pos.Create(ctx, e.maker, req)
		wantCode(t, err, c)
	}
	e.w.locs[e.loc] = false
	_, err = e.pos.Create(ctx, e.maker, dto.CreatePositionRequest{Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String(), LocationID: sp(e.loc.String())})
	wantCode(t, err, "INACTIVE_REFERENCE")
	_, err = e.pos.Create(ctx, e.maker, dto.CreatePositionRequest{Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String(), LocationID: sp(uuid.NewString())})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.pos.Create(ctx, e.maker, dto.CreatePositionRequest{Code: "PX", Title: "T", JobProfileID: e.profile.String(), DepartmentID: uuid.NewString()})
	wantCode(t, err, "NOT_FOUND")
}

// The outbox relay and the background loops.
func TestRelayAndLoops(t *testing.T) {
	e := newEnv(t)
	e.pub.err = errBoom
	e.complete("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1))
	e.pub.err = nil
	n, err := e.relay.RelayOnce(ctx)
	if must(t, err); n == 0 {
		t.Fatal("relay publishes pending rows")
	}
	e.pub.err = errBoom
	e.complete("change_manager", e.alice.ID, e.day(1), models.Changes{ManagerEmployeeID: &e.bob.ID})
	if n, _ = e.relay.RelayOnce(ctx); n != 0 {
		t.Fatal("failures are not counted")
	}
	e.st.fail["fetch"] = errBoom
	if _, err := e.relay.RelayOnce(ctx); err == nil {
		t.Fatal("fetch error")
	}
	c, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	e.relay.interval = time.Millisecond
	e.relay.Run(c)
	c2, cancel2 := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel2()
	e.effects.Run(c2, time.Millisecond)
}

// Infrastructure failures propagate and roll back.
func TestFailures(t *testing.T) {
	cases := []struct {
		name string
		run  func(e *env) error
	}{
		{"code", func(e *env) error {
			_, err := e.arch.CreateFamily(ctx, e.maker, dto.CreateFamilyRequest{Code: "OPS", Name: "Ops"})
			return err
		}},
		{"rank", func(e *env) error {
			_, err := e.arch.CreateGrade(ctx, e.maker, dto.CreateGradeRequest{Code: "L3", Name: "L3", Rank: 3})
			return err
		}},
		{"families", func(e *env) error { _, _, err := e.arch.Families(ctx, e.tenant, "", dto.ParsePage("", "")); return err }},
		{"grades", func(e *env) error { _, _, err := e.arch.Grades(ctx, e.tenant, "", dto.ParsePage("", "")); return err }},
		{"profiles", func(e *env) error {
			_, _, err := e.arch.Profiles(ctx, e.tenant, repositories.ProfileFilter{}, dto.ParsePage("", ""))
			return err
		}},
		{"save", func(e *env) error {
			_, err := e.arch.UpdateFamily(ctx, e.maker, e.family, dto.UpdateArchitectureRequest{Name: sp("x")})
			return err
		}},
		{"save", func(e *env) error {
			_, err := e.arch.UpdateGrade(ctx, e.maker, e.g1, dto.UpdateArchitectureRequest{Rank: intp(9)})
			return err
		}},
		{"rank", func(e *env) error {
			_, err := e.arch.UpdateGrade(ctx, e.maker, e.g1, dto.UpdateArchitectureRequest{Rank: intp(9)})
			return err
		}},
		{"save", func(e *env) error {
			_, err := e.arch.UpdateProfile(ctx, e.maker, e.profile, dto.UpdateArchitectureRequest{})
			return err
		}},
		{"positions", func(e *env) error {
			_, _, err := e.pos.List(ctx, e.tenant, repositories.PositionFilter{}, nil, "", dto.ParsePage("", ""))
			return err
		}},
		{"positions", func(e *env) error {
			v := true
			_, _, err := e.pos.List(ctx, e.tenant, repositories.PositionFilter{}, &v, "", dto.ParsePage("", ""))
			return err
		}},
		{"positionAssignments", func(e *env) error { _, err := e.pos.Get(ctx, e.tenant, e.p1, ""); return err }},
		{"lockPosition", func(e *env) error {
			_, err := e.pos.Update(ctx, e.maker, e.p1, dto.UpdatePositionRequest{})
			return err
		}},
		{"lockPosition", func(e *env) error { _, err := e.pos.SetStatus(ctx, e.maker, e.p1, "frozen"); return err }},
		{"positionAssignments", func(e *env) error { _, err := e.pos.SetStatus(ctx, e.maker, e.p1, "closed"); return err }},
		{"create", func(e *env) error {
			_, err := e.pos.Create(ctx, e.maker, dto.CreatePositionRequest{Code: "PZ", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String()})
			return err
		}},
		{"dept", func(e *env) error {
			_, err := e.pos.Create(ctx, e.maker, dto.CreatePositionRequest{Code: "PZ", Title: "T", JobProfileID: e.profile.String(), DepartmentID: e.dept1.String()})
			return err
		}},
		{"create.action", func(e *env) error { _, err := e.draft("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"lockAction", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"completedActions", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"workerAssignments", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"profile", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"position", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"employees", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"lifecycle", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"project", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"create.assignment", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"outbox", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"lockWorker", func(e *env) error { _, err := e.run("hire", e.alice.ID, e.day(0), e.hireChanges(e.p1)); return err }},
		{"actions", func(e *env) error {
			_, _, err := e.acts.List(ctx, e.tenant, repositories.ActionFilter{}, dto.ParsePage("", ""))
			return err
		}},
		{"asOf", func(e *env) error { _, err := e.workers.Headcount(ctx, e.tenant, nil, ""); return err }},
		{"plans", func(e *env) error { _, err := e.workers.Headcount(ctx, e.tenant, nil, ""); return err }},
		{"plans", func(e *env) error {
			_, _, err := e.workers.Plans(ctx, e.tenant, nil, dto.ParsePage("", ""))
			return err
		}},
		{"planOverlaps", func(e *env) error {
			_, err := e.workers.CreatePlan(ctx, e.maker, dto.CreatePlanRequest{DepartmentID: e.dept1.String(), PeriodStart: e.day(0), PeriodEnd: e.day(1)})
			return err
		}},
		{"asOf", func(e *env) error {
			_, _, err := e.workers.Assignments(ctx, e.tenant, "", repositories.AssignmentFilter{}, dto.ParsePage("", ""))
			return err
		}},
		{"actions", func(e *env) error { _, err := e.workers.Worker(ctx, e.tenant, e.alice.ID, ""); return err }},
		{"assignedWorkers", func(e *env) error { _, err := e.workers.Backfill(ctx, e.maker); return err }},
		{"list", func(e *env) error { _, err := e.workers.Backfill(ctx, e.maker); return err }},
		{"placement", func(e *env) error { _, err := e.workers.Backfill(ctx, e.maker); return err }},
		{"pending", func(e *env) error { _, err := e.effects.Sweep(ctx, nil); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.st.fail[tc.name] = errBoom
			e.w.fail[tc.name] = errBoom
			if err := tc.run(e); !errors.Is(err, errBoom) {
				t.Fatalf("%s: want boom, got %v", tc.name, err)
			}
		})
	}
}

// Failures while reversing effects and in the sweep roll the whole operation back.
func TestFailures_ReverseAndSweep(t *testing.T) {
	for _, op := range []string{"lifecycle", "logins", "project", "save.assignment", "assignment"} {
		t.Run(op, func(t *testing.T) {
			e := newEnv(t)
			e.complete("hire", e.alice.ID, e.day(-100), e.hireChanges(e.p1))
			term := e.complete("terminate", e.alice.ID, e.day(-5), models.Changes{TerminationType: sp("voluntary"), LastWorkingDay: sp(e.day(-1))})
			e.st.fail[op], e.w.fail[op] = errBoom, errBoom
			if _, err := e.acts.Rescind(ctx, e.checker, term.ID, dto.DecisionRequest{Reason: sp("x")}); !errors.Is(err, errBoom) {
				t.Fatalf("want boom, got %v", err)
			}
			if e.st.actions[term.ID].Status != "completed" {
				t.Fatal("rolled back")
			}
		})
	}
	e := newEnv(t)
	e.complete("hire", e.alice.ID, e.day(5), e.hireChanges(e.p1))
	e.now = e.now.AddDate(0, 0, 5)
	e.w.fail["lifecycle"] = errBoom
	if _, err := e.effects.Sweep(ctx, nil); !errors.Is(err, errBoom) {
		t.Fatalf("sweep failure: %v", err)
	}
}
