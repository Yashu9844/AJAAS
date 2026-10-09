package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"github.com/jaas/jaas/internal/recruitment/services"
)

// stub implements every Module 6 service interface, recording the last call.
type stub struct {
	err    error
	called string
	page   dto.Page
	jobF   repositories.JobFilter
	candF  repositories.CandidateFilter
}

func (s *stub) rec(name string) { s.called = name }

type jobStub struct{ *stub }
type candStub struct{ *stub }
type ivStub struct{ *stub }
type offerStub struct{ *stub }

var (
	_ JobService       = jobStub{}
	_ CandidateService = candStub{}
	_ HireService      = candStub{}
	_ InterviewService = ivStub{}
	_ OfferService     = offerStub{}
)

func (s jobStub) Create(context.Context, services.Actor, dto.CreateJobRequest) (*dto.JobResponse, error) {
	s.rec("JobCreate")
	return &dto.JobResponse{}, s.err
}
func (s jobStub) Get(context.Context, uuid.UUID, uuid.UUID) (*dto.JobResponse, error) {
	s.rec("JobGet")
	return &dto.JobResponse{}, s.err
}
func (s jobStub) List(_ context.Context, _ uuid.UUID, f repositories.JobFilter, p dto.Page) ([]dto.JobResponse, dto.PageMeta, error) {
	s.rec("JobList")
	s.page, s.jobF = p, f
	return nil, p.Meta(0), s.err
}
func (s jobStub) Update(context.Context, services.Actor, uuid.UUID, dto.UpdateJobRequest) (*dto.JobResponse, error) {
	s.rec("JobUpdate")
	return &dto.JobResponse{}, s.err
}
func (s jobStub) SetStatus(context.Context, services.Actor, uuid.UUID, string) (*dto.JobResponse, error) {
	s.rec("JobStatus")
	return &dto.JobResponse{}, s.err
}
func (s candStub) Create(context.Context, services.Actor, dto.CreateCandidateRequest) (*dto.CandidateResponse, error) {
	s.rec("CandCreate")
	return &dto.CandidateResponse{}, s.err
}
func (s candStub) Get(context.Context, uuid.UUID, uuid.UUID) (*dto.CandidateResponse, error) {
	s.rec("CandGet")
	return &dto.CandidateResponse{}, s.err
}
func (s candStub) List(_ context.Context, _ uuid.UUID, f repositories.CandidateFilter, p dto.Page) ([]dto.CandidateResponse, dto.PageMeta, error) {
	s.rec("CandList")
	s.page, s.candF = p, f
	return nil, p.Meta(0), s.err
}
func (s candStub) Update(context.Context, services.Actor, uuid.UUID, dto.UpdateCandidateRequest) (*dto.CandidateResponse, error) {
	s.rec("CandUpdate")
	return &dto.CandidateResponse{}, s.err
}
func (s candStub) MoveStage(context.Context, services.Actor, uuid.UUID, dto.StageRequest) (*dto.CandidateResponse, error) {
	s.rec("CandStage")
	return &dto.CandidateResponse{}, s.err
}
func (s candStub) Hire(context.Context, services.Actor, uuid.UUID, dto.HireRequest) (*dto.HireResponse, error) {
	s.rec("Hire")
	return &dto.HireResponse{}, s.err
}
func (s ivStub) Schedule(context.Context, services.Actor, dto.ScheduleInterviewRequest) (*dto.InterviewResponse, error) {
	s.rec("IvSchedule")
	return &dto.InterviewResponse{}, s.err
}
func (s ivStub) ListByCandidate(_ context.Context, _, _ uuid.UUID, p dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error) {
	s.rec("IvList")
	s.page = p
	return nil, p.Meta(0), s.err
}
func (s ivStub) ListMine(_ context.Context, _ services.Actor, _ string, p dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error) {
	s.rec("IvMine")
	return nil, p.Meta(0), s.err
}
func (s ivStub) Feedback(context.Context, services.Actor, uuid.UUID, dto.FeedbackRequest) (*dto.InterviewResponse, error) {
	s.rec("IvFeedback")
	return &dto.InterviewResponse{}, s.err
}
func (s ivStub) Cancel(context.Context, services.Actor, uuid.UUID) (*dto.InterviewResponse, error) {
	s.rec("IvCancel")
	return &dto.InterviewResponse{}, s.err
}
func (s offerStub) Create(context.Context, services.Actor, dto.CreateOfferRequest) (*dto.OfferResponse, error) {
	s.rec("OfferCreate")
	return &dto.OfferResponse{}, s.err
}
func (s offerStub) List(_ context.Context, _, _ uuid.UUID, p dto.Page) ([]dto.OfferResponse, dto.PageMeta, error) {
	s.rec("OfferList")
	return nil, p.Meta(0), s.err
}
func (s offerStub) Decide(context.Context, services.Actor, uuid.UUID, string) (*dto.OfferResponse, error) {
	s.rec("OfferDecide")
	return &dto.OfferResponse{}, s.err
}
func (s offerStub) Withdraw(context.Context, services.Actor, uuid.UUID) (*dto.OfferResponse, error) {
	s.rec("OfferWithdraw")
	return &dto.OfferResponse{}, s.err
}

type route struct {
	method, path string
	handler      func(*Controller) gin.HandlerFunc
	body, call   string
	status       int
}

const (
	id   = "00000000-0000-0000-0000-000000000001"
	cand = "?candidate_id=" + id
)

var table = []route{
	{"POST", "/jobs", func(c *Controller) gin.HandlerFunc { return c.CreateJob }, `{"title":"SRE","headcount":1,"employment_type":"intern","description":"x"}`, "JobCreate", 201},
	{"GET", "/jobs", func(c *Controller) gin.HandlerFunc { return c.ListJobs }, "", "JobList", 200},
	{"GET", "/jobs/:id", func(c *Controller) gin.HandlerFunc { return c.GetJob }, "", "JobGet", 200},
	{"PATCH", "/jobs/:id", func(c *Controller) gin.HandlerFunc { return c.UpdateJob }, `{}`, "JobUpdate", 200},
	{"POST", "/jobs/:id/status", func(c *Controller) gin.HandlerFunc { return c.SetJobStatus }, `{"status":"open"}`, "JobStatus", 200},
	{"POST", "/candidates", func(c *Controller) gin.HandlerFunc { return c.CreateCandidate }, `{"job_id":"` + id + `","first_name":"A","last_name":"B","email":"a@x.io","source":"other"}`, "CandCreate", 201},
	{"GET", "/candidates", func(c *Controller) gin.HandlerFunc { return c.ListCandidates }, "", "CandList", 200},
	{"GET", "/candidates/:id", func(c *Controller) gin.HandlerFunc { return c.GetCandidate }, "", "CandGet", 200},
	{"PATCH", "/candidates/:id", func(c *Controller) gin.HandlerFunc { return c.UpdateCandidate }, `{}`, "CandUpdate", 200},
	{"POST", "/candidates/:id/stage", func(c *Controller) gin.HandlerFunc { return c.MoveStage }, `{"stage":"screening"}`, "CandStage", 200},
	{"POST", "/candidates/:id/hire", func(c *Controller) gin.HandlerFunc { return c.Hire }, `{"employee_code":"E-1"}`, "Hire", 200},
	{"POST", "/interviews", func(c *Controller) gin.HandlerFunc { return c.ScheduleInterview }, `{"candidate_id":"` + id + `","round_name":"R","interviewer_employee_id":"` + id + `","scheduled_at":"2030-01-01T10:00:00Z","duration_mins":30}`, "IvSchedule", 201},
	{"GET", "/interviews" + cand, func(c *Controller) gin.HandlerFunc { return c.ListInterviews }, "", "IvList", 200},
	{"GET", "/interviews/me", func(c *Controller) gin.HandlerFunc { return c.MyInterviews }, "", "IvMine", 200},
	{"POST", "/interviews/:id/feedback", func(c *Controller) gin.HandlerFunc { return c.Feedback }, `{"rating":4,"recommendation":"hire"}`, "IvFeedback", 200},
	{"POST", "/interviews/:id/cancel", func(c *Controller) gin.HandlerFunc { return c.CancelInterview }, "", "IvCancel", 200},
	{"POST", "/offers", func(c *Controller) gin.HandlerFunc { return c.CreateOffer }, `{"candidate_id":"` + id + `","offered_ctc":"1200000.00","joining_date":"2030-01-01"}`, "OfferCreate", 201},
	{"GET", "/offers" + cand, func(c *Controller) gin.HandlerFunc { return c.ListOffers }, "", "OfferList", 200},
	{"POST", "/offers/:id/decision", func(c *Controller) gin.HandlerFunc { return c.DecideOffer }, `{"decision":"accepted"}`, "OfferDecide", 200},
	{"POST", "/offers/:id/withdraw", func(c *Controller) gin.HandlerFunc { return c.WithdrawOffer }, "", "OfferWithdraw", 200},
}

func newCtl(s *stub) *Controller {
	return New(Services{Jobs: jobStub{s}, Candidates: candStub{s}, Hire: candStub{s}, Interviews: ivStub{s}, Offers: offerStub{s}})
}

// serve runs one handler; withActor sets the Module 0 context keys.
func serve(r route, ctl *Controller, path, body string, withActor bool) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	pattern := strings.SplitN(r.path, "?", 2)[0]
	e.Handle(r.method, pattern, func(c *gin.Context) {
		if withActor {
			c.Set("tenant_id", uuid.New())
			c.Set("user_id", uuid.New())
			c.Set("request_id", uuid.NewString())
		}
		r.handler(ctl)(c)
	})
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(r.method, strings.ReplaceAll(path, ":id", id), strings.NewReader(body)))
	return w
}

func errCode(w *httptest.ResponseRecorder) string {
	var out struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Error.Code
}

func TestControllers_HappyPathAndAuth(t *testing.T) {
	for _, r := range table {
		s := &stub{}
		if w := serve(r, newCtl(s), r.path, r.body, true); w.Code != r.status || s.called != r.call {
			t.Errorf("%s %s: %d %q (%s)", r.method, r.path, w.Code, s.called, w.Body.String())
		}
		s = &stub{}
		if w := serve(r, newCtl(s), r.path, r.body, false); w.Code != http.StatusUnauthorized || s.called != "" {
			t.Errorf("%s %s without actor: %d", r.method, r.path, w.Code)
		}
	}
}

func TestControllers_ServiceErrorsAndBadInput(t *testing.T) {
	for _, r := range table {
		s := &stub{err: services.ErrNotFound}
		if w := serve(r, newCtl(s), r.path, r.body, true); w.Code != http.StatusNotFound || errCode(w) != "NOT_FOUND" {
			t.Errorf("%s %s: %d", r.method, r.path, w.Code)
		}
		s = &stub{err: errors.New("pq: secret internals")}
		if w := serve(r, newCtl(s), r.path, r.body, true); w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
			t.Errorf("%s %s must be opaque 500: %s", r.method, r.path, w.Body.String())
		}
		if strings.Contains(r.path, ":id") {
			bad := strings.ReplaceAll(r.path, ":id", "nope")
			if w := serve(r, newCtl(&stub{}), bad, r.body, true); w.Code != 400 {
				t.Errorf("%s %s bad id: %d", r.method, bad, w.Code)
			}
		}
		if r.body != "" && r.body != "{}" {
			if w := serve(r, newCtl(&stub{}), r.path, `{"x":`, true); w.Code != 400 || errCode(w) != "VALIDATION_ERROR" {
				t.Errorf("%s %s malformed body: %d", r.method, r.path, w.Code)
			}
			if w := serve(r, newCtl(&stub{}), r.path, `{}`, true); w.Code != 400 {
				t.Errorf("%s %s empty body must fail validation: %d", r.method, r.path, w.Code)
			}
		}
	}
}

func TestControllers_QueryFilters(t *testing.T) {
	byCall := map[string]route{}
	for _, r := range table {
		byCall[r.call] = r
	}
	s := &stub{}
	r := byCall["JobList"]
	serve(r, newCtl(s), "/jobs?status=open&department_id="+id+"&page=2&per_page=500", "", true)
	if s.jobF.Status != "open" || s.jobF.DepartmentID == nil || s.page != (dto.Page{Page: 2, PerPage: 100}) {
		t.Fatalf("job filter %+v %+v", s.jobF, s.page)
	}
	s = &stub{}
	serve(byCall["CandList"], newCtl(s), "/candidates?job_id="+id+"&stage=offer", "", true)
	if s.candF.JobID == nil || s.candF.Stage != "offer" {
		t.Fatalf("candidate filter %+v", s.candF)
	}
	for call, path := range map[string]string{"JobList": "/jobs?department_id=x", "CandList": "/candidates?job_id=x",
		"IvList": "/interviews", "OfferList": "/offers?candidate_id=x"} {
		s = &stub{}
		if w := serve(byCall[call], newCtl(s), path, "", true); w.Code != 400 || s.called != "" {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}
