package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/recruitment/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

type fakeEmployees struct {
	byID    map[uuid.UUID]*employeeDTO.EmployeeResponse
	created []employeeDTO.CreateEmployeeRequest
	err     error
	failNew error
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
func (f *fakeEmployees) GetByUserID(_ context.Context, t, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, e := range f.byID {
		if e.UserID == u && e.TenantID == t {
			return e, nil
		}
	}
	return nil, sharedErrors.ErrNotFound
}
func (f *fakeEmployees) CreateEmployee(_ context.Context, t uuid.UUID, req employeeDTO.CreateEmployeeRequest) (*employeeDTO.EmployeeResponse, error) {
	if f.failNew != nil {
		return nil, f.failNew
	}
	f.created = append(f.created, req)
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: t, UserID: req.UserID, EmployeeCode: req.EmployeeCode, Status: "active"}
	f.byID[e.ID] = e
	return e, nil
}
func (f *fakeEmployees) add(t uuid.UUID, status string) *employeeDTO.EmployeeResponse {
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: t, UserID: uuid.New(), Status: status}
	f.byID[e.ID] = e
	return e
}

type fakeUsers struct {
	invites []identityDTO.InviteUserRequest
	err     error
	badID   bool
}

func (f *fakeUsers) InviteUser(_ context.Context, _ *gorm.DB, _ uuid.UUID, req identityDTO.InviteUserRequest, _ uuid.UUID) (*identityDTO.UserResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.invites = append(f.invites, req)
	if f.badID {
		return &identityDTO.UserResponse{ID: "not-a-uuid"}, nil
	}
	return &identityDTO.UserResponse{ID: uuid.NewString(), Email: req.Email}, nil
}

type fakeOrg struct {
	ids map[uuid.UUID]bool
	err error
}

func (f *fakeOrg) DepartmentExists(_ context.Context, _, id uuid.UUID) (bool, error) {
	return f.ids[id], f.err
}
func (f *fakeOrg) DesignationExists(_ context.Context, _, id uuid.UUID) (bool, error) {
	return f.ids[id], f.err
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
	st    *store
	emps  *fakeEmployees
	users *fakeUsers
	org   *fakeOrg
	audit *spyAudit
	pub   *spyPublisher
	now   time.Time
	actor Actor
	deps  Deps
	jobs  *JobService
	cands *CandidateService
	ivs   *InterviewService
	offs  *OfferService
	hire  *HireService
}

func newHarness() *harness {
	h := &harness{st: newStore(), emps: &fakeEmployees{byID: map[uuid.UUID]*employeeDTO.EmployeeResponse{}}, users: &fakeUsers{},
		org: &fakeOrg{ids: map[uuid.UUID]bool{}}, audit: &spyAudit{}, pub: &spyPublisher{},
		now: time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)}
	h.actor = Actor{TenantID: uuid.New(), UserID: uuid.New(), CorrelationID: uuid.New()}
	h.deps = Deps{Tx: fakeRunner{h.st}, Repos: Repos{Jobs: fakeJobs{h.st}, Candidates: fakeCandidates{h.st}, Interviews: fakeInterviews{h.st},
		Offers: fakeOffers{h.st}, Outbox: fakeOutbox{h.st}}, Employees: h.emps, Users: h.users, Org: h.org, Audit: h.audit,
		Publisher: h.pub, Now: func() time.Time { return h.now }}
	h.jobs, h.cands, h.ivs = NewJobService(h.deps), NewCandidateService(h.deps), NewInterviewService(h.deps)
	h.offs, h.hire = NewOfferService(h.deps), NewHireService(h.deps)
	return h
}

// must panics on err; a panic fails the running test with a stack trace.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func wantErr(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var ae *sharedErrors.AppError
	if !errors.As(err, &ae) || ae.Code != code {
		t.Fatalf("got %v, want code %s", err, code)
	}
}

func (h *harness) openJob(t *testing.T, headcount int) *dto.JobResponse {
	t.Helper()
	j := must(h.jobs.Create(context.Background(), h.actor, dto.CreateJobRequest{Title: "Backend Engineer", Headcount: headcount,
		EmploymentType: "full_time", Description: "Go"}))
	return must(h.jobs.SetStatus(context.Background(), h.actor, j.ID, "open"))
}

func (h *harness) candidate(t *testing.T, jobID uuid.UUID, email string) *dto.CandidateResponse {
	t.Helper()
	return must(h.cands.Create(context.Background(), h.actor, dto.CreateCandidateRequest{JobID: jobID.String(), FirstName: "Asha",
		LastName: "Rao", Email: email, Source: "referral"}))
}

func (h *harness) toStage(t *testing.T, id uuid.UUID, stages ...string) {
	t.Helper()
	for _, s := range stages {
		must(h.cands.MoveStage(context.Background(), h.actor, id, dto.StageRequest{Stage: s}))
	}
}

func (h *harness) offer(t *testing.T, candID uuid.UUID) *dto.OfferResponse {
	t.Helper()
	return must(h.offs.Create(context.Background(), h.actor, dto.CreateOfferRequest{CandidateID: candID.String(),
		OfferedCTC: calc.Rupees(1200000), JoiningDate: "2026-11-02"}))
}

// accepted drives a fresh candidate on jobID to stage offer with an accepted offer.
func (h *harness) accepted(t *testing.T, jobID uuid.UUID, email string) uuid.UUID {
	t.Helper()
	c := h.candidate(t, jobID, email)
	h.toStage(t, c.ID, "screening", "interview")
	o := h.offer(t, c.ID)
	must(h.offs.Decide(context.Background(), h.actor, o.ID, "accepted"))
	return c.ID
}
