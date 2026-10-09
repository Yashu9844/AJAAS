package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// --- collaborators ---
type fakeEmployees struct {
	byUser map[uuid.UUID]*employeeDTO.EmployeeResponse
	byID   map[uuid.UUID]*employeeDTO.EmployeeResponse
	err    error
}

func newEmployees() *fakeEmployees {
	return &fakeEmployees{byUser: map[uuid.UUID]*employeeDTO.EmployeeResponse{}, byID: map[uuid.UUID]*employeeDTO.EmployeeResponse{}}
}

func (f *fakeEmployees) add(tenant, user uuid.UUID, status string) *employeeDTO.EmployeeResponse {
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: tenant, UserID: user, Status: status}
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

func (f *fakeEmployees) CountWorking(_ context.Context, t uuid.UUID) (int64, error) {
	var n int64
	for _, e := range f.byID {
		if e.TenantID == t && workingStatuses[e.Status] {
			n++
		}
	}
	return n, f.err
}

type loggedAudit struct {
	action, resource, resourceID string
	metadata                     interface{}
	ip                           string
}

type spyAudit struct{ entries []loggedAudit }

func (s *spyAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, resource, resourceID string, metadata interface{}, ip, _ string) error {
	s.entries = append(s.entries, loggedAudit{action, resource, resourceID, metadata, ip})
	return errors.New("audit sink down") // AT/D3-07: audit failure must never fail the request
}

func (s *spyAudit) actions() []string {
	out := make([]string, len(s.entries))
	for i, e := range s.entries {
		out[i] = e.action
	}
	return out
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

// harness wires a full Deps over the fakes with a controllable clock.
type harness struct {
	st     *store
	emps   *fakeEmployees
	audit  *spyAudit
	pub    *spyPublisher
	now    time.Time
	deps   Deps
	tenant uuid.UUID
}

func newHarness() *harness {
	h := &harness{st: newStore(), emps: newEmployees(), audit: &spyAudit{}, pub: &spyPublisher{},
		now: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC), tenant: uuid.New()}
	h.deps = Deps{
		Tx: fakeRunner{st: h.st},
		Repos: Repos{Shifts: fakeShifts{h.st}, Assignments: fakeAssigns{h.st}, Records: fakeRecords{h.st},
			Punches: fakePunches{h.st}, Regularizations: fakeRegs{h.st}, Outbox: fakeOutbox{h.st}},
		Employees: h.emps, Audit: h.audit, Publisher: h.pub,
		Now: func() time.Time { return h.now },
	}
	return h
}

func (h *harness) actor(user uuid.UUID) Actor {
	return Actor{TenantID: h.tenant, UserID: user, IP: "203.0.113.7", UserAgent: "test", CorrelationID: uuid.New()}
}

func (h *harness) advance(d time.Duration) { h.now = h.now.Add(d) }

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }
