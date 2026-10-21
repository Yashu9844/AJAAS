package controllers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/controllers"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"github.com/jaas/jaas/internal/hr/core/routes"
	"github.com/jaas/jaas/internal/hr/core/services"
)

type base struct{ err error }

type archStub struct{ base }

func (s *archStub) CreateFamily(context.Context, services.Actor, dto.CreateFamilyRequest) (*dto.FamilyResponse, error) {
	return &dto.FamilyResponse{}, s.err
}
func (s *archStub) Families(context.Context, uuid.UUID, string, dto.Page) ([]dto.FamilyResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *archStub) UpdateFamily(context.Context, services.Actor, uuid.UUID, dto.UpdateArchitectureRequest) (*dto.FamilyResponse, error) {
	return &dto.FamilyResponse{}, s.err
}
func (s *archStub) CreateGrade(context.Context, services.Actor, dto.CreateGradeRequest) (*dto.GradeResponse, error) {
	return &dto.GradeResponse{}, s.err
}
func (s *archStub) Grades(context.Context, uuid.UUID, string, dto.Page) ([]dto.GradeResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *archStub) UpdateGrade(context.Context, services.Actor, uuid.UUID, dto.UpdateArchitectureRequest) (*dto.GradeResponse, error) {
	return &dto.GradeResponse{}, s.err
}
func (s *archStub) CreateProfile(context.Context, services.Actor, dto.CreateProfileRequest) (*dto.ProfileResponse, error) {
	return &dto.ProfileResponse{}, s.err
}
func (s *archStub) Profiles(context.Context, uuid.UUID, repositories.ProfileFilter, dto.Page) ([]dto.ProfileResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *archStub) Profile(context.Context, uuid.UUID, uuid.UUID) (*dto.ProfileResponse, error) {
	return &dto.ProfileResponse{}, s.err
}
func (s *archStub) UpdateProfile(context.Context, services.Actor, uuid.UUID, dto.UpdateArchitectureRequest) (*dto.ProfileResponse, error) {
	return &dto.ProfileResponse{}, s.err
}

type posStub struct {
	base
	vacant *bool
}

func (s *posStub) Create(context.Context, services.Actor, dto.CreatePositionRequest) (*dto.PositionResponse, error) {
	return &dto.PositionResponse{}, s.err
}
func (s *posStub) Get(context.Context, uuid.UUID, uuid.UUID, string) (*dto.PositionResponse, error) {
	return &dto.PositionResponse{}, s.err
}
func (s *posStub) List(_ context.Context, _ uuid.UUID, _ repositories.PositionFilter, v *bool, _ string, _ dto.Page) ([]dto.PositionResponse, dto.PageMeta, error) {
	s.vacant = v
	return nil, dto.PageMeta{}, s.err
}
func (s *posStub) Update(context.Context, services.Actor, uuid.UUID, dto.UpdatePositionRequest) (*dto.PositionResponse, error) {
	return &dto.PositionResponse{}, s.err
}
func (s *posStub) SetStatus(context.Context, services.Actor, uuid.UUID, string) (*dto.PositionResponse, error) {
	return &dto.PositionResponse{}, s.err
}

type actStub struct{ base }

func (s *actStub) Create(context.Context, services.Actor, dto.CreateActionRequest) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) Get(context.Context, uuid.UUID, uuid.UUID) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) List(context.Context, uuid.UUID, repositories.ActionFilter, dto.Page) ([]dto.ActionResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *actStub) Update(context.Context, services.Actor, uuid.UUID, dto.UpdateActionRequest) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) Submit(context.Context, services.Actor, uuid.UUID) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) Approve(context.Context, services.Actor, uuid.UUID, dto.DecisionRequest) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) Reject(context.Context, services.Actor, uuid.UUID, dto.DecisionRequest) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) Cancel(context.Context, services.Actor, uuid.UUID) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}
func (s *actStub) Rescind(context.Context, services.Actor, uuid.UUID, dto.DecisionRequest) (*dto.ActionResponse, error) {
	return &dto.ActionResponse{}, s.err
}

type workStub struct{ base }

func (s *workStub) Worker(context.Context, uuid.UUID, uuid.UUID, string) (*dto.WorkerResponse, error) {
	return &dto.WorkerResponse{}, s.err
}
func (s *workStub) Me(context.Context, services.Actor, string) (*dto.WorkerResponse, error) {
	return &dto.WorkerResponse{}, s.err
}
func (s *workStub) Assignments(context.Context, uuid.UUID, string, repositories.AssignmentFilter, dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *workStub) CreatePlan(context.Context, services.Actor, dto.CreatePlanRequest) (*dto.PlanResponse, error) {
	return &dto.PlanResponse{}, s.err
}
func (s *workStub) Plans(context.Context, uuid.UUID, *uuid.UUID, dto.Page) ([]dto.PlanResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *workStub) Headcount(context.Context, uuid.UUID, *uuid.UUID, string) (*dto.HeadcountResponse, error) {
	return &dto.HeadcountResponse{}, s.err
}
func (s *workStub) Backfill(context.Context, services.Actor) (*dto.RunResponse, error) {
	return &dto.RunResponse{}, s.err
}

type effStub struct{ base }

func (s *effStub) Sweep(context.Context, *uuid.UUID) (*dto.RunResponse, error) {
	return &dto.RunResponse{}, s.err
}

func router(err error, anonymous bool) (*gin.Engine, *posStub) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	auth := func(c *gin.Context) {
		if !anonymous {
			c.Set("tenant_id", uuid.New())
			c.Set("user_id", uuid.New())
		}
	}
	pos := &posStub{base: base{err}}
	routes.RegisterRoutes(e.Group("/hr"), routes.Middleware{TenantResolver: pass, Authenticate: auth, Require: func(string) gin.HandlerFunc { return pass }},
		controllers.New(controllers.Services{Architecture: &archStub{base{err}}, Positions: pos, Actions: &actStub{base{err}}, Workers: &workStub{base{err}},
			Effects: &effStub{base{err}}}))
	return e, pos
}

func call(e *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(w, req)
	return w
}

var id = uuid.NewString()

var ops = []struct {
	method, path, body string
	status             int
}{
	{"POST", "/hr/core/job-families", `{"code":"ENG","name":"Eng"}`, 201},
	{"GET", "/hr/core/job-families?status=active", "", 200},
	{"PATCH", "/hr/core/job-families/" + id, `{"name":"x"}`, 200},
	{"POST", "/hr/core/grades", `{"code":"L1","name":"L1","rank":1}`, 201},
	{"GET", "/hr/core/grades", "", 200},
	{"PATCH", "/hr/core/grades/" + id, `{"rank":2}`, 200},
	{"POST", "/hr/core/job-profiles", `{"code":"SWE","title":"SWE","family_id":"` + id + `","category":"staff"}`, 201},
	{"GET", "/hr/core/job-profiles?family_id=" + id, "", 200},
	{"GET", "/hr/core/job-profiles/" + id, "", 200},
	{"PATCH", "/hr/core/job-profiles/" + id, `{"title":"x"}`, 200},
	{"POST", "/hr/core/positions", `{"code":"P1","title":"P","job_profile_id":"` + id + `","department_id":"` + id + `"}`, 201},
	{"GET", "/hr/core/positions?department_id=" + id + "&job_profile_id=" + id + "&vacant=true", "", 200},
	{"GET", "/hr/core/positions/" + id + "?as_of=2026-01-01", "", 200},
	{"PATCH", "/hr/core/positions/" + id, `{"budgeted_fte":2}`, 200},
	{"POST", "/hr/core/positions/" + id + "/status", `{"status":"frozen"}`, 200},
	{"POST", "/hr/core/actions", `{"type":"hire","employee_id":"` + id + `","effective_date":"2026-01-01","reason_code":"x"}`, 201},
	{"GET", "/hr/core/actions?employee_id=" + id, "", 200},
	{"GET", "/hr/core/actions/" + id, "", 200},
	{"PATCH", "/hr/core/actions/" + id, `{"comment":"x"}`, 200},
	{"POST", "/hr/core/actions/" + id + "/submit", "", 200},
	{"POST", "/hr/core/actions/" + id + "/approve", `{"comment":"ok"}`, 200},
	{"POST", "/hr/core/actions/" + id + "/reject", "", 200},
	{"POST", "/hr/core/actions/" + id + "/cancel", "", 200},
	{"POST", "/hr/core/actions/" + id + "/rescind", `{"reason":"x"}`, 200},
	{"GET", "/hr/core/workers/me", "", 200},
	{"GET", "/hr/core/workers/" + id, "", 200},
	{"GET", "/hr/core/assignments?department_id=" + id + "&position_id=" + id, "", 200},
	{"POST", "/hr/core/headcount-plans", `{"department_id":"` + id + `","period_start":"2026-01-01","period_end":"2026-12-31","planned_fte":3}`, 201},
	{"GET", "/hr/core/headcount-plans?department_id=" + id, "", 200},
	{"GET", "/hr/core/headcount?department_id=" + id, "", 200},
	{"POST", "/hr/core/backfill", "", 200},
	{"POST", "/hr/core/effects/run", "", 200},
}

func TestControllers_EveryOperation(t *testing.T) {
	if len(ops) != 32 {
		t.Fatalf("32 operations, table has %d", len(ops))
	}
	e, pos := router(nil, false)
	for _, op := range ops {
		if w := call(e, op.method, op.path, op.body); w.Code != op.status || !strings.HasPrefix(w.Body.String(), `{"data"`) {
			t.Errorf("%s %s → %d %s", op.method, op.path, w.Code, w.Body.String())
		}
	}
	call(e, "GET", "/hr/core/positions?vacant=false", "")
	if pos.vacant == nil || *pos.vacant {
		t.Fatal("vacant=false reaches the service")
	}
	call(e, "GET", "/hr/core/positions", "")
	if pos.vacant != nil {
		t.Fatal("no vacant filter")
	}
}

func TestControllers_Errors(t *testing.T) {
	conflict := services.ErrInvalidState
	e, _ := router(conflict, false)
	for _, op := range ops {
		if w := call(e, op.method, op.path, op.body); w.Code != http.StatusConflict {
			t.Errorf("%s %s → %d", op.method, op.path, w.Code)
		}
	}
	e500, _ := router(errors.New("secret"), false)
	if w := call(e500, "GET", "/hr/core/grades", ""); w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("opaque 500: %s", w.Body.String())
	}
	anon, _ := router(nil, true)
	for _, op := range ops {
		if w := call(anon, op.method, op.path, op.body); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s → %d", op.method, op.path, w.Code)
		}
	}
}

func TestControllers_BadInput(t *testing.T) {
	e, _ := router(nil, false)
	for _, op := range ops {
		if strings.Contains(op.path, id) && !strings.Contains(op.path, "?") {
			if w := call(e, op.method, strings.Replace(op.path, id, "bad", 1), op.body); w.Code != 400 {
				t.Errorf("bad id %s %s → %d", op.method, op.path, w.Code)
			}
		}
		if op.body != "" {
			if w := call(e, op.method, op.path, "{"); w.Code != 400 {
				t.Errorf("bad body %s %s → %d", op.method, op.path, w.Code)
			}
		}
	}
	for _, q := range []string{"/hr/core/job-profiles?family_id=x", "/hr/core/positions?department_id=x", "/hr/core/positions?job_profile_id=x",
		"/hr/core/actions?employee_id=x", "/hr/core/assignments?department_id=x", "/hr/core/assignments?position_id=x",
		"/hr/core/headcount-plans?department_id=x", "/hr/core/headcount?department_id=x"} {
		if w := call(e, "GET", q, ""); w.Code != 400 {
			t.Errorf("bad query %s → %d", q, w.Code)
		}
	}
	if w := call(e, "POST", "/hr/core/positions/"+id+"/status", `{"status":"open"}`); w.Code != 400 {
		t.Errorf("status enum → %d", w.Code)
	}
}
