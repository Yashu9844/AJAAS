package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/score"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

var ctx = context.Background()

// env is one tenant with services over the fake store; hr holds performance:manage + read.
type env struct {
	t                          *testing.T
	st                         *store
	ppl                        *people
	aud                        *audit
	pub                        *publisher
	tenant                     uuid.UUID
	cycles                     *CycleService
	goals                      *GoalService
	reviews                    *ReviewService
	relay                      *OutboxRelay
	hr, mgr, alice, bob, carol *employeeDTO.EmployeeResponse
}

// newEnv: alice and bob report to mgr; carol is a colleague with no relationship; hr administers.
func newEnv(t *testing.T) *env {
	e := &env{t: t, st: newStore(), ppl: newPeople(), aud: &audit{}, pub: &publisher{}, tenant: uuid.New()}
	clock := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	d := Deps{Tx: fakeRunner{e.st}, Repos: Repos{Cycles: fakeCycles{e.st}, Goals: fakeGoals{e.st}, Reviews: fakeReviews{e.st}, Outbox: fakeOutbox{e.st}},
		Employees: e.ppl, Managers: e.ppl, Audit: e.aud, Publisher: e.pub, Now: func() time.Time { clock = clock.Add(time.Minute); return clock }}
	e.cycles, e.goals, e.reviews, e.relay = NewCycleService(d), NewGoalService(d), NewReviewService(d), NewOutboxRelay(d)
	e.hr, e.mgr = e.ppl.add(e.tenant, "active"), e.ppl.add(e.tenant, "active")
	e.alice, e.bob, e.carol = e.ppl.add(e.tenant, "active"), e.ppl.add(e.tenant, "probation"), e.ppl.add(e.tenant, "active")
	e.ppl.managers[e.alice.UserID] = e.mgr.UserID
	e.ppl.managers[e.bob.UserID] = e.mgr.UserID
	return e
}

func (e *env) as(emp *employeeDTO.EmployeeResponse) Actor {
	return Actor{TenantID: e.tenant, UserID: emp.UserID, CorrelationID: uuid.New()}
}

func (e *env) admin() Actor {
	a := e.as(e.hr)
	a.CanRead, a.CanManage = true, true
	return a
}

func fx(s string) score.Fixed {
	v, err := score.Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func fxp(s string) *score.Fixed { v := fx(s); return &v }

func sp(s string) *string { return &s }

// cycle creates a cycle and advances it to status.
func (e *env) cycle(status string) uuid.UUID {
	e.t.Helper()
	c, err := e.cycles.Create(ctx, e.admin(), dto.CreateCycleRequest{Title: "FY26 H1", StartDate: "2026-01-01", EndDate: "2026-06-30"})
	must(e.t, err)
	steps := map[string][]func() error{
		"draft":  nil,
		"active": {func() error { _, err := e.cycles.Activate(ctx, e.admin(), c.ID); return err }},
	}
	steps["review"] = append(steps["active"], func() error { _, err := e.cycles.LaunchReviews(ctx, e.admin(), c.ID); return err })
	steps["completed"] = append(steps["review"], func() error { _, err := e.cycles.Complete(ctx, e.admin(), c.ID); return err })
	for _, step := range steps[status] {
		must(e.t, step())
	}
	return c.ID
}

func (e *env) goal(a Actor, cycleID uuid.UUID, owner *employeeDTO.EmployeeResponse, weight string) *dto.GoalResponse {
	e.t.Helper()
	g, err := e.goals.Create(ctx, a, dto.CreateGoalRequest{CycleID: cycleID.String(), EmployeeID: sp(owner.ID.String()), Title: "Ship it", Weight: fx(weight)})
	must(e.t, err)
	return g
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// code returns the AppError code of err ("" for nil, "?" for a non-AppError).
func code(err error) string {
	if err == nil {
		return ""
	}
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return "?"
}

func wantCode(t *testing.T, err error, want string) {
	t.Helper()
	if got := code(err); got != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

// reviewOf finds the review of emp with the given type.
func (e *env) reviewOf(emp *employeeDTO.EmployeeResponse, typ string) uuid.UUID {
	e.t.Helper()
	for id, r := range e.st.reviews {
		if r.EmployeeID == emp.ID && r.Type == typ {
			return id
		}
	}
	e.t.Fatalf("no %s review of %s", typ, emp.ID)
	return uuid.Nil
}
