package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/services"
	"gorm.io/gorm"
)

// stub implements every Module 5 service interface, recording the last call.
type stub struct {
	err    error
	called string
	actor  services.Actor
	page   dto.Page
	csv    []byte
}

func (s *stub) rec(name string, a services.Actor) { s.called, s.actor = name, a }

type structStub struct{ *stub }
type assignStub struct{ *stub }
type runStub struct{ *stub }
type slipStub struct{ *stub }

var (
	_ services.StructureService  = structStub{}
	_ services.AssignmentService = assignStub{}
	_ services.RunService        = runStub{}
	_ services.PayslipService    = slipStub{}
)

func ta(t uuid.UUID) services.Actor { return services.Actor{TenantID: t} }

func (s structStub) Create(_ context.Context, a services.Actor, _ dto.CreateStructureRequest) (*dto.StructureResponse, error) {
	s.rec("StructureCreate", a)
	return &dto.StructureResponse{}, s.err
}
func (s structStub) Get(_ context.Context, t, _ uuid.UUID) (*dto.StructureResponse, error) {
	s.rec("StructureGet", ta(t))
	return &dto.StructureResponse{}, s.err
}
func (s structStub) List(_ context.Context, t uuid.UUID, _ string, p dto.Page) ([]dto.StructureResponse, dto.PageMeta, error) {
	s.rec("StructureList", ta(t))
	s.page = p
	return nil, p.Meta(0), s.err
}
func (s structStub) Update(_ context.Context, a services.Actor, _ uuid.UUID, _ dto.UpdateStructureRequest) (*dto.StructureResponse, error) {
	s.rec("StructureUpdate", a)
	return &dto.StructureResponse{}, s.err
}
func (s structStub) Deactivate(_ context.Context, a services.Actor, _ uuid.UUID) (*dto.StructureResponse, error) {
	s.rec("StructureDeactivate", a)
	return &dto.StructureResponse{}, s.err
}
func (s assignStub) Assign(_ context.Context, a services.Actor, _ dto.AssignRequest) (*dto.AssignmentResponse, error) {
	s.rec("Assign", a)
	return &dto.AssignmentResponse{}, s.err
}
func (s assignStub) List(_ context.Context, t uuid.UUID, _ string, p dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error) {
	s.rec("AssignList", ta(t))
	return nil, p.Meta(0), s.err
}
func (s assignStub) Mine(_ context.Context, a services.Actor) (*dto.MyAssignmentResponse, error) {
	s.rec("AssignMine", a)
	return &dto.MyAssignmentResponse{}, s.err
}
func (s assignStub) Preview(_ context.Context, t uuid.UUID, _ dto.PreviewRequest) (*dto.BreakdownResponse, error) {
	s.rec("Preview", ta(t))
	return &dto.BreakdownResponse{}, s.err
}
func (s runStub) Create(_ context.Context, a services.Actor, _ dto.CreateRunRequest) (*dto.RunResponse, error) {
	s.rec("RunCreate", a)
	return &dto.RunResponse{}, s.err
}
func (s runStub) Get(_ context.Context, t, _ uuid.UUID) (*dto.RunResponse, error) {
	s.rec("RunGet", ta(t))
	return &dto.RunResponse{}, s.err
}
func (s runStub) List(_ context.Context, t uuid.UUID, _ string, p dto.Page) ([]dto.RunResponse, dto.PageMeta, error) {
	s.rec("RunList", ta(t))
	s.page = p
	return nil, p.Meta(0), s.err
}
func (s runStub) Calculate(_ context.Context, a services.Actor, _ uuid.UUID) (*dto.RunResponse, error) {
	s.rec("Calculate", a)
	return &dto.RunResponse{}, s.err
}
func (s runStub) Approve(_ context.Context, a services.Actor, _ uuid.UUID) (*dto.RunResponse, error) {
	s.rec("Approve", a)
	return &dto.RunResponse{}, s.err
}
func (s runStub) Finalize(_ context.Context, a services.Actor, _ uuid.UUID) (*dto.RunResponse, error) {
	s.rec("Finalize", a)
	return &dto.RunResponse{}, s.err
}
func (s slipStub) ByRun(_ context.Context, t, _ uuid.UUID, p dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error) {
	s.rec("ByRun", ta(t))
	return nil, p.Meta(0), s.err
}
func (s slipStub) Get(_ context.Context, t, _ uuid.UUID) (*dto.PayslipResponse, error) {
	s.rec("SlipGet", ta(t))
	return &dto.PayslipResponse{}, s.err
}
func (s slipStub) Mine(_ context.Context, a services.Actor, p dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error) {
	s.rec("SlipMine", a)
	return nil, p.Meta(0), s.err
}
func (s slipStub) MineOne(_ context.Context, a services.Actor, _ uuid.UUID) (*dto.PayslipResponse, error) {
	s.rec("SlipMineOne", a)
	return &dto.PayslipResponse{}, s.err
}
func (s slipStub) PayoutCSV(_ context.Context, t, _ uuid.UUID) ([]byte, error) {
	s.rec("CSV", ta(t))
	return s.csv, s.err
}

type env struct {
	st           *stub
	r            *gin.Engine
	tenant, user uuid.UUID
	noActor      bool
}

func newEnv() *env {
	gin.SetMode(gin.TestMode)
	e := &env{st: &stub{csv: []byte("employee_code,employee_name,net_pay\n")}, r: gin.New(), tenant: uuid.New(), user: uuid.New()}
	e.r.Use(func(c *gin.Context) {
		if !e.noActor {
			c.Set("tenant_id", e.tenant)
			c.Set("user_id", e.user)
		}
		c.Next()
	})
	s := NewSetupController(structStub{e.st}, assignStub{e.st})
	r := NewRunController(runStub{e.st}, slipStub{e.st})
	for _, rt := range []struct {
		m, p string
		h    gin.HandlerFunc
	}{
		{"POST", "/structures", s.CreateStructure}, {"GET", "/structures", s.ListStructures}, {"GET", "/structures/:id", s.GetStructure},
		{"PATCH", "/structures/:id", s.UpdateStructure}, {"POST", "/structures/:id/deactivate", s.DeactivateStructure},
		{"POST", "/assignments", s.Assign}, {"GET", "/assignments", s.ListAssignments}, {"GET", "/assignments/me", s.MyAssignment},
		{"POST", "/assignments/preview", s.Preview}, {"POST", "/runs", r.CreateRun}, {"GET", "/runs", r.ListRuns},
		{"GET", "/runs/:id", r.GetRun}, {"POST", "/runs/:id/calculate", r.Calculate}, {"POST", "/runs/:id/approve", r.Approve},
		{"POST", "/runs/:id/finalize", r.Finalize}, {"GET", "/runs/:id/payslips", r.RunPayslips}, {"GET", "/runs/:id/payout.csv", r.PayoutCSV},
		{"GET", "/payslips/me", r.MyPayslips}, {"GET", "/payslips/me/:id", r.MyPayslip}, {"GET", "/payslips/:id", r.GetPayslip},
	} {
		e.r.Handle(rt.m, rt.p, rt.h)
	}
	return e
}

func (e *env) do(m, p, b string) (*httptest.ResponseRecorder, errorBody) {
	req := httptest.NewRequest(m, p, strings.NewReader(b))
	if b != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var out struct {
		Error errorBody `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out.Error
}

var id = uuid.NewString()

func endpoints() []struct {
	m, p, b, call string
	code          int
} {
	comps := `{"name":"Std","components":[{"code":"BASIC","name":"Basic","kind":"earning","calc":"percent_of_ctc","value":40}]}`
	return []struct {
		m, p, b, call string
		code          int
	}{
		{"POST", "/structures", comps, "StructureCreate", 201}, {"GET", "/structures", "", "StructureList", 200},
		{"GET", "/structures/" + id, "", "StructureGet", 200}, {"PATCH", "/structures/" + id, `{"name":"X"}`, "StructureUpdate", 200},
		{"POST", "/structures/" + id + "/deactivate", "", "StructureDeactivate", 200},
		{"POST", "/assignments", `{"employee_id":"` + id + `","structure_id":"` + id + `","annual_ctc":1200000,"effective_from":"2026-01-01"}`, "Assign", 201},
		{"GET", "/assignments?employee_id=" + id, "", "AssignList", 200}, {"GET", "/assignments/me", "", "AssignMine", 200},
		{"POST", "/assignments/preview", `{"structure_id":"` + id + `","annual_ctc":600000}`, "Preview", 200},
		{"POST", "/runs", `{"month":9,"year":2026}`, "RunCreate", 201}, {"GET", "/runs", "", "RunList", 200},
		{"GET", "/runs/" + id, "", "RunGet", 200}, {"POST", "/runs/" + id + "/calculate", "", "Calculate", 200},
		{"POST", "/runs/" + id + "/approve", "", "Approve", 200}, {"POST", "/runs/" + id + "/finalize", "", "Finalize", 200},
		{"GET", "/runs/" + id + "/payslips", "", "ByRun", 200}, {"GET", "/runs/" + id + "/payout.csv", "", "CSV", 200},
		{"GET", "/payslips/me", "", "SlipMine", 200}, {"GET", "/payslips/me/" + id, "", "SlipMineOne", 200},
		{"GET", "/payslips/" + id, "", "SlipGet", 200},
	}
}

func TestEndpoints(t *testing.T) {
	e := newEnv()
	for _, c := range endpoints() {
		e.st.called = ""
		w, eb := e.do(c.m, c.p, c.b)
		if w.Code != c.code || e.st.called != c.call || e.st.actor.TenantID != e.tenant {
			t.Errorf("%s %s: %d %q %+v", c.m, c.p, w.Code, e.st.called, eb)
		}
	}
	w, _ := e.do("GET", "/runs/"+id+"/payout.csv", "")
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("csv headers: %v", w.Header())
	}
}

func TestEndpoints_ErrorsAuthValidation(t *testing.T) {
	e := newEnv()
	e.st.err = errors.New("pq: internal 10.0.0.9")
	for _, c := range endpoints() {
		if w, eb := e.do(c.m, c.p, c.b); w.Code != 500 || strings.Contains(eb.Message, "10.0.0.9") {
			t.Errorf("%s %s: %d", c.m, c.p, w.Code)
		}
	}
	e.st.err = fmt.Errorf("x: %w", gorm.ErrDuplicatedKey)
	if w, _ := e.do("POST", "/runs", `{"month":9,"year":2026}`); w.Code != 409 {
		t.Fatalf("normalized conflict: %d", w.Code)
	}
	e.st.err, e.noActor, e.st.called = nil, true, ""
	for _, c := range endpoints() {
		if w, _ := e.do(c.m, c.p, c.b); w.Code != 401 {
			t.Errorf("%s %s without actor: %d", c.m, c.p, w.Code)
		}
	}
	if e.st.called != "" {
		t.Fatal("no service without actor")
	}
	e.noActor = false
	for _, c := range []struct{ m, p, b string }{
		{"POST", "/runs", `{"month":13,"year":2026}`}, {"POST", "/structures", `{"name":"x","components":[]}`},
		{"POST", "/assignments", `{"annual_ctc":1.234}`}, {"GET", "/runs/nope", ""}, {"PATCH", "/structures/nope", `{}`},
		{"POST", "/runs/nope/approve", ""}, {"GET", "/payslips/me/nope", ""}, {"GET", "/runs/nope/payout.csv", ""},
	} {
		if w, _ := e.do(c.m, c.p, c.b); w.Code != 400 {
			t.Errorf("%s %s: want 400 got %d", c.m, c.p, w.Code)
		}
	}
}

// G16 pagination at the edge.
func TestEndpoints_G16(t *testing.T) {
	e := newEnv()
	for _, q := range []string{"?per_page=0", "?page=-1&per_page=abc", "?per_page=99999"} {
		if w, _ := e.do("GET", "/runs"+q, ""); w.Code != 200 || e.st.page.PerPage < 1 || e.st.page.PerPage > 100 || e.st.page.Page < 1 {
			t.Errorf("%s: %d %+v", q, w.Code, e.st.page)
		}
	}
}
