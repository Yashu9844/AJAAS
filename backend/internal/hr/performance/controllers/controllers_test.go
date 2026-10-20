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
	"github.com/jaas/jaas/internal/hr/performance/controllers"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/hr/performance/routes"
	"github.com/jaas/jaas/internal/hr/performance/services"
)

// stub implements every service interface; err (when set) is returned by all calls; seen records the last actor.
type stub struct {
	err  error
	seen services.Actor
	page dto.Page
}

func (s *stub) Create(_ context.Context, a services.Actor, _ dto.CreateCycleRequest) (*dto.CycleResponse, error) {
	s.seen = a
	return &dto.CycleResponse{}, s.err
}
func (s *stub) Get(context.Context, uuid.UUID, uuid.UUID) (*dto.CycleResponse, error) { return &dto.CycleResponse{}, s.err }
func (s *stub) List(_ context.Context, _ uuid.UUID, _ string, p dto.Page) ([]dto.CycleResponse, dto.PageMeta, error) {
	s.page = p
	return nil, p.Meta(0), s.err
}
func (s *stub) Update(context.Context, services.Actor, uuid.UUID, dto.UpdateCycleRequest) (*dto.CycleResponse, error) {
	return &dto.CycleResponse{}, s.err
}
func (s *stub) Activate(context.Context, services.Actor, uuid.UUID) (*dto.CycleResponse, error) { return &dto.CycleResponse{}, s.err }
func (s *stub) LaunchReviews(context.Context, services.Actor, uuid.UUID) (*dto.LaunchResponse, error) {
	return &dto.LaunchResponse{}, s.err
}
func (s *stub) Complete(context.Context, services.Actor, uuid.UUID) (*dto.CycleResponse, error) { return &dto.CycleResponse{}, s.err }
func (s *stub) NineBox(context.Context, uuid.UUID, uuid.UUID) (*dto.NineBoxResponse, error) {
	return &dto.NineBoxResponse{}, s.err
}

type goalStub struct{ stub }

func (s *goalStub) Create(_ context.Context, a services.Actor, _ dto.CreateGoalRequest) (*dto.GoalResponse, error) {
	s.seen = a
	return &dto.GoalResponse{}, s.err
}
func (s *goalStub) Get(context.Context, services.Actor, uuid.UUID) (*dto.GoalResponse, error) { return &dto.GoalResponse{}, s.err }
func (s *goalStub) List(context.Context, services.Actor, repositories.GoalFilter, dto.Page) ([]dto.GoalResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *goalStub) ListMine(context.Context, services.Actor, *uuid.UUID, dto.Page) ([]dto.GoalResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *goalStub) Update(context.Context, services.Actor, uuid.UUID, dto.UpdateGoalRequest) (*dto.GoalResponse, error) {
	return &dto.GoalResponse{}, s.err
}
func (s *goalStub) SetStatus(context.Context, services.Actor, uuid.UUID, string) (*dto.GoalResponse, error) {
	return &dto.GoalResponse{}, s.err
}
func (s *goalStub) AddKeyResult(context.Context, services.Actor, uuid.UUID, dto.KeyResultRequest) (*dto.GoalResponse, error) {
	return &dto.GoalResponse{}, s.err
}
func (s *goalStub) CheckIn(context.Context, services.Actor, uuid.UUID, dto.CheckInRequest) (*dto.GoalResponse, error) {
	return &dto.GoalResponse{}, s.err
}

type reviewStub struct{ stub }

func (s *reviewStub) Assign(context.Context, services.Actor, dto.AssignReviewerRequest) (*dto.ReviewResponse, error) {
	return &dto.ReviewResponse{}, s.err
}
func (s *reviewStub) List(context.Context, uuid.UUID, repositories.ReviewFilter, dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *reviewStub) ListAssigned(context.Context, services.Actor, string, dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *reviewStub) ListMine(context.Context, services.Actor, *uuid.UUID) ([]dto.ReviewResponse, error) { return nil, s.err }
func (s *reviewStub) Get(context.Context, services.Actor, uuid.UUID) (*dto.ReviewResponse, error) {
	return &dto.ReviewResponse{}, s.err
}
func (s *reviewStub) Submit(context.Context, services.Actor, uuid.UUID, dto.SubmitReviewRequest) (*dto.ReviewResponse, error) {
	return &dto.ReviewResponse{}, s.err
}

// router mounts the real routes; auth sets the actor unless anonymous; every permission check passes and the flags
// are set to read=true, manage=false.
func router(cy *stub, g *goalStub, r *reviewStub, anonymous bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	auth := func(c *gin.Context) {
		if !anonymous {
			c.Set("tenant_id", uuid.New())
			c.Set("user_id", uuid.New())
		}
	}
	routes.RegisterRoutes(e.Group("/hr"), routes.Middleware{TenantResolver: pass, Authenticate: auth,
		Require: func(string) gin.HandlerFunc { return pass },
		Flag: func(action, key string) gin.HandlerFunc {
			return func(c *gin.Context) { c.Set(key, action == routes.ActionRead) }
		}}, controllers.New(controllers.Services{Cycles: cy, Goals: g, Reviews: r}))
	return e
}

func call(e *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(w, req)
	return w
}

var id = uuid.NewString()

// every operation with a valid request and its success status (22 operations).
var ops = []struct {
	method, path, body string
	status             int
}{
	{"POST", "/hr/performance/cycles", `{"title":"t","start_date":"2026-01-01","end_date":"2026-02-01"}`, 201},
	{"GET", "/hr/performance/cycles?status=draft", "", 200},
	{"GET", "/hr/performance/cycles/" + id, "", 200},
	{"PATCH", "/hr/performance/cycles/" + id, `{"title":"x"}`, 200},
	{"POST", "/hr/performance/cycles/" + id + "/activate", "", 200},
	{"POST", "/hr/performance/cycles/" + id + "/launch-reviews", "", 200},
	{"POST", "/hr/performance/cycles/" + id + "/complete", "", 200},
	{"GET", "/hr/performance/cycles/" + id + "/nine-box", "", 200},
	{"POST", "/hr/performance/goals", `{"cycle_id":"` + id + `","title":"t","weight":10}`, 201},
	{"GET", "/hr/performance/goals?cycle_id=" + id + "&employee_id=" + id, "", 200},
	{"GET", "/hr/performance/goals/me?cycle_id=" + id, "", 200},
	{"GET", "/hr/performance/goals/" + id, "", 200},
	{"PATCH", "/hr/performance/goals/" + id, `{"weight":"12.5"}`, 200},
	{"POST", "/hr/performance/goals/" + id + "/status", `{"status":"completed"}`, 200},
	{"POST", "/hr/performance/goals/" + id + "/key-results", `{"title":"kr","start_value":0,"target_value":10}`, 201},
	{"POST", "/hr/performance/goals/" + id + "/check-ins", `{"progress":40}`, 201},
	{"POST", "/hr/performance/reviews", `{"cycle_id":"` + id + `","employee_id":"` + id + `","reviewer_employee_id":"` + id + `","type":"peer"}`, 201},
	{"GET", "/hr/performance/reviews?cycle_id=" + id, "", 200},
	{"GET", "/hr/performance/reviews/assigned", "", 200},
	{"GET", "/hr/performance/reviews/me", "", 200},
	{"GET", "/hr/performance/reviews/" + id, "", 200},
	{"POST", "/hr/performance/reviews/" + id + "/submit", `{"ratings":[{"criterion":"Quality","score":4}]}`, 200},
}

func TestControllers_EveryOperation(t *testing.T) {
	if len(ops) != 22 {
		t.Fatalf("22 operations, table has %d", len(ops))
	}
	cy, g, r := &stub{}, &goalStub{}, &reviewStub{}
	e := router(cy, g, r, false)
	for _, op := range ops {
		if w := call(e, op.method, op.path, op.body); w.Code != op.status || !strings.HasPrefix(w.Body.String(), `{"data"`) {
			t.Errorf("%s %s → %d %s", op.method, op.path, w.Code, w.Body.String())
		}
	}
	if !g.seen.CanRead || g.seen.CanManage {
		t.Fatalf("flags reach the actor: %+v", g.seen)
	}
	if w := call(e, "GET", "/hr/performance/cycles?page=0&per_page=500", ""); w.Code != 200 || cy.page.Page != 1 || cy.page.PerPage != 100 {
		t.Fatalf("G14 pagination clamped: %+v", cy.page)
	}
}

func TestControllers_Errors(t *testing.T) {
	cy, g, r := &stub{err: services.ErrCycleState}, &goalStub{stub{err: services.ErrCycleState}}, &reviewStub{stub{err: services.ErrCycleState}}
	e := router(cy, g, r, false)
	for _, op := range ops {
		if w := call(e, op.method, op.path, op.body); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "CYCLE_STATE") {
			t.Errorf("%s %s → %d %s", op.method, op.path, w.Code, w.Body.String())
		}
	}
	cy.err, g.err, r.err = errors.New("db down: secret"), errors.New("x"), errors.New("x")
	if w := call(e, "GET", "/hr/performance/cycles", ""); w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("opaque 500: %s", w.Body.String())
	}
	anon := router(&stub{}, &goalStub{}, &reviewStub{}, true)
	for _, op := range ops {
		if w := call(anon, op.method, op.path, op.body); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s → %d", op.method, op.path, w.Code)
		}
	}
}

func TestControllers_BadInput(t *testing.T) {
	e := router(&stub{}, &goalStub{}, &reviewStub{}, false)
	for _, op := range ops {
		if strings.Contains(op.path, id) && !strings.Contains(op.path, "?") {
			if w := call(e, op.method, strings.Replace(op.path, id, "not-a-uuid", 1), op.body); w.Code != 400 {
				t.Errorf("bad id %s %s → %d", op.method, op.path, w.Code)
			}
		}
		if op.body != "" {
			if w := call(e, op.method, op.path, "{"); w.Code != 400 || !strings.Contains(w.Body.String(), "VALIDATION_ERROR") {
				t.Errorf("malformed body %s %s → %d", op.method, op.path, w.Code)
			}
		}
	}
	for _, q := range []string{"/hr/performance/goals?cycle_id=x", "/hr/performance/goals?employee_id=x", "/hr/performance/goals/me?cycle_id=x",
		"/hr/performance/reviews?cycle_id=x", "/hr/performance/reviews?employee_id=x", "/hr/performance/reviews/me?cycle_id=x"} {
		if w := call(e, "GET", q, ""); w.Code != 400 {
			t.Errorf("bad query %s → %d", q, w.Code)
		}
	}
	for _, b := range []string{`{"status":"paused"}`, `{}`} {
		if w := call(e, "POST", "/hr/performance/goals/"+id+"/status", b); w.Code != 400 || !strings.Contains(w.Body.String(), "status") {
			t.Errorf("status rule %s → %d %s", b, w.Code, w.Body.String())
		}
	}
	if w := call(e, "POST", "/hr/performance/goals", `{"cycle_id":"x","title":"t","weight":"1.234"}`); w.Code != 400 {
		t.Errorf("weight with 3 decimals → %d", w.Code)
	}
}
