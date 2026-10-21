package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/models"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

var ctx = context.Background()

// env is one tenant: two active departments (and an inactive one), an active location, a job architecture, a single
// seat (p1) and a shared seat (budget 1.00); maker initiates, checker approves.
type env struct {
	t                          *testing.T
	st                         *store
	w                          *world
	aud                        *audit
	pub                        *publisher
	now                        time.Time
	tenant                     uuid.UUID
	arch                       *ArchitectureService
	pos                        *PositionService
	acts                       *ActionService
	workers                    *WorkerService
	effects                    *EffectsService
	relay                      *OutboxRelay
	maker, checker             Actor
	dept1, dept2, deptOff, loc uuid.UUID
	family, g1, g2, profile    uuid.UUID
	p1, shared                 uuid.UUID
	alice, bob, carol, mgr     *employeeDTO.EmployeeResponse
}

func newEnv(t *testing.T) *env {
	e := &env{t: t, st: newStore(), w: newWorld(), aud: &audit{}, pub: &publisher{}, tenant: uuid.New(),
		now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}
	d := Deps{Tx: fakeRunner{e.st}, Repo: fakeRepo{e.st}, Employees: e.w, Lifecycle: e.w, Org: e.w, Logins: e.w, Audit: e.aud, Publisher: e.pub,
		Now: func() time.Time { return e.now }}
	e.arch, e.pos, e.acts, e.workers = NewArchitectureService(d), NewPositionService(d), NewActionService(d), NewWorkerService(d)
	e.effects, e.relay = NewEffectsService(d), NewOutboxRelay(d)
	e.maker = Actor{TenantID: e.tenant, UserID: uuid.New(), CorrelationID: uuid.New()}
	e.checker = Actor{TenantID: e.tenant, UserID: uuid.New(), CorrelationID: uuid.New()}
	e.dept1, e.dept2, e.deptOff, e.loc = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	e.w.depts[e.dept1], e.w.depts[e.dept2], e.w.depts[e.deptOff], e.w.locs[e.loc] = true, true, false, true
	e.alice, e.bob, e.carol, e.mgr = e.w.add(e.tenant, "probation"), e.w.add(e.tenant, "active"), e.w.add(e.tenant, "active"), e.w.add(e.tenant, "active")
	f, err := e.arch.CreateFamily(ctx, e.maker, dto.CreateFamilyRequest{Code: "eng", Name: "Engineering"})
	must(t, err)
	g1, err := e.arch.CreateGrade(ctx, e.maker, dto.CreateGradeRequest{Code: "L1", Name: "Level 1", Rank: 1})
	must(t, err)
	g2, err := e.arch.CreateGrade(ctx, e.maker, dto.CreateGradeRequest{Code: "L2", Name: "Level 2", Rank: 2})
	must(t, err)
	e.family, e.g1, e.g2 = f.ID, g1.ID, g2.ID
	p, err := e.arch.CreateProfile(ctx, e.maker, dto.CreateProfileRequest{Code: "SWE", Title: "Engineer", FamilyID: f.ID.String(),
		DefaultGradeID: sp(g1.ID.String()), Category: "staff"})
	must(t, err)
	e.profile = p.ID
	e.p1 = e.position("P1", false, "1")
	e.shared = e.position("PS", true, "1")
	return e
}

func (e *env) position(c string, shared bool, budget string) uuid.UUID {
	e.t.Helper()
	b := fx(budget)
	p, err := e.pos.Create(ctx, e.maker, dto.CreatePositionRequest{Code: c, Title: c, JobProfileID: e.profile.String(), DepartmentID: e.dept1.String(),
		LocationID: sp(e.loc.String()), BudgetedFTE: &b, Shared: shared})
	must(e.t, err)
	return p.ID
}

func (e *env) day(n int) string { return fmtDate(dateOf(e.now).AddDate(0, 0, n)) }

func (e *env) hireChanges(pos uuid.UUID) models.Changes {
	return models.Changes{PositionID: &pos, ManagerEmployeeID: &e.mgr.ID, WorkerType: sp("employee"), EmploymentType: sp("full_time")}
}

// draft creates a draft action as the maker.
func (e *env) draft(typ string, emp uuid.UUID, date string, ch models.Changes) (*dto.ActionResponse, error) {
	return e.acts.Create(ctx, e.maker, dto.CreateActionRequest{Type: typ, EmployeeID: emp.String(), EffectiveDate: date, ReasonCode: "Test",
		Comment: sp("SECRET-COMMENT"), Changes: ch})
}

// run creates, submits (maker) and approves (checker); the first error is returned.
func (e *env) run(typ string, emp uuid.UUID, date string, ch models.Changes) (*dto.ActionResponse, error) {
	d, err := e.draft(typ, emp, date, ch)
	if err != nil {
		return nil, err
	}
	if _, err := e.acts.Submit(ctx, e.maker, d.ID); err != nil {
		return nil, err
	}
	return e.acts.Approve(ctx, e.checker, d.ID, dto.DecisionRequest{})
}

func (e *env) complete(typ string, emp uuid.UUID, date string, ch models.Changes) *dto.ActionResponse {
	e.t.Helper()
	r, err := e.run(typ, emp, date, ch)
	must(e.t, err)
	return r
}

func (e *env) current(emp uuid.UUID, asOf string) *dto.AssignmentResponse {
	e.t.Helper()
	w, err := e.workers.Worker(ctx, e.tenant, emp, asOf)
	must(e.t, err)
	return w.Current
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func code_(err error) string {
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
	if got := code_(err); got != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

func sp(s string) *string { return &s }

func fx(s string) kernel.Fixed {
	v, err := kernel.ParseFixed(s)
	if err != nil {
		panic(err)
	}
	return v
}

func fxp(s string) *kernel.Fixed { v := fx(s); return &v }

func idp(id uuid.UUID) *uuid.UUID { return &id }
