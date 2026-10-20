package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
)

// FR-CY001/002, PM-001, PM-002: cycle create, read, list and draft-only edits.
func TestCycles_CRUDAndStateGraph(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	for _, bad := range []dto.CreateCycleRequest{
		{Title: " ", StartDate: "2026-01-01", EndDate: "2026-02-01"},
		{Title: "x", StartDate: "2026-13-01", EndDate: "2026-02-01"},
		{Title: "x", StartDate: "2026-03-01", EndDate: "2026-02-01"},
		{Title: "x", StartDate: "2026-01-01", EndDate: "2026-02-01", Criteria: []string{"A", "a"}},
	} {
		_, err := e.cycles.Create(ctx, a, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	c, err := e.cycles.Create(ctx, a, dto.CreateCycleRequest{Title: " Q1 ", StartDate: "2026-01-01", EndDate: "2026-03-31", Criteria: []string{" Impact ", "Craft"}})
	must(t, err)
	if c.Title != "Q1" || len(c.Criteria) != 2 || c.Criteria[0] != "Impact" || c.Status != "draft" {
		t.Fatalf("create: %+v", c)
	}
	got, err := e.cycles.Get(ctx, e.tenant, c.ID)
	if must(t, err); got.EndDate != "2026-03-31" {
		t.Fatalf("get: %+v", got)
	}
	_, err = e.cycles.Get(ctx, uuid.New(), c.ID)
	wantCode(t, err, "NOT_FOUND") // EC-11 foreign tenant

	upd, err := e.cycles.Update(ctx, a, c.ID, dto.UpdateCycleRequest{Title: sp("Q1 FY26"), EndDate: sp("2026-04-30"), Criteria: &[]string{"Impact"}})
	if must(t, err); upd.Title != "Q1 FY26" || upd.EndDate != "2026-04-30" || len(upd.Criteria) != 1 {
		t.Fatalf("update: %+v", upd)
	}
	for _, bad := range []dto.UpdateCycleRequest{{Title: sp("")}, {StartDate: sp("2026-05-01")}, {Criteria: &[]string{}}, {Criteria: &[]string{"", "b"}}} {
		_, err = e.cycles.Update(ctx, a, c.ID, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err = e.cycles.Complete(ctx, a, c.ID)
	wantCode(t, err, "CYCLE_STATE") // PM-002: no skipping
	_, err = e.cycles.Activate(ctx, a, c.ID)
	must(t, err)
	_, err = e.cycles.Activate(ctx, a, c.ID)
	wantCode(t, err, "CYCLE_STATE")
	_, err = e.cycles.Update(ctx, a, c.ID, dto.UpdateCycleRequest{Title: sp("late")})
	wantCode(t, err, "CYCLE_STATE")
	_, err = e.cycles.Activate(ctx, a, uuid.New())
	wantCode(t, err, "NOT_FOUND")

	rows, meta, err := e.cycles.List(ctx, e.tenant, "active", dto.ParsePage("", ""))
	if must(t, err); len(rows) != 1 || meta.TotalItems != 1 {
		t.Fatalf("list active: %d", len(rows))
	}
	_, _, err = e.cycles.List(ctx, e.tenant, "closed", dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.cycles.NineBox(ctx, e.tenant, uuid.New())
	wantCode(t, err, "NOT_FOUND")
}

// FR-RV001/002, PM-011: reviewer assignment and review lists.
func TestReviews_AssignAndLists(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	cy := e.cycle("active")
	assign := func(subject, reviewer string) error {
		_, err := e.reviews.Assign(ctx, a, dto.AssignReviewerRequest{CycleID: cy.String(), EmployeeID: subject, ReviewerEmployeeID: reviewer, Type: "peer"})
		return err
	}
	wantCode(t, assign(e.alice.ID.String(), e.carol.ID.String()), "CYCLE_STATE")
	_, err := e.cycles.LaunchReviews(ctx, a, cy)
	must(t, err)
	wantCode(t, assign("nope", e.carol.ID.String()), "VALIDATION_ERROR")
	wantCode(t, assign(e.alice.ID.String(), "nope"), "VALIDATION_ERROR")
	wantCode(t, assign(e.alice.ID.String(), e.alice.ID.String()), "VALIDATION_ERROR")
	wantCode(t, assign(uuid.NewString(), e.carol.ID.String()), "EMPLOYEE_NOT_FOUND")
	wantCode(t, assign(e.alice.ID.String(), e.ppl.add(e.tenant, "exited").ID.String()), "EMPLOYEE_STATE")
	_, err = e.reviews.Assign(ctx, a, dto.AssignReviewerRequest{CycleID: "x", EmployeeID: e.alice.ID.String(), ReviewerEmployeeID: e.carol.ID.String(), Type: "peer"})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.reviews.Assign(ctx, a, dto.AssignReviewerRequest{CycleID: uuid.NewString(), EmployeeID: e.alice.ID.String(), ReviewerEmployeeID: e.carol.ID.String(), Type: "peer"})
	wantCode(t, err, "NOT_FOUND")
	must(t, assign(e.alice.ID.String(), e.carol.ID.String()))
	wantCode(t, assign(e.alice.ID.String(), e.carol.ID.String()), "CONFLICT")

	rows, _, err := e.reviews.List(ctx, e.tenant, repositories.ReviewFilter{CycleID: &cy, EmployeeID: &e.alice.ID}, dto.ParsePage("", ""))
	if must(t, err); len(rows) != 1 || rows[0].ReviewerEmployeeID == nil {
		t.Fatalf("admin list shows reviewers: %+v", rows)
	}
	_, _, err = e.reviews.List(ctx, e.tenant, repositories.ReviewFilter{Status: "done"}, dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
	_, _, err = e.reviews.ListAssigned(ctx, e.as(e.carol), "done", dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
	_, _, err = e.reviews.ListAssigned(ctx, Actor{TenantID: e.tenant, UserID: uuid.New()}, "", dto.ParsePage("", ""))
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.reviews.ListMine(ctx, Actor{TenantID: e.tenant, UserID: uuid.New()}, nil)
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	reader := e.as(e.carol)
	reader.CanRead = true
	_, err = e.reviews.Get(ctx, reader, rows[0].ID)
	must(t, err)
	_, err = e.reviews.Get(ctx, a, uuid.New())
	wantCode(t, err, "NOT_FOUND")
	_, err = e.reviews.Submit(ctx, e.as(e.carol), rows[0].ID, dto.SubmitReviewRequest{Strengths: sp(string(make([]byte, 5001)))})
	wantCode(t, err, "VALIDATION_ERROR")
}

// Goal input validation (FR-GL001, FR-GL005).
func TestGoals_Validation(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	me := e.as(e.alice)
	for _, bad := range []dto.CreateGoalRequest{
		{CycleID: "x", Title: "t", Weight: fx("1")},
		{CycleID: cy.String(), Title: " ", Weight: fx("1")},
		{CycleID: cy.String(), Title: "t", Weight: 0},
		{CycleID: cy.String(), Title: "t", Weight: fx("100.01")},
		{CycleID: cy.String(), Title: "t", Weight: fx("1"), ParentGoalID: sp("x")},
		{CycleID: cy.String(), Title: "t", Weight: fx("1"), EmployeeID: sp("x")},
		{CycleID: cy.String(), Title: "t", Weight: fx("1"), Description: sp(string(make([]rune, 5001)))},
	} {
		_, err := e.goals.Create(ctx, me, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err := e.goals.Create(ctx, Actor{TenantID: e.tenant, UserID: uuid.New()}, dto.CreateGoalRequest{CycleID: cy.String(), Title: "t", Weight: fx("1")})
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.goals.Create(ctx, e.admin(), dto.CreateGoalRequest{CycleID: cy.String(), EmployeeID: sp(uuid.NewString()), Title: "t", Weight: fx("1")})
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.goals.Create(ctx, me, dto.CreateGoalRequest{CycleID: uuid.NewString(), Title: "t", Weight: fx("1")})
	wantCode(t, err, "NOT_FOUND")
	g := e.goal(me, cy, e.alice, "10")
	for _, bad := range []dto.KeyResultRequest{{Title: "", TargetValue: fx("1")}, {Title: "t", Unit: sp("a-very-long-unit-name-x"), TargetValue: fx("1")}, {Title: "t"}} {
		_, err = e.goals.AddKeyResult(ctx, me, g.ID, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	for _, bad := range []dto.UpdateGoalRequest{{Title: sp("")}, {Weight: fxp("0")}, {Description: sp(string(make([]rune, 5001)))}} {
		_, err = e.goals.Update(ctx, me, g.ID, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err = e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{Progress: fxp("1"), Note: sp(string(make([]rune, 1001)))})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{KeyResultID: sp("x"), Value: fxp("1")})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.goals.SetStatus(ctx, me, g.ID, "completed")
	must(t, err)
	_, err = e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{Progress: fxp("1")})
	wantCode(t, err, "GOAL_STATE")
	_, err = e.goals.AddKeyResult(ctx, me, g.ID, dto.KeyResultRequest{Title: "t", TargetValue: fx("1")})
	wantCode(t, err, "GOAL_STATE")
	_, err = e.goals.SetStatus(ctx, me, g.ID, "active")
	must(t, err) // completed → active
	_, err = e.goals.Get(ctx, me, uuid.New())
	wantCode(t, err, "NOT_FOUND")
	_, err = e.goals.SetStatus(ctx, me, uuid.New(), "completed")
	wantCode(t, err, "NOT_FOUND")
}

// FR-EV001: the relay republishes rows the request path could not publish.
func TestOutboxRelay(t *testing.T) {
	e := newEnv(t)
	e.pub.err = errBoom
	e.cycle("active") // cycle_started stays unpublished
	e.pub.err = nil
	n, err := e.relay.RelayOnce(ctx)
	if must(t, err); n != 1 {
		t.Fatalf("relayed %d", n)
	}
	e.pub.err = errBoom
	e.cycle("active")
	if n, _ = e.relay.RelayOnce(ctx); n != 0 {
		t.Fatalf("failed publish is not counted: %d", n)
	}
	e.st.fail["outbox.fetch"] = errBoom
	if _, err := e.relay.RelayOnce(ctx); err == nil {
		t.Fatal("fetch error surfaces")
	}
	c, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	e.relay.interval = time.Millisecond
	e.relay.Run(c) // returns on cancellation
}
