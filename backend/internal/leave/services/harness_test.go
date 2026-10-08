package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

type fakeEmployees struct {
	byUser map[uuid.UUID]*employeeDTO.EmployeeResponse
	byID   map[uuid.UUID]*employeeDTO.EmployeeResponse
	err    error
}

func newEmployees() *fakeEmployees {
	return &fakeEmployees{byUser: map[uuid.UUID]*employeeDTO.EmployeeResponse{}, byID: map[uuid.UUID]*employeeDTO.EmployeeResponse{}}
}

func (f *fakeEmployees) add(tenant, user uuid.UUID, status, gender string, joining time.Time) *employeeDTO.EmployeeResponse {
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: tenant, UserID: user, Status: status, Gender: gender,
		Employment: &employeeDTO.EmploymentDetail{JoiningDate: joining}}
	f.byUser[user], f.byID[e.ID] = e, e
	return e
}

func (f *fakeEmployees) GetByUserID(_ context.Context, t, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	if e, ok := f.byUser[u]; ok && e.TenantID == t {
		return e, nil
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

type loggedAudit struct {
	action, resource, resourceID string
	metadata                     interface{}
}

type spyAudit struct{ entries []loggedAudit }

func (s *spyAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, resource, resourceID string, metadata interface{}, _, _ string) error {
	s.entries = append(s.entries, loggedAudit{action, resource, resourceID, metadata})
	return errors.New("audit sink down") // audit failure must never fail the request
}

type spyPublisher struct {
	keys []string
	fail bool
}

func (p *spyPublisher) Publish(_ context.Context, _, routingKey string, _ interface{}) error {
	if p.fail {
		return errors.New("broker down")
	}
	p.keys = append(p.keys, routingKey)
	return nil
}

// spyAttendance records Module 3 LeaveSync calls (C8).
type spyAttendance struct {
	marked, cleared []time.Time
	err             error
}

func (s *spyAttendance) MarkLeave(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, dates []time.Time) error {
	s.marked = append(s.marked, dates...)
	return s.err
}

func (s *spyAttendance) ClearLeave(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, dates []time.Time) error {
	s.cleared = append(s.cleared, dates...)
	return s.err
}

type harness struct {
	st     *store
	emps   *fakeEmployees
	audit  *spyAudit
	pub    *spyPublisher
	att    *spyAttendance
	now    time.Time
	deps   Deps
	tenant uuid.UUID
}

// newHarness pins "now" to Thursday 2026-10-08 09:00 UTC.
func newHarness() *harness {
	h := &harness{st: newStore(), emps: newEmployees(), audit: &spyAudit{}, pub: &spyPublisher{}, att: &spyAttendance{},
		now: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC), tenant: uuid.New()}
	h.deps = Deps{
		Tx: fakeRunner{st: h.st},
		Repos: Repos{Types: fakeTypes{h.st}, Holidays: fakeHolidays{h.st}, Balances: fakeBalances{h.st},
			Requests: fakeRequests{h.st}, Ledger: fakeLedger{h.st}, Outbox: fakeOutbox{h.st}},
		Employees: h.emps, Audit: h.audit, Attendance: h.att, Publisher: h.pub,
		Now: func() time.Time { return h.now },
	}
	return h
}

func (h *harness) actor(user uuid.UUID) Actor {
	return Actor{TenantID: h.tenant, UserID: user, IP: "203.0.113.7", UserAgent: "test", CorrelationID: uuid.New()}
}

// employee adds an active employee who joined before the leave year.
func (h *harness) employee() (uuid.UUID, *employeeDTO.EmployeeResponse) {
	user := uuid.New()
	return user, h.emps.add(h.tenant, user, "active", "female", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
}

// leaveType creates an active type directly in the store.
func (h *harness) leaveType(code string, allowance calc.Days, mut func(*models.LeaveType)) *models.LeaveType {
	t := models.LeaveType{TenantID: h.tenant, Name: code + " leave", Code: code, IsPaid: true, AnnualAllowance: allowance,
		Accrual: models.AccrualAnnual, AllowHalfDay: true, ApplicableGender: models.GenderAll, Status: models.TypeActive}
	t.ID = uuid.New()
	if mut != nil {
		mut(&t)
	}
	h.st.types[t.ID] = t
	return &t
}

func (h *harness) balanceOf(emp, typ uuid.UUID) models.Balance {
	for _, b := range h.st.balances {
		if b.EmployeeProfileID == emp && b.LeaveTypeID == typ {
			return b
		}
	}
	return models.Balance{}
}

func (h *harness) apply(t *testing.T, user uuid.UUID, typ *models.LeaveType, from, to string) (*dto.LeaveRequestResponse, error) {
	t.Helper()
	return NewRequestService(h.deps).Apply(context.Background(), h.actor(user),
		dto.ApplyLeaveRequest{LeaveTypeID: typ.ID.String(), StartDate: from, EndDate: to, Reason: "family event"})
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

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }
func boolp(b bool) *bool    { return &b }
