package controllers

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/services"
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
	att := NewAttendanceController(e.st, e.st)
	reg := NewRegularizationController(regStub{e.st})
	sh := NewShiftController(shiftStub{e.st}, assignStub{e.st})
	e.r.POST("/punch", att.Punch)
	e.r.GET("/today", att.Today)
	e.r.GET("/me", att.Mine)
	e.r.GET("/records", att.List)
	e.r.GET("/records/:id", att.Detail)
	e.r.GET("/summary", att.Summary)
	e.r.POST("/regs", reg.Create)
	e.r.GET("/regs/me", reg.ListMine)
	e.r.GET("/regs", reg.List)
	e.r.POST("/regs/:id/cancel", reg.Cancel)
	e.r.POST("/regs/:id/approve", reg.Approve)
	e.r.POST("/regs/:id/reject", reg.Reject)
	e.r.POST("/shifts", sh.Create)
	e.r.GET("/shifts", sh.List)
	e.r.GET("/shifts/:id", sh.Get)
	e.r.PATCH("/shifts/:id", sh.Update)
	e.r.POST("/shifts/:id/deactivate", sh.Deactivate)
	e.r.POST("/shifts/:id/assignments", sh.Assign)
	e.r.GET("/assignments", sh.ListAssignments)
	return e
}

type resp struct {
	code int
	Data json.RawMessage `json:"data"`
	Meta *dto.PageMeta   `json:"meta"`
	Err  *errorBody      `json:"error"`
}

func (e *env) do(method, path, body string) resp {
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
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

func TestEndpoints_HappyPathsAndStatusCodes(t *testing.T) {
	e := newEnv()
	id := uuid.New().String()
	cases := []struct {
		method, path, body string
		status             int
		call               string
	}{
		{"POST", "/punch", `{"type":"in"}`, 201, "Punch"},
		{"GET", "/today", "", 200, "Today"},
		{"GET", "/me?from=2026-10-01&to=2026-10-08", "", 200, "Mine"},
		{"GET", "/records?status=present", "", 200, "RecordList"},
		{"GET", "/records/" + id, "", 200, "Detail"},
		{"GET", "/summary?date=2026-10-08", "", 200, "Summary"},
		{"POST", "/regs", `{"attendance_date":"2026-10-07","requested_punch_in":"2026-10-07T09:00:00Z","requested_punch_out":"2026-10-07T18:00:00Z","reason":"x"}`, 201, "RegCreate"},
		{"GET", "/regs/me?status=pending", "", 200, "RegListMine"},
		{"GET", "/regs?status=pending", "", 200, "RegList"},
		{"POST", "/regs/" + id + "/cancel", "", 200, "RegCancel"},
		{"POST", "/regs/" + id + "/approve", "", 200, "RegApprove"},
		{"POST", "/regs/" + id + "/reject", `{"comment":"no"}`, 200, "RegReject"},
		{"POST", "/shifts", `{"name":"Day","start_time":"09:00","end_time":"18:00"}`, 201, "ShiftCreate"},
		{"GET", "/shifts", "", 200, "ShiftList"},
		{"GET", "/shifts/" + id, "", 200, "ShiftGet"},
		{"PATCH", "/shifts/" + id, `{"name":"Day 2"}`, 200, "ShiftUpdate"},
		{"POST", "/shifts/" + id + "/deactivate", "", 200, "ShiftDeactivate"},
		{"POST", "/shifts/" + id + "/assignments", `{"employee_id":"` + id + `","effective_from":"2026-10-01"}`, 201, "Assign"},
		{"GET", "/assignments?employee_id=" + id, "", 200, "AssignList"},
	}
	for _, tc := range cases {
		e.st.called = ""
		r := e.do(tc.method, tc.path, tc.body)
		if r.code != tc.status || e.st.called != tc.call {
			t.Errorf("%s %s: status %d call %q, want %d %q (err %+v)", tc.method, tc.path, r.code, e.st.called, tc.status, tc.call, r.Err)
		}
		if tc.call != "" && e.st.actor.TenantID != e.tenant {
			t.Errorf("%s: tenant not taken from context", tc.call)
		}
	}
}

func TestEndpoints_ServiceErrorsMapToEnvelope(t *testing.T) {
	e := newEnv()
	e.st.err = services.ErrAlreadyPunchedIn
	r := e.do("POST", "/punch", `{"type":"in"}`)
	if r.code != 409 || r.Err == nil || r.Err.Code != "ALREADY_PUNCHED_IN" {
		t.Fatalf("app error mapping: %+v", r)
	}
	e.st.err = errors.New("pq: connection refused at 10.0.0.5")
	r = e.do("GET", "/shifts", "")
	if r.code != 500 || r.Err.Code != "INTERNAL_ERROR" || strings.Contains(r.Err.Message, "10.0.0.5") {
		t.Fatalf("opaque errors must not leak internals: %+v", r)
	}
	paths := []struct{ m, p, b string }{
		{"GET", "/today", ""}, {"GET", "/me", ""}, {"GET", "/records", ""}, {"GET", "/records/" + uuid.NewString(), ""},
		{"GET", "/summary", ""}, {"POST", "/regs", `{"attendance_date":"2026-10-07","requested_punch_in":"2026-10-07T09:00:00Z","requested_punch_out":"2026-10-07T18:00:00Z","reason":"x"}`},
		{"GET", "/regs/me", ""}, {"GET", "/regs", ""}, {"POST", "/regs/" + uuid.NewString() + "/approve", ""},
		{"POST", "/shifts", `{"name":"D","start_time":"09:00","end_time":"18:00"}`}, {"GET", "/shifts/" + uuid.NewString(), ""},
		{"PATCH", "/shifts/" + uuid.NewString(), `{}`}, {"POST", "/shifts/" + uuid.NewString() + "/deactivate", ""},
		{"POST", "/shifts/" + uuid.NewString() + "/assignments", `{"employee_id":"` + uuid.NewString() + `","effective_from":"2026-10-01"}`},
		{"GET", "/assignments", ""},
	}
	for _, p := range paths {
		if r := e.do(p.m, p.p, p.b); r.code != 500 {
			t.Errorf("%s %s: service error must surface, got %d", p.m, p.p, r.code)
		}
	}
}

func TestEndpoints_ValidationEnvelope(t *testing.T) {
	e := newEnv()
	r := e.do("POST", "/punch", `{"type":"sideways","latitude":200}`)
	if r.code != 400 || r.Err.Code != "VALIDATION_ERROR" || len(r.Err.Details) != 2 {
		t.Fatalf("field details expected: %+v", r.Err)
	}
	fields := r.Err.Details[0].Field + "," + r.Err.Details[1].Field
	if fields != "type,latitude" || e.st.called != "" {
		t.Fatalf("fields = %s, service called = %q", fields, e.st.called)
	}
	r = e.do("POST", "/punch", `{bad json`)
	if r.code != 400 || r.Err.Message != "malformed JSON body" {
		t.Fatalf("malformed JSON: %+v", r.Err)
	}
	for _, p := range []string{"/records/nope", "/shifts/nope", "/regs/nope/cancel"} {
		method := "GET"
		if strings.HasSuffix(p, "cancel") {
			method = "POST"
		}
		if r := e.do(method, p, ""); r.code != 400 {
			t.Errorf("%s bad uuid: %d", p, r.code)
		}
	}
	if r := e.do("POST", "/regs/"+uuid.NewString()+"/reject", `{"comment":""}`); r.code != 400 {
		t.Errorf("empty comment must fail binding: %d", r.code)
	}
	if r := e.do("POST", "/shifts/nope/assignments", `{}`); r.code != 400 {
		t.Errorf("bad shift id on assign: %d", r.code)
	}
	if r := e.do("PATCH", "/shifts/nope", `{}`); r.code != 400 {
		t.Errorf("bad shift id on update: %d", r.code)
	}
	if r := e.do("POST", "/shifts/"+uuid.NewString()+"/deactivate", ""); r.code != 200 {
		t.Errorf("deactivate needs no body: %d", r.code)
	}
	if r := e.do("POST", "/shifts/nope/deactivate", ""); r.code != 400 {
		t.Errorf("bad id deactivate: %d", r.code)
	}
}

// TestEndpoints_G13_PaginationClamp — golden G13 at the HTTP edge.
func TestEndpoints_G13_PaginationClamp(t *testing.T) {
	e := newEnv()
	for _, q := range []string{"?per_page=0", "?page=-1&per_page=abc", "?per_page=100000"} {
		r := e.do("GET", "/records"+q, "")
		if r.code != 200 || e.st.page.Page < 1 || e.st.page.PerPage < 1 || e.st.page.PerPage > 100 {
			t.Errorf("%s: code %d page %+v", q, r.code, e.st.page)
		}
	}
	r := e.do("GET", "/regs/me?per_page=0", "")
	if r.code != 200 || r.Meta == nil || r.Meta.PerPage != 20 {
		t.Fatalf("meta must reflect clamp: %+v", r)
	}
}

func TestEndpoints_RequireActorContext(t *testing.T) {
	e := newEnv()
	e.noActor = true
	for _, p := range []struct{ m, path, body string }{
		{"POST", "/punch", `{"type":"in"}`}, {"GET", "/today", ""}, {"GET", "/me", ""}, {"GET", "/records", ""},
		{"GET", "/records/" + uuid.NewString(), ""}, {"GET", "/summary", ""}, {"POST", "/regs", `{}`}, {"GET", "/regs/me", ""},
		{"GET", "/regs", ""}, {"POST", "/regs/" + uuid.NewString() + "/cancel", ""}, {"POST", "/shifts", `{}`}, {"GET", "/shifts", ""},
		{"GET", "/shifts/" + uuid.NewString(), ""}, {"PATCH", "/shifts/" + uuid.NewString(), `{}`},
		{"POST", "/shifts/" + uuid.NewString() + "/deactivate", ""}, {"POST", "/shifts/" + uuid.NewString() + "/assignments", `{}`},
		{"GET", "/assignments", ""},
	} {
		if r := e.do(p.m, p.path, p.body); r.code != 401 {
			t.Errorf("%s %s without auth context: %d", p.m, p.path, r.code)
		}
	}
	if e.st.called != "" {
		t.Fatalf("no service may run without an actor, got %q", e.st.called)
	}
}

// Self endpoints never accept identifiers from the client (NFR-SEC002 / AS-T2).
func TestEndpoints_SelfEndpointsIgnoreClientIDs(t *testing.T) {
	e := newEnv()
	other := uuid.NewString()
	e.do("GET", "/me?employee_id="+other+"&user_id="+other, "")
	if e.st.actor.UserID != e.user {
		t.Fatal("self query must use the authenticated user")
	}
	e.do("POST", "/punch", `{"type":"in","employee_id":"`+other+`","user_id":"`+other+`","punch_time":"2020-01-01T00:00:00Z"}`)
	if e.st.actor.UserID != e.user || e.st.punchReq.Type != "in" {
		t.Fatal("punch must ignore client identity fields")
	}
}

func TestSnake(t *testing.T) {
	for in, want := range map[string]string{"Type": "type", "DeviceID": "device_id", "EmployeeID": "employee_id", "RequestedPunchIn": "requested_punch_in"} {
		if got := snake(in); got != want {
			t.Errorf("snake(%q) = %q, want %q", in, got, want)
		}
	}
}
