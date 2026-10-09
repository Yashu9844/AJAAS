package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	leavecalc "github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

type fakeEmployees struct {
	byUser map[uuid.UUID]*employeeDTO.EmployeeResponse
	byID   map[uuid.UUID]*employeeDTO.EmployeeResponse
	err    error
}

func (f *fakeEmployees) GetByUserID(_ context.Context, t, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if e, ok := f.byUser[u]; ok && e.TenantID == t && f.err == nil {
		return e, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	return nil, sharedErrors.ErrNotFound
}

func (f *fakeEmployees) GetByID(_ context.Context, t, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	if e, ok := f.byID[id]; ok && e.TenantID == t {
		return e, nil
	}
	return nil, sharedErrors.ErrNotFound
}

// fakeLOP returns configured unpaid days per employee (Module 4 port).
type fakeLOP struct {
	days map[uuid.UUID]leavecalc.Days
	err  error
}

func (f *fakeLOP) UnpaidDays(_ context.Context, _, emp uuid.UUID, _, _ time.Time) (leavecalc.Days, error) {
	return f.days[emp], f.err
}

type spyAudit struct{ entries []map[string]interface{} }

func (s *spyAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, meta interface{}, _, _ string) error {
	s.entries = append(s.entries, map[string]interface{}{"action": action, "meta": meta})
	return errors.New("audit sink down")
}

type spyPublisher struct {
	keys []string
	fail bool
}

func (p *spyPublisher) Publish(_ context.Context, _, key string, _ interface{}) error {
	if p.fail {
		return errors.New("broker down")
	}
	p.keys = append(p.keys, key)
	return nil
}

type harness struct {
	st     *store
	emps   *fakeEmployees
	lop    *fakeLOP
	audit  *spyAudit
	pub    *spyPublisher
	now    time.Time
	deps   Deps
	tenant uuid.UUID
}

// newHarness pins now to 2026-10-09; payroll period tests use September 2026 (30 days).
func newHarness() *harness {
	h := &harness{st: newStore(), lop: &fakeLOP{days: map[uuid.UUID]leavecalc.Days{}}, audit: &spyAudit{}, pub: &spyPublisher{},
		emps: &fakeEmployees{byUser: map[uuid.UUID]*employeeDTO.EmployeeResponse{}, byID: map[uuid.UUID]*employeeDTO.EmployeeResponse{}},
		now:  time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC), tenant: uuid.New()}
	h.deps = Deps{Tx: fakeRunner{h.st}, Repos: Repos{Structures: fakeStructures{h.st}, Assignments: fakeAssignments{h.st},
		Runs: fakeRuns{h.st}, Payslips: fakePayslips{h.st}, Outbox: fakeOutbox{h.st}},
		Employees: h.emps, Leave: h.lop, Audit: h.audit, Publisher: h.pub, Now: func() time.Time { return h.now }}
	return h
}

func (h *harness) actor(user uuid.UUID) Actor {
	return Actor{TenantID: h.tenant, UserID: user, CorrelationID: uuid.New(), IP: "203.0.113.9", UserAgent: "test"}
}

func d(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// employee adds an employee joined 2026-01-01 (or the given date) and returns (user, profile).
func (h *harness) employee(code string, joining time.Time, exit *time.Time) (uuid.UUID, *employeeDTO.EmployeeResponse) {
	user := uuid.New()
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: h.tenant, UserID: user, EmployeeCode: code, FirstName: "Emp", LastName: code,
		Status: "active", Employment: &employeeDTO.EmploymentDetail{JoiningDate: joining, ExitDate: exit}}
	h.emps.byUser[user], h.emps.byID[e.ID] = e, e
	return user, e
}

// standardStructure creates BASIC 40% CTC, HRA 50% basic, SPECIAL balance with all statutory switches on.
func (h *harness) standardStructure(t *testing.T, admin Actor) uuid.UUID {
	t.Helper()
	res, err := NewStructureService(h.deps).Create(context.Background(), admin, dto.CreateStructureRequest{Name: "Standard " + uuid.NewString()[:6],
		Components: []dto.ComponentRequest{
			{Code: "basic", Name: "Basic", Kind: "earning", Calc: "percent_of_ctc", Value: 4000},
			{Code: "HRA", Name: "HRA", Kind: "earning", Calc: "percent_of_basic", Value: 5000},
			{Code: "SPECIAL", Name: "Special", Kind: "earning", Calc: "balance"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	return res.ID
}

func (h *harness) assign(t *testing.T, admin Actor, emp uuid.UUID, structure uuid.UUID, ctc calc.Money, from string) {
	t.Helper()
	if _, err := NewAssignmentService(h.deps).Assign(context.Background(), admin, dto.AssignRequest{EmployeeID: emp.String(),
		StructureID: structure.String(), AnnualCTC: ctc, EffectiveFrom: from}); err != nil {
		t.Fatal(err)
	}
}

func codeOf(err error) string {
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	if err != nil {
		return "ERR:" + err.Error()
	}
	return ""
}

func boolp(b bool) *bool    { return &b }
func strp(s string) *string { return &s }
