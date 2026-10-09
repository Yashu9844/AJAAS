package controllers

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/services"
)

type env struct {
	st      *stub
	r       *gin.Engine
	tenant  uuid.UUID
	user    uuid.UUID
	noActor bool
}

func newEnv() *env {
	gin.SetMode(gin.TestMode)
	e := &env{st: &stub{}, r: gin.New(), tenant: uuid.New(), user: uuid.New()}
	e.r.Use(func(c *gin.Context) {
		if !e.noActor {
			c.Set("tenant_id", e.tenant)
			c.Set("user_id", e.user)
		}
		c.Next()
	})
	pol := NewPolicyController(typeStub{e.st}, holidayStub{e.st})
	bal := NewBalanceController(balanceStub{e.st})
	req := NewRequestController(requestStub{e.st})
	e.r.POST("/types", pol.CreateType)
	e.r.GET("/types", pol.ListTypes)
	e.r.GET("/types/:id", pol.GetType)
	e.r.PATCH("/types/:id", pol.UpdateType)
	e.r.POST("/types/:id/deactivate", pol.DeactivateType)
	e.r.POST("/holidays", pol.CreateHoliday)
	e.r.GET("/holidays", pol.ListHolidays)
	e.r.DELETE("/holidays/:id", pol.DeleteHoliday)
	e.r.GET("/balances/me", bal.Mine)
	e.r.GET("/balances", bal.ForEmployee)
	e.r.POST("/balances/adjust", bal.Adjust)
	e.r.GET("/ledger", bal.Ledger)
	e.r.POST("/requests/preview", req.Preview)
	e.r.POST("/requests", req.Apply)
	e.r.GET("/requests/me", req.ListMine)
	e.r.GET("/requests", req.List)
	e.r.GET("/requests/:id", req.Get)
	e.r.POST("/requests/:id/cancel", req.Cancel)
	e.r.POST("/requests/:id/approve", req.Approve)
	e.r.POST("/requests/:id/reject", req.Reject)
	return e
}

type resp struct {
	code int
	Data json.RawMessage `json:"data"`
	Meta *dto.PageMeta   `json:"meta"`
	Err  *errorBody      `json:"error"`
}

func (e *env) do(method, path, body string) resp {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var out resp
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	out.code = w.Code
	return out
}

var (
	anyID      = uuid.NewString()
	applyBody  = `{"leave_type_id":"` + anyID + `","start_date":"2026-10-12","end_date":"2026-10-13","reason":"trip"}`
	adjustBody = `{"employee_id":"` + anyID + `","leave_type_id":"` + anyID + `","days":1.5,"reason":"fix"}`
)

func endpoints() []struct {
	m, p, b, call string
	status        int
} {
	return []struct {
		m, p, b, call string
		status        int
	}{
		{"POST", "/types", `{"name":"Earned","code":"EL","annual_allowance":12}`, "TypeCreate", 201},
		{"GET", "/types?status=active", "", "TypeList", 200},
		{"GET", "/types/" + anyID, "", "TypeGet", 200},
		{"PATCH", "/types/" + anyID, `{"name":"EL2"}`, "TypeUpdate", 200},
		{"POST", "/types/" + anyID + "/deactivate", "", "TypeDeactivate", 200},
		{"POST", "/holidays", `{"date":"2026-12-25","name":"Xmas"}`, "HolidayCreate", 201},
		{"GET", "/holidays?year=2026", "", "HolidayList", 200},
		{"DELETE", "/holidays/" + anyID, "", "HolidayDelete", 200},
		{"GET", "/balances/me", "", "BalanceMine", 200},
		{"GET", "/balances?employee_id=" + anyID, "", "BalanceFor", 200},
		{"POST", "/balances/adjust", adjustBody, "BalanceAdjust", 200},
		{"GET", "/ledger?employee_id=" + anyID, "", "Ledger", 200},
		{"POST", "/requests/preview", applyBody, "Preview", 200},
		{"POST", "/requests", applyBody, "Apply", 201},
		{"GET", "/requests/me?status=pending", "", "ListMine", 200},
		{"GET", "/requests?status=approved&from=2026-10-01", "", "List", 200},
		{"GET", "/requests/" + anyID, "", "Get", 200},
		{"POST", "/requests/" + anyID + "/cancel", "", "Cancel", 200},
		{"POST", "/requests/" + anyID + "/approve", "", "Approve", 200},
		{"POST", "/requests/" + anyID + "/reject", `{"comment":"no"}`, "Reject", 200},
	}
}

func TestEndpoints_HappyPaths(t *testing.T) {
	e := newEnv()
	for _, tc := range endpoints() {
		e.st.called = ""
		r := e.do(tc.m, tc.p, tc.b)
		if r.code != tc.status || e.st.called != tc.call || e.st.actor.TenantID != e.tenant {
			t.Errorf("%s %s: %d %q (err %+v)", tc.m, tc.p, r.code, e.st.called, r.Err)
		}
	}
	if e.st.review.Comment == nil || *e.st.review.Comment != "no" {
		t.Error("reject comment must reach the service")
	}
}

func TestEndpoints_ErrorsAndAuth(t *testing.T) {
	e := newEnv()
	e.st.err = services.ErrInsufficient
	if r := e.do("POST", "/requests", applyBody); r.code != 409 || r.Err.Code != "INSUFFICIENT_BALANCE" {
		t.Fatalf("app error: %+v", r)
	}
	e.st.err = errors.New("pq: secret 10.0.0.5")
	for _, tc := range endpoints() {
		if r := e.do(tc.m, tc.p, tc.b); r.code != 500 || strings.Contains(r.Err.Message, "10.0.0.5") {
			t.Errorf("%s %s: opaque 500 expected, got %d", tc.m, tc.p, r.code)
		}
	}
	e.st.err, e.noActor, e.st.called = nil, true, ""
	for _, tc := range endpoints() {
		if r := e.do(tc.m, tc.p, tc.b); r.code != 401 {
			t.Errorf("%s %s without actor: %d", tc.m, tc.p, r.code)
		}
	}
	if e.st.called != "" {
		t.Fatalf("no service may run without an actor: %q", e.st.called)
	}
}

func TestEndpoints_Validation(t *testing.T) {
	e := newEnv()
	r := e.do("POST", "/requests", `{"leave_type_id":"x","half_day":"evening"}`)
	if r.code != 400 || r.Err.Code != "VALIDATION_ERROR" || len(r.Err.Details) < 4 || e.st.called != "" {
		t.Fatalf("details: %+v", r.Err)
	}
	if r := e.do("POST", "/balances/adjust", `{"employee_id":"`+anyID+`","leave_type_id":"`+anyID+`","days":0.123,"reason":"x"}`); r.code != 400 {
		t.Fatalf("3-decimal days: %d", r.code)
	}
	if r := e.do("POST", "/requests", `{bad`); r.code != 400 || r.Err.Message != "malformed JSON body" {
		t.Fatalf("malformed: %+v", r.Err)
	}
	for _, p := range []struct{ m, path, b string }{
		{"GET", "/types/nope", ""}, {"PATCH", "/types/nope", `{}`}, {"POST", "/types/nope/deactivate", ""},
		{"DELETE", "/holidays/nope", ""}, {"GET", "/requests/nope", ""}, {"POST", "/requests/nope/cancel", ""},
		{"POST", "/requests/nope/approve", ""}, {"POST", "/requests/" + anyID + "/reject", `{"comment":""}`},
	} {
		if r := e.do(p.m, p.path, p.b); r.code != 400 {
			t.Errorf("%s %s: %d", p.m, p.path, r.code)
		}
	}
}

// TestEndpoints_G15_Pagination — golden G15 at the HTTP edge.
func TestEndpoints_G15_Pagination(t *testing.T) {
	e := newEnv()
	for _, q := range []string{"?per_page=0", "?page=-1&per_page=abc", "?per_page=100000"} {
		r := e.do("GET", "/requests"+q, "")
		if r.code != 200 || e.st.page.Page < 1 || e.st.page.PerPage < 1 || e.st.page.PerPage > 100 {
			t.Errorf("%s: %d %+v", q, r.code, e.st.page)
		}
	}
	if r := e.do("GET", "/requests/me?per_page=0", ""); r.Meta == nil || r.Meta.PerPage != 20 {
		t.Fatalf("meta clamp: %+v", r.Meta)
	}
}

// Self endpoints never take identity from the client (LS-T1).
func TestEndpoints_SelfIgnoresClientIdentity(t *testing.T) {
	e := newEnv()
	other := uuid.NewString()
	e.do("POST", "/requests", `{"leave_type_id":"`+anyID+`","start_date":"2026-10-12","end_date":"2026-10-12","reason":"x","employee_id":"`+other+`","status":"approved","total_days":99}`)
	if e.st.actor.UserID != e.user || e.st.applied.Reason != "x" {
		t.Fatal("apply must use the authenticated user")
	}
	e.do("GET", "/balances/me?employee_id="+other, "")
	if e.st.actor.UserID != e.user || len(e.st.args) != 0 {
		t.Fatal("balances/me must ignore employee_id")
	}
}
