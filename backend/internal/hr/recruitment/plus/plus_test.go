package plus

import (
	"context"
	"errors"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/kernel"
	"github.com/jaas/jaas/internal/hr/recruitment/models"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fakes ---

type fakeStore struct {
	reqs       map[uuid.UUID]Requisition
	jobs       map[uuid.UUID]models.Job
	cards      map[uuid.UUID]Scorecard
	jobCard    map[uuid.UUID]JobScorecard
	interviews map[uuid.UUID]models.Interview
	cands      map[uuid.UUID]models.Candidate
	scores     map[uuid.UUID][]InterviewScore
	policy     *OfferPolicy
	offers     map[uuid.UUID]models.Offer
	approvals  []OfferApproval
	events     []string
	fail       map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{reqs: map[uuid.UUID]Requisition{}, jobs: map[uuid.UUID]models.Job{}, cards: map[uuid.UUID]Scorecard{}, jobCard: map[uuid.UUID]JobScorecard{},
		interviews: map[uuid.UUID]models.Interview{}, cands: map[uuid.UUID]models.Candidate{}, scores: map[uuid.UUID][]InterviewScore{},
		offers: map[uuid.UUID]models.Offer{}, fail: map[string]error{}}
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	reqs, jobs, cands, offers, appr, ev := map[uuid.UUID]Requisition{}, map[uuid.UUID]models.Job{}, map[uuid.UUID]models.Candidate{}, map[uuid.UUID]models.Offer{},
		append([]OfferApproval(nil), f.approvals...), len(f.events)
	for k, v := range f.reqs {
		reqs[k] = v
	}
	for k, v := range f.jobs {
		jobs[k] = v
	}
	for k, v := range f.cands {
		cands[k] = v
	}
	for k, v := range f.offers {
		offers[k] = v
	}
	if err := fn(nil); err != nil {
		f.reqs, f.jobs, f.cands, f.offers, f.approvals, f.events = reqs, jobs, cands, offers, appr, f.events[:ev]
		return err
	}
	return nil
}
func (f *fakeStore) DB() *gorm.DB { return nil }
func (f *fakeStore) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["create"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *Requisition:
		_ = v.BeforeCreate(nil)
		f.reqs[v.ID] = *v
	case *Scorecard:
		_ = v.BeforeCreate(nil)
		f.cards[v.ID] = *v
	case *OfferApproval:
		_ = v.BeforeCreate(nil)
		f.approvals = append(f.approvals, *v)
	}
	return nil
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["save"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *Requisition:
		f.reqs[v.ID] = *v
	case *Scorecard:
		f.cards[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Requisition(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Requisition, error) {
	if r, ok := f.reqs[id]; ok {
		return &r, f.fail["req"]
	}
	return nil, f.fail["req"]
}
func (f *fakeStore) Requisitions(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl ReqFilter, _ int) ([]Requisition, error) {
	var out []Requisition
	for _, r := range f.reqs {
		if (fl.Status == "" || r.Status == fl.Status) && (fl.PositionID == nil || r.PositionID == *fl.PositionID) {
			out = append(out, r)
		}
	}
	return out, f.fail["reqs"]
}
func (f *fakeStore) RequestedSeats(_ context.Context, _ *gorm.DB, _, position, except uuid.UUID) (int, error) {
	n := 0
	for _, r := range f.reqs {
		if r.PositionID != position || r.ID == except {
			continue
		}
		if r.Status == ReqSubmitted || (r.Status == ReqApproved && r.JobID != nil && jobLive(f.jobs[*r.JobID].Status)) {
			n += r.Headcount
		}
	}
	return n, f.fail["seats"]
}
func jobLive(status string) bool { return status == "draft" || status == "open" || status == "on_hold" }
func (f *fakeStore) CreateJob(_ context.Context, _ *gorm.DB, job *models.Job) error {
	if err := f.fail["createjob"]; err != nil {
		return err
	}
	_ = job.BeforeCreate(nil)
	f.jobs[job.ID] = *job
	return nil
}
func (f *fakeStore) Job(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*models.Job, error) {
	if j, ok := f.jobs[id]; ok {
		return &j, f.fail["job"]
	}
	return nil, f.fail["job"]
}
func (f *fakeStore) SaveJob(_ context.Context, _ *gorm.DB, j *models.Job) error {
	f.jobs[j.ID] = *j
	return f.fail["savejob"]
}
func (f *fakeStore) Scorecard(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*Scorecard, error) {
	if c, ok := f.cards[id]; ok {
		return &c, f.fail["card"]
	}
	return nil, f.fail["card"]
}
func (f *fakeStore) Scorecards(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string) ([]Scorecard, error) {
	var out []Scorecard
	for _, c := range f.cards {
		if status == "" || c.Status == status {
			out = append(out, c)
		}
	}
	return out, f.fail["cards"]
}
func (f *fakeStore) JobScorecard(_ context.Context, _ *gorm.DB, _, job uuid.UUID) (*JobScorecard, error) {
	if l, ok := f.jobCard[job]; ok {
		return &l, f.fail["joblink"]
	}
	return nil, f.fail["joblink"]
}
func (f *fakeStore) SaveJobScorecard(_ context.Context, _ *gorm.DB, row *JobScorecard) error {
	f.jobCard[row.JobID] = *row
	return f.fail["savelink"]
}
func (f *fakeStore) Interview(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*models.Interview, error) {
	if i, ok := f.interviews[id]; ok {
		return &i, f.fail["interview"]
	}
	return nil, f.fail["interview"]
}
func (f *fakeStore) Candidate(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*models.Candidate, error) {
	if c, ok := f.cands[id]; ok {
		return &c, f.fail["cand"]
	}
	return nil, f.fail["cand"]
}
func (f *fakeStore) SaveCandidate(_ context.Context, _ *gorm.DB, c *models.Candidate) error {
	f.cands[c.ID] = *c
	return f.fail["savecand"]
}
func (f *fakeStore) ReplaceScores(_ context.Context, _ *gorm.DB, _, interview uuid.UUID, rows []InterviewScore) error {
	f.scores[interview] = rows
	return f.fail["replace"]
}
func (f *fakeStore) CandidateScores(_ context.Context, _ *gorm.DB, _, cand uuid.UUID) ([]ScoredInterview, error) {
	var out []ScoredInterview
	for id, rows := range f.scores {
		if f.interviews[id].CandidateID != cand {
			continue
		}
		for _, r := range rows {
			out = append(out, ScoredInterview{InterviewID: id, Competency: r.Competency, Score: r.Score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].InterviewID.String()+out[i].Competency < out[j].InterviewID.String()+out[j].Competency
	})
	return out, f.fail["candscores"]
}
func (f *fakeStore) OfferPolicy(context.Context, *gorm.DB, uuid.UUID) (*OfferPolicy, error) {
	return f.policy, f.fail["policy"]
}
func (f *fakeStore) SaveOfferPolicy(_ context.Context, _ *gorm.DB, p *OfferPolicy) error {
	f.policy = p
	return f.fail["savepolicy"]
}
func (f *fakeStore) Offer(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*models.Offer, error) {
	if o, ok := f.offers[id]; ok {
		return &o, f.fail["offer"]
	}
	return nil, f.fail["offer"]
}
func (f *fakeStore) SaveOffer(_ context.Context, _ *gorm.DB, o *models.Offer) error {
	f.offers[o.ID] = *o
	return f.fail["saveoffer"]
}
func (f *fakeStore) Approvals(_ context.Context, _ *gorm.DB, _, offer uuid.UUID) ([]OfferApproval, error) {
	var out []OfferApproval
	for _, a := range f.approvals {
		if a.OfferID == offer {
			out = append(out, a)
		}
	}
	return out, f.fail["approvals"]
}
func (f *fakeStore) Event(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string, _ map[string]string) error {
	if err := f.fail["event"]; err != nil {
		return err
	}
	f.events = append(f.events, key)
	return nil
}

type fakePositions struct {
	info map[uuid.UUID]PositionInfo
	err  error
}

func (p *fakePositions) PositionInfo(_ context.Context, _, id uuid.UUID) (PositionInfo, error) {
	return p.info[id], p.err
}

type fakeEmployees struct {
	byUser map[uuid.UUID]employeeDTO.EmployeeResponse
	err    error
}

func (e *fakeEmployees) GetByUserID(_ context.Context, _, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if r, ok := e.byUser[u]; ok {
		return &r, e.err
	}
	return nil, sharedErrors.ErrNotFound
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type env struct {
	svc                  *Service
	store                *fakeStore
	pos                  *fakePositions
	emps                 *fakeEmployees
	tenant, position     uuid.UUID
	hr, approver, second Actor
	interviewer          Actor
	interviewerEmp       uuid.UUID
}

var now = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{store: newStore(), tenant: uuid.New(), position: uuid.New(), interviewerEmp: uuid.New()}
	e.pos = &fakePositions{info: map[uuid.UUID]PositionInfo{e.position: {Exists: true, Active: true, Title: "Backend engineer", DepartmentID: uuid.New(), OpenSeats: 3}}}
	e.interviewer = Actor{TenantID: e.tenant, UserID: uuid.New()}
	e.emps = &fakeEmployees{byUser: map[uuid.UUID]employeeDTO.EmployeeResponse{e.interviewer.UserID: {ID: e.interviewerEmp, UserID: e.interviewer.UserID}}}
	e.svc = NewService(e.store, Ports{Positions: e.pos, Employees: e.emps, Audit: noAudit{}}, func() time.Time { return now })
	e.hr = Actor{TenantID: e.tenant, UserID: uuid.New()}
	e.approver = Actor{TenantID: e.tenant, UserID: uuid.New()}
	e.second = Actor{TenantID: e.tenant, UserID: uuid.New()}
	return e
}

func ctx() context.Context { return context.Background() }

func code(t *testing.T, err error, want string) {
	t.Helper()
	var app *sharedErrors.AppError
	if !errors.As(err, &app) || app.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustErr(_ interface{}, err error) error { return err }

func str(s string) *string { return &s }

// --- RP-01 seat release ---

func TestReleaseHire(t *testing.T) {
	e := setup(t)
	job := models.Job{ID: uuid.New(), TenantID: e.tenant, Headcount: 1, ReservedCount: 1}
	cand := models.Candidate{ID: uuid.New(), TenantID: e.tenant, JobID: job.ID, SeatReserved: true}
	e.store.jobs[job.ID], e.store.cands[cand.ID] = job, cand
	ok(t, e.svc.ReleaseHire(ctx(), e.hr, cand.ID))
	if e.store.jobs[job.ID].ReservedCount != 0 || e.store.cands[cand.ID].SeatReserved {
		t.Fatal("RG1 the seat is given back")
	}
	ok(t, e.svc.ReleaseHire(ctx(), e.hr, cand.ID)) // idempotent: nothing reserved any more
	if e.store.jobs[job.ID].ReservedCount != 0 {
		t.Fatal("never below zero")
	}
	code(t, e.svc.ReleaseHire(ctx(), e.hr, uuid.New()), "NOT_FOUND")
	emp := uuid.New()
	hired := e.store.cands[cand.ID]
	hired.SeatReserved, hired.HiredEmployeeID = true, &emp
	e.store.cands[cand.ID] = hired
	code(t, e.svc.ReleaseHire(ctx(), e.hr, cand.ID), "HIRE_COMPLETED")
	lost := models.Candidate{ID: uuid.New(), TenantID: e.tenant, JobID: uuid.New(), SeatReserved: true}
	e.store.cands[lost.ID] = lost
	code(t, e.svc.ReleaseHire(ctx(), e.hr, lost.ID), "NOT_FOUND")
	for _, k := range []string{"cand", "savejob", "savecand"} {
		e.store.fail[k] = errBoom
		fresh := models.Candidate{ID: uuid.New(), TenantID: e.tenant, JobID: job.ID, SeatReserved: true}
		e.store.cands[fresh.ID] = fresh
		if err := e.svc.ReleaseHire(ctx(), e.hr, fresh.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
}

// --- RP-02, RP-03 requisitions ---

func TestRequisitions(t *testing.T) {
	e := setup(t)
	pos := e.position.String()
	for _, in := range []RequisitionInput{{PositionID: "x"}, {PositionID: pos, Headcount: 0, Justification: "j"}, {PositionID: pos, Headcount: 1},
		{PositionID: pos, Headcount: 1, Justification: strings.Repeat("j", 1001)}} {
		code(t, mustErr(e.svc.CreateRequisition(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: uuid.NewString(), Headcount: 1, Justification: "j"})), "NOT_FOUND")
	closed := uuid.New()
	e.pos.info[closed] = PositionInfo{Exists: true, Active: false, OpenSeats: 2}
	code(t, mustErr(e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: closed.String(), Headcount: 1, Justification: "j"})), "INACTIVE_REFERENCE")
	code(t, mustErr(e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 4, Justification: "j"})), "NO_VACANCY")

	r1, err := e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 2, Justification: " need two "})
	if err != nil || r1.Status != ReqDraft || r1.Justification != "need two" || r1.Title != "Backend engineer" {
		t.Fatal(r1, err)
	}
	code(t, mustErr(e.svc.SubmitRequisition(ctx(), e.approver, r1.ID)), "NOT_FOUND") // only the requester submits
	r1, err = e.svc.SubmitRequisition(ctx(), e.hr, r1.ID)
	ok(t, err)
	code(t, mustErr(e.svc.SubmitRequisition(ctx(), e.hr, r1.ID)), "REQUISITION_STATE")
	// RG2: 3 seats open, 2 are held by the submitted requisition.
	r2, err := e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 1, Justification: "one more"})
	ok(t, err)
	if _, err := e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 2, Justification: "too many"}); err != nil {
		code(t, err, "NO_VACANCY")
	}
	code(t, mustErr(e.svc.RejectRequisition(ctx(), e.approver, r1.ID, nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.ApproveRequisition(ctx(), e.hr, r1.ID, nil)), "RECRUITMENT_SELF_APPROVAL")
	code(t, mustErr(e.svc.RejectRequisition(ctx(), e.hr, r1.ID, str("no"))), "RECRUITMENT_SELF_APPROVAL")
	done, err := e.svc.ApproveRequisition(ctx(), e.approver, r1.ID, str("ok"))
	if err != nil || done.Status != ReqApproved || done.JobID == nil || *done.DecisionComment != "ok" {
		t.Fatal(done, err)
	}
	job := e.store.jobs[*done.JobID]
	if job.Status != "draft" || job.Headcount != 2 || job.Title != "Backend engineer" || job.DepartmentID == nil || job.CreatedByUserID != e.hr.UserID {
		t.Fatalf("RP-03 draft job: %+v", job)
	}
	code(t, mustErr(e.svc.ApproveRequisition(ctx(), e.approver, r1.ID, nil)), "REQUISITION_STATE")
	// The approved requisition still holds its seats while the job is live; once the job is filled they are free again.
	if _, err := e.svc.SubmitRequisition(ctx(), e.hr, r2.ID); err != nil {
		t.Fatal(err)
	}
	code(t, mustErr(e.svc.CancelRequisition(ctx(), e.approver, r2.ID)), "NOT_FOUND")
	cancelled, err := e.svc.CancelRequisition(ctx(), e.hr, r2.ID)
	if err != nil || cancelled.Status != ReqCancelled {
		t.Fatal(cancelled, err)
	}
	code(t, mustErr(e.svc.CancelRequisition(ctx(), e.hr, r2.ID)), "REQUISITION_STATE")
	r3, _ := e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 1, Justification: "x"})
	ok(t, mustErrOnly(e.svc.SubmitRequisition(ctx(), e.hr, r3.ID)))
	rej, err := e.svc.RejectRequisition(ctx(), e.approver, r3.ID, str("budget"))
	if err != nil || rej.Status != ReqRejected {
		t.Fatal(rej, err)
	}

	// Reads.
	if rows, err := e.svc.Requisitions(ctx(), e.hr, ReqApproved, pos); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.Requisitions(ctx(), e.hr, "weird", "")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Requisitions(ctx(), e.hr, "", "x")), "VALIDATION_ERROR")
	if _, err := e.svc.GetRequisition(ctx(), e.hr, r1.ID); err != nil {
		t.Fatal(err)
	}
	code(t, mustErr(e.svc.GetRequisition(ctx(), e.hr, uuid.New())), "NOT_FOUND")

	// Failures roll back.
	e.pos.err = errBoom
	if _, err := e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 1, Justification: "x"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.pos.err = nil
	r4, _ := e.svc.CreateRequisition(ctx(), e.hr, RequisitionInput{PositionID: pos, Headcount: 1, Justification: "x"})
	ok(t, mustErrOnly(e.svc.SubmitRequisition(ctx(), e.hr, r4.ID)))
	for _, k := range []string{"createjob", "event", "save", "seats"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.ApproveRequisition(ctx(), e.approver, r4.ID, nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
		if e.store.reqs[r4.ID].Status != ReqSubmitted {
			t.Fatalf("%s: a failed approval must change nothing", k)
		}
	}
	e.store.fail["reqs"] = errBoom
	if _, err := e.svc.Requisitions(ctx(), e.hr, "", ""); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func mustErrOnly[T any](_ T, err error) error { return err }

// --- RP-04, RP-05 scorecards ---

func competencies() *Competencies {
	return &Competencies{{Name: "Coding", WeightPct: 6000}, {Name: "Communication", WeightPct: 4000}}
}

func TestScorecards(t *testing.T) {
	e := setup(t)
	bad := []ScorecardInput{{}, {Name: str("x")}, {Name: str(" "), Competencies: competencies()}, {Name: str("x"), Competencies: &Competencies{}},
		{Name: str("x"), Competencies: &Competencies{{Name: "A", WeightPct: 5000}}},
		{Name: str("x"), Competencies: &Competencies{{Name: "A", WeightPct: 5000}, {Name: "a", WeightPct: 5000}}},
		{Name: str("x"), Competencies: &Competencies{{Name: "A", WeightPct: 0}, {Name: "B", WeightPct: 10000}}},
		{Name: str("x"), Competencies: competencies(), Status: str("weird")}}
	for _, in := range bad {
		if _, err := e.svc.CreateScorecard(ctx(), e.hr, in); err == nil {
			t.Fatalf("RG3 must refuse %+v", in)
		}
	}
	sc, err := e.svc.CreateScorecard(ctx(), e.hr, ScorecardInput{Name: str(" Backend "), Competencies: competencies()})
	if err != nil || sc.Name != "Backend" || sc.Status != ScorecardActive {
		t.Fatal(sc, err)
	}
	up, err := e.svc.UpdateScorecard(ctx(), e.hr, sc.ID, ScorecardInput{Status: str(ScorecardArchived)})
	if err != nil || up.Status != ScorecardArchived {
		t.Fatal(up, err)
	}
	code(t, mustErr(e.svc.UpdateScorecard(ctx(), e.hr, uuid.New(), ScorecardInput{})), "NOT_FOUND")
	if _, err := e.svc.UpdateScorecard(ctx(), e.hr, sc.ID, ScorecardInput{Competencies: &Competencies{{Name: "Solo", WeightPct: 10000}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetScorecard(ctx(), e.hr, sc.ID); err != nil {
		t.Fatal(err)
	}
	code(t, mustErr(e.svc.GetScorecard(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	if rows, _ := e.svc.Scorecards(ctx(), e.hr, ScorecardArchived); len(rows) != 1 {
		t.Fatal("status filter")
	}
	code(t, mustErr(e.svc.Scorecards(ctx(), e.hr, "weird")), "VALIDATION_ERROR")

	// Assign to a job.
	job := models.Job{ID: uuid.New(), TenantID: e.tenant, Status: "draft"}
	e.store.jobs[job.ID] = job
	code(t, mustErr(e.svc.AssignScorecard(ctx(), e.hr, job.ID, "x")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.AssignScorecard(ctx(), e.hr, uuid.New(), sc.ID.String())), "NOT_FOUND")
	code(t, mustErr(e.svc.AssignScorecard(ctx(), e.hr, job.ID, uuid.NewString())), "NOT_FOUND")
	code(t, mustErr(e.svc.AssignScorecard(ctx(), e.hr, job.ID, sc.ID.String())), "INACTIVE_REFERENCE")
	_, err = e.svc.UpdateScorecard(ctx(), e.hr, sc.ID, ScorecardInput{Status: str(ScorecardActive)})
	ok(t, err)
	closedJob := models.Job{ID: uuid.New(), TenantID: e.tenant, Status: "closed"}
	e.store.jobs[closedJob.ID] = closedJob
	code(t, mustErr(e.svc.AssignScorecard(ctx(), e.hr, closedJob.ID, sc.ID.String())), "JOB_NOT_OPEN")
	if _, err := e.svc.AssignScorecard(ctx(), e.hr, job.ID, sc.ID.String()); err != nil {
		t.Fatal(err)
	}
	e.store.fail["savelink"] = errBoom
	if _, err := e.svc.AssignScorecard(ctx(), e.hr, job.ID, sc.ID.String()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func (e *env) interviewFor(t *testing.T) (jobID, candID, interviewID uuid.UUID) {
	t.Helper()
	sc, err := e.svc.CreateScorecard(ctx(), e.hr, ScorecardInput{Name: str("Backend " + uuid.NewString()[:4]), Competencies: competencies()})
	ok(t, err)
	job := models.Job{ID: uuid.New(), TenantID: e.tenant, Status: "open"}
	cand := models.Candidate{ID: uuid.New(), TenantID: e.tenant, JobID: job.ID}
	iv := models.Interview{ID: uuid.New(), TenantID: e.tenant, CandidateID: cand.ID, InterviewerEmployeeID: e.interviewerEmp, Status: models.InterviewScheduled}
	e.store.jobs[job.ID], e.store.cands[cand.ID], e.store.interviews[iv.ID] = job, cand, iv
	_, err = e.svc.AssignScorecard(ctx(), e.hr, job.ID, sc.ID.String())
	ok(t, err)
	return job.ID, cand.ID, iv.ID
}

func TestScores(t *testing.T) {
	e := setup(t)
	_, cand, iv := e.interviewFor(t)
	good := []ScoreInput{{Competency: "coding", Score: 4, Note: str("solid")}, {Competency: "Communication", Score: 3}}
	code(t, mustErr(e.svc.SubmitScores(ctx(), e.hr, iv, good)), "NOT_FOUND") // no employee profile
	other := Actor{TenantID: e.tenant, UserID: uuid.New()}
	e.emps.byUser[other.UserID] = employeeDTO.EmployeeResponse{ID: uuid.New(), UserID: other.UserID}
	code(t, mustErr(e.svc.SubmitScores(ctx(), other, iv, good)), "NOT_FOUND") // not the assigned interviewer
	code(t, mustErr(e.svc.SubmitScores(ctx(), e.interviewer, uuid.New(), good)), "NOT_FOUND")
	for _, in := range [][]ScoreInput{nil, {{Competency: "Coding", Score: 4}}, {{Competency: "Coding", Score: 4}, {Competency: "Coding", Score: 4}},
		{{Competency: "Coding", Score: 4}, {Competency: "Nope", Score: 4}}, {{Competency: "Coding", Score: 0}, {Competency: "Communication", Score: 3}},
		{{Competency: "Coding", Score: 6}, {Competency: "Communication", Score: 3}},
		{{Competency: "Coding", Score: 4, Note: str(strings.Repeat("n", 501))}, {Competency: "Communication", Score: 3}}} {
		if _, err := e.svc.SubmitScores(ctx(), e.interviewer, iv, in); err == nil {
			t.Fatalf("must refuse %+v", in)
		}
	}
	rows, err := e.svc.SubmitScores(ctx(), e.interviewer, iv, good)
	if err != nil || len(rows) != 2 || rows[0].Competency != "Coding" || *rows[0].Note != "solid" {
		t.Fatal(rows, err)
	}
	// A second interview by another interviewer; RG3 exact averages: coding (4+5)/2 = 4.50, communication (3+3)/2 = 3.00.
	second := models.Interview{ID: uuid.New(), TenantID: e.tenant, CandidateID: cand, InterviewerEmployeeID: e.interviewerEmp, Status: models.InterviewCompleted}
	e.store.interviews[second.ID] = second
	_, err = e.svc.SubmitScores(ctx(), e.interviewer, second.ID, []ScoreInput{{Competency: "Coding", Score: 5}, {Competency: "Communication", Score: 3}})
	ok(t, err)
	sc, err := e.svc.Scorecard(ctx(), e.hr, cand)
	if err != nil || sc.Interviews != 2 || sc.Competences[0].Average != 450 || sc.Competences[1].Average != 300 {
		t.Fatalf("%+v %v", sc, err)
	}
	// 4.50 Ã— 60% + 3.00 Ã— 40% = 3.90 â†’ 390 hundredths.
	if sc.Overall != 390 {
		t.Fatalf("RG3 overall %d", sc.Overall)
	}
	e.store.interviews[iv] = func() models.Interview { i := e.store.interviews[iv]; i.Status = models.InterviewCancelled; return i }()
	code(t, mustErr(e.svc.SubmitScores(ctx(), e.interviewer, iv, good)), "INTERVIEW_STATE")
	code(t, mustErr(e.svc.Scorecard(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	noCard := models.Candidate{ID: uuid.New(), TenantID: e.tenant, JobID: uuid.New()}
	e.store.cands[noCard.ID] = noCard
	code(t, mustErr(e.svc.Scorecard(ctx(), e.hr, noCard.ID)), "NO_SCORECARD")
	for _, k := range []string{"candscores", "card", "joblink", "cand"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Scorecard(ctx(), e.hr, cand); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["replace"] = errBoom
	if _, err := e.svc.SubmitScores(ctx(), e.interviewer, second.ID, []ScoreInput{{Competency: "Coding", Score: 5}, {Competency: "Communication", Score: 3}}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "replace")
	e.emps.err = errBoom
	if _, err := e.svc.SubmitScores(ctx(), e.interviewer, second.ID, good); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- RP-06, RP-07 offer approval ---

func (e *env) offer(status string, ctc kernel.Money) models.Offer {
	o := models.Offer{ID: uuid.New(), TenantID: e.tenant, CandidateID: uuid.New(), OfferedCTC: ctc, Status: status, CreatedByUserID: e.hr.UserID}
	e.store.offers[o.ID] = o
	return o
}

func TestOfferPolicyAndGate(t *testing.T) {
	e := setup(t)
	if p, err := e.svc.GetOfferPolicy(ctx(), e.hr); err != nil || p.RequiredApprovals != 0 {
		t.Fatal(p, err)
	}
	for _, in := range []PolicyInput{{RequiredApprovals: -1}, {RequiredApprovals: 4}, {RequiredApprovals: 1, ThresholdCTC: -1}} {
		code(t, mustErr(e.svc.SetOfferPolicy(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	if st, err := e.svc.StartingStatus(ctx(), nil, e.tenant, kernel.Rupees(5000000)); err != nil || st != StatusOffered {
		t.Fatal("no policy, no gate", st, err)
	}
	_, err := e.svc.SetOfferPolicy(ctx(), e.hr, PolicyInput{RequiredApprovals: 2, ThresholdCTC: kernel.Rupees(1000000)})
	ok(t, err)
	if st, _ := e.svc.StartingStatus(ctx(), nil, e.tenant, kernel.Rupees(1000000)); st != StatusOffered {
		t.Fatal("at the threshold: no approval")
	}
	if st, _ := e.svc.StartingStatus(ctx(), nil, e.tenant, kernel.Rupees(1000001)); st != StatusPendingApproval {
		t.Fatal("above the threshold: approval needed")
	}
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.StartingStatus(ctx(), nil, e.tenant, 1); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.GetOfferPolicy(ctx(), e.hr); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "policy")
	e.store.fail["savepolicy"] = errBoom
	if _, err := e.svc.SetOfferPolicy(ctx(), e.hr, PolicyInput{}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestOfferApproval(t *testing.T) {
	e := setup(t)
	_, err := e.svc.SetOfferPolicy(ctx(), e.hr, PolicyInput{RequiredApprovals: 2})
	ok(t, err)
	o := e.offer(StatusPendingApproval, kernel.Rupees(2000000))
	code(t, mustErr(e.svc.DecideOffer(ctx(), e.hr, o.ID, DecisionApproved, nil)), "RECRUITMENT_SELF_APPROVAL")
	code(t, mustErr(e.svc.DecideOffer(ctx(), e.approver, o.ID, DecisionRejected, nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.DecideOffer(ctx(), e.approver, uuid.New(), DecisionApproved, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.DecideOffer(ctx(), e.approver, o.ID, DecisionApproved, str(strings.Repeat("c", 501)))), "VALIDATION_ERROR")
	first, err := e.svc.DecideOffer(ctx(), e.approver, o.ID, DecisionApproved, str("ok"))
	if err != nil || first.Status != StatusPendingApproval {
		t.Fatalf("RG4 one of two approvals keeps it pending: %+v %v", first, err)
	}
	code(t, mustErr(e.svc.DecideOffer(ctx(), e.approver, o.ID, DecisionApproved, nil)), "ALREADY_DECIDED")
	last, err := e.svc.DecideOffer(ctx(), e.second, o.ID, DecisionApproved, nil)
	if err != nil || last.Status != StatusOffered {
		t.Fatalf("RG4 the second distinct approval releases it: %+v %v", last, err)
	}
	code(t, mustErr(e.svc.DecideOffer(ctx(), e.second, o.ID, DecisionApproved, nil)), "OFFER_STATE")
	if rows, err := e.svc.OfferApprovals(ctx(), e.hr, o.ID); err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.OfferApprovals(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	if e.store.events[len(e.store.events)-1] != "recruitment.offer.approved" {
		t.Fatalf("events: %v", e.store.events)
	}

	// Rejection withdraws the offer at once.
	r := e.offer(StatusPendingApproval, kernel.Rupees(3000000))
	rej, err := e.svc.DecideOffer(ctx(), e.approver, r.ID, DecisionRejected, str("too high"))
	if err != nil || rej.Status != StatusWithdrawn || rej.DecidedAt == nil {
		t.Fatal(rej, err)
	}

	// Failures roll back.
	f := e.offer(StatusPendingApproval, kernel.Rupees(3000000))
	for _, k := range []string{"offer", "approvals", "create", "saveoffer", "event"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.DecideOffer(ctx(), e.second, f.ID, DecisionApproved, nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
		if e.store.offers[f.ID].Status != StatusPendingApproval {
			t.Fatalf("%s: a failure leaves the offer pending", k)
		}
	}
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.DecideOffer(ctx(), e.second, f.ID, DecisionApproved, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "policy")
	e.store.fail["approvals"] = errBoom
	if _, err := e.svc.OfferApprovals(ctx(), e.hr, f.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestModelsAndHelpers(t *testing.T) {
	cs := Competencies{{Name: "A", WeightPct: 10000}}
	v, err := cs.Value()
	ok(t, err)
	var back Competencies
	if back.Scan(v) != nil || back.Scan([]byte(v.(string))) != nil || len(back) != 1 || back.Scan(1) == nil {
		t.Fatal("competencies")
	}
	for _, m := range []interface{ TableName() string }{Requisition{}, Scorecard{}, JobScorecard{}, InterviewScore{}, OfferPolicy{}, OfferApproval{}} {
		if !strings.HasPrefix(m.TableName(), "recruitment_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&Requisition{}, &Scorecard{}, &OfferApproval{}} {
		ok(t, m.BeforeCreate(nil))
	}
	m := NewModule(nil, Ports{})
	if len(Catalogue()) != 1 || len(m.RegisterModels()) != 6 || m.Service() == nil || NewStore(nil).DB() != nil || orNotFound(errBoom) != errBoom {
		t.Fatal("module")
	}
}

// --- HTTP edge ---

type guards struct{ deny map[string]bool }

func (g guards) TenantResolver() gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
func (g guards) Authenticate() gin.HandlerFunc   { return func(c *gin.Context) { c.Next() } }
func (g guards) Require(_, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.deny[action] {
			c.AbortWithStatusJSON(403, gin.H{"error": gin.H{"code": "FORBIDDEN"}})
			return
		}
		c.Next()
	}
}
func (g guards) RequireScoped(r, a string, _ access.Scope) gin.HandlerFunc { return g.Require(r, a) }
func (g guards) Flag(_, _, key string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(key, true); c.Next() }
}
func (g guards) AuditTrail(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }

var _ identityport.HTTP = guards{}

func router(e *env, user uuid.UUID, deny ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if user != uuid.Nil {
			c.Set("tenant_id", e.tenant)
			c.Set("user_id", user)
		}
	})
	g := guards{deny: map[string]bool{}}
	for _, d := range deny {
		g.deny[d] = true
	}
	(&Module{svc: e.svc}).RegisterRoutes(r.Group(""), g)
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHTTP(t *testing.T) {
	e := setup(t)
	hr, approver := router(e, e.hr.UserID), router(e, e.approver.UserID)
	worker := router(e, e.interviewer.UserID, "read", "manage", "hire", "approve")
	const B = "/recruitment"
	expect := func(w *httptest.ResponseRecorder, status int, label string) string {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	idOf := func(body string) string { return body[strings.Index(body, `"id":"`)+6:][:36] }
	expect(do(router(e, uuid.Nil), "GET", B+"/requisitions", ""), 401, "unauthenticated")

	out := expect(do(hr, "POST", B+"/requisitions", `{"position_id":"`+e.position.String()+`","headcount":1,"justification":"grow"}`), 201, "requisition")
	rid := idOf(out)
	expect(do(hr, "POST", B+"/requisitions", "{"), 400, "bad json")
	expect(do(hr, "GET", B+"/requisitions?status=draft", ""), 200, "list")
	expect(do(hr, "GET", B+"/requisitions/"+rid, ""), 200, "get")
	expect(do(hr, "GET", B+"/requisitions/x", ""), 400, "bad id")
	expect(do(hr, "POST", B+"/requisitions/"+rid+"/submit", ""), 200, "submit")
	expect(do(hr, "POST", B+"/requisitions/"+rid+"/approve", ""), 403, "self approval")
	expect(do(approver, "POST", B+"/requisitions/"+rid+"/reject", ""), 400, "reject needs a comment")
	expect(do(approver, "POST", B+"/requisitions/"+rid+"/approve", `{"comment":"ok"}`), 200, "approve")
	expect(do(approver, "POST", B+"/requisitions/"+rid+"/approve", "{"), 400, "bad body")
	expect(do(hr, "POST", B+"/requisitions/"+rid+"/cancel", ""), 409, "cancel approved")
	expect(do(worker, "POST", B+"/requisitions", `{}`), 403, "worker cannot manage")

	out = expect(do(hr, "POST", B+"/scorecards", `{"name":"Backend","competencies":[{"name":"Coding","weight_pct":60},{"name":"Communication","weight_pct":40}]}`), 201, "scorecard")
	cid := idOf(out)
	expect(do(hr, "POST", B+"/scorecards", "{"), 400, "bad scorecard")
	expect(do(hr, "GET", B+"/scorecards", ""), 200, "scorecards")
	expect(do(hr, "GET", B+"/scorecards/"+cid, ""), 200, "scorecard")
	expect(do(hr, "PATCH", B+"/scorecards/"+cid, `{"name":"Backend v2"}`), 200, "patch")
	expect(do(hr, "PATCH", B+"/scorecards/"+cid, "{"), 400, "bad patch")
	job, cand, iv := e.interviewFor(t)
	expect(do(hr, "PUT", B+"/jobs/"+job.String()+"/scorecard", `{"scorecard_id":"`+cid+`"}`), 200, "assign")
	expect(do(hr, "PUT", B+"/jobs/"+job.String()+"/scorecard", "{"), 400, "bad assign")
	expect(do(worker, "PUT", B+"/interviews/"+iv.String()+"/scores", `{"scores":[{"competency":"Coding","score":5},{"competency":"Communication","score":4}]}`), 200, "scores")
	expect(do(worker, "PUT", B+"/interviews/"+iv.String()+"/scores", "{"), 400, "bad scores")
	expect(do(hr, "GET", B+"/candidates/"+cand.String()+"/scorecard", ""), 200, "candidate scorecard")

	expect(do(hr, "PUT", B+"/offer-policy", `{"required_approvals":1,"threshold_ctc":0}`), 200, "policy")
	expect(do(hr, "PUT", B+"/offer-policy", "{"), 400, "bad policy")
	expect(do(hr, "GET", B+"/offer-policy", ""), 200, "get policy")
	o := e.offer(StatusPendingApproval, kernel.Rupees(100))
	expect(do(approver, "POST", B+"/offers/"+o.ID.String()+"/approve", `{"comment":"ok"}`), 200, "approve offer")
	expect(do(approver, "POST", B+"/offers/"+o.ID.String()+"/reject", "{"), 400, "bad body")
	expect(do(hr, "GET", B+"/offers/"+o.ID.String()+"/approvals", ""), 200, "approvals")

	held := models.Candidate{ID: uuid.New(), TenantID: e.tenant, JobID: job, SeatReserved: true}
	e.store.cands[held.ID] = held
	expect(do(hr, "POST", B+"/candidates/"+held.ID.String()+"/hire/release", ""), 204, "release")
	expect(do(hr, "POST", B+"/candidates/"+uuid.NewString()+"/hire/release", ""), 404, "release unknown")
	e.store.fail["reqs"] = errBoom
	expect(do(hr, "GET", B+"/requisitions", ""), 500, "internal")
}
