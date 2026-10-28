package learning

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/shared/identityport"
)

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
func (g guards) Flag(_, action, key string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(key, !g.deny[action]); c.Next() }
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
	hr, ann, mgr := router(e, e.hr.user), router(e, e.ann.user, "read", "manage"), router(e, e.mgr.user, "read", "manage")
	expect := func(w *httptest.ResponseRecorder, status int, label string) string {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	idOf := func(body string) string {
		var out struct{ Data struct{ ID string } }
		_ = json.Unmarshal([]byte(body), &out)
		return out.Data.ID
	}
	const B = "/learning"
	expect(do(router(e, uuid.Nil), "GET", B+"/catalog", ""), 401, "unauthenticated")
	expect(do(ann, "POST", B+"/courses", `{}`), 403, "learner cannot manage")
	expect(do(ann, "GET", B+"/courses", ""), 403, "learner cannot read the admin catalogue")

	// catalogue
	expect(do(hr, "POST", B+"/courses", "{"), 400, "bad json")
	expect(do(hr, "POST", B+"/courses", `{"title":"x","content_kind":"link","content_url":"javascript:alert(1)"}`), 400, "unsafe url")
	out := expect(do(hr, "POST", B+"/courses", `{"title":"Fire safety","content_kind":"link","content_url":"https://learn.example.com/f","self_enrol":true,"verification":"manager"}`), 201, "course")
	cid := idOf(out)
	expect(do(hr, "GET", B+"/courses?status=draft", ""), 200, "list")
	expect(do(hr, "GET", B+"/courses?status=weird", ""), 400, "bad status")
	expect(do(hr, "GET", B+"/courses/"+cid, ""), 200, "get")
	expect(do(hr, "GET", B+"/courses/x", ""), 400, "bad id")
	expect(do(hr, "PATCH", B+"/courses/"+cid, `{"duration_mins":20}`), 200, "patch")
	expect(do(hr, "PATCH", B+"/courses/"+cid, "{"), 400, "bad patch")
	expect(do(hr, "GET", B+"/courses/"+cid+"/versions", ""), 200, "versions")
	expect(do(hr, "POST", B+"/courses/"+cid+"/publish", ""), 200, "publish")
	expect(do(hr, "POST", B+"/courses/"+cid+"/publish", ""), 409, "publish twice")
	expect(do(ann, "GET", B+"/catalog", ""), 200, "catalog")

	// plans
	out = expect(do(hr, "POST", B+"/plans", `{"name":"Onboarding"}`), 201, "plan")
	pid := idOf(out)
	expect(do(hr, "POST", B+"/plans", "{"), 400, "bad plan")
	expect(do(hr, "GET", B+"/plans", ""), 200, "plans")
	expect(do(hr, "GET", B+"/plans/"+pid, ""), 200, "plan")
	expect(do(hr, "PATCH", B+"/plans/"+pid, `{"description":"week one"}`), 200, "patch plan")
	expect(do(hr, "PUT", B+"/plans/"+pid+"/items", `{"items":[{"course_id":"`+cid+`"}]}`), 200, "items")
	expect(do(hr, "PUT", B+"/plans/"+pid+"/items", "{"), 400, "bad items")
	expect(do(ann, "GET", B+"/plans/"+pid+"/progress", ""), 200, "own progress")
	expect(do(ann, "GET", B+"/plans/"+pid+"/progress?employee_id="+e.bob.emp.String(), ""), 403, "others' progress")
	expect(do(ann, "GET", B+"/plans/"+pid+"/progress?employee_id=x", ""), 400, "bad employee id")

	// assignments
	out = expect(do(hr, "POST", B+"/assignments", `{"target_kind":"employee","target_id":"`+e.ann.emp.String()+`","course_id":"`+cid+`","due_in_days":14}`), 201, "assignment")
	var asn struct {
		Data struct{ Assignment struct{ ID string } }
	}
	_ = json.Unmarshal([]byte(out), &asn)
	aid := asn.Data.Assignment.ID
	expect(do(hr, "POST", B+"/assignments", "{"), 400, "bad assignment")
	expect(do(hr, "GET", B+"/assignments", ""), 200, "assignments")
	expect(do(hr, "GET", B+"/assignments/"+aid, ""), 200, "assignment")

	// enrolments
	var mine struct{ Data []struct{ ID string } }
	_ = json.Unmarshal([]byte(expect(do(ann, "GET", B+"/enrolments/me", ""), 200, "mine")), &mine)
	eid := mine.Data[0].ID
	expect(do(ann, "GET", B+"/enrolments/me?status=weird", ""), 400, "bad status")
	expect(do(mgr, "GET", B+"/enrolments/team", ""), 200, "team")
	expect(do(hr, "GET", B+"/enrolments?course_id="+cid+"&employee_id="+e.ann.emp.String()+"&assignment_id="+aid, ""), 200, "all")
	expect(do(hr, "GET", B+"/enrolments?course_id=x", ""), 400, "bad filter")
	expect(do(ann, "GET", B+"/enrolments", ""), 403, "learner cannot list all")
	expect(do(ann, "GET", B+"/enrolments/"+eid, ""), 200, "get enrolment")
	expect(do(ann, "POST", B+"/enrolments/"+eid+"/start", ""), 200, "start")
	expect(do(ann, "POST", B+"/enrolments/"+eid+"/complete", "{"), 400, "bad completion")
	out = expect(do(ann, "POST", B+"/enrolments/"+eid+"/complete", `{"score":90,"evidence_ref":"certs/a.pdf"}`), 200, "complete")
	var done struct {
		Data struct{ Completion struct{ ID, Status string } }
	}
	_ = json.Unmarshal([]byte(out), &done)
	if done.Data.Completion.Status != CompPending {
		t.Fatalf("manager-verified course stays pending: %s", out)
	}
	expect(do(mgr, "GET", B+"/completions/pending", ""), 200, "pending")
	expect(do(ann, "POST", B+"/completions/"+done.Data.Completion.ID+"/verify", ""), 403, "self verify")
	expect(do(mgr, "POST", B+"/completions/"+done.Data.Completion.ID+"/reject", ""), 400, "reject needs a comment")
	expect(do(mgr, "POST", B+"/completions/"+done.Data.Completion.ID+"/reject", `{"comment":"redo"}`), 200, "reject")
	expect(do(ann, "POST", B+"/enrolments/"+eid+"/complete", ""), 200, "resubmit")
	expect(do(mgr, "GET", B+"/completions/pending", ""), 200, "pending again")
	var pend struct{ Data []struct{ ID string } }
	_ = json.Unmarshal([]byte(do(mgr, "GET", B+"/completions/pending", "").Body.String()), &pend)
	expect(do(mgr, "POST", B+"/completions/"+pend.Data[0].ID+"/verify", ""), 200, "verify")
	expect(do(ann, "GET", B+"/transcript/me", ""), 200, "transcript")
	expect(do(hr, "POST", B+"/enrolments/"+eid+"/correct", `{"reason":"typo","score":95}`), 200, "correct")
	expect(do(hr, "POST", B+"/enrolments/"+eid+"/correct", "{"), 400, "bad correction")
	expect(do(hr, "POST", B+"/enrolments/"+eid+"/waive", `{"reason":"x"}`), 409, "waive a completed one")
	expect(do(hr, "POST", B+"/enrolments/"+eid+"/waive", "{"), 400, "bad waiver")
	expect(do(ann, "POST", B+"/enrolments/self", "{"), 400, "bad self enrol")
	expect(do(e2(e, e.bob.user), "POST", B+"/enrolments/self", `{"course_id":"`+cid+`"}`), 201, "self enrol")
	expect(do(e2(e, e.bob.user), "POST", B+"/enrolments/self", `{"course_id":"`+cid+`"}`), 409, "already")

	// assignment cancel, recurrence, reports, settings
	expect(do(hr, "POST", B+"/assignments/"+aid+"/cancel", ""), 200, "cancel")
	expect(do(hr, "POST", B+"/recurrence/run", ""), 200, "recurrence")
	expect(do(hr, "GET", B+"/reports/compliance?course_id="+cid+"&department_id="+e.dept.String()+"&mandatory=true", ""), 200, "compliance")
	expect(do(hr, "GET", B+"/reports/compliance?mandatory=maybe", ""), 400, "bad mandatory")
	expect(do(hr, "GET", B+"/reports/compliance?course_id=x", ""), 400, "bad course")
	expect(do(hr, "GET", B+"/reports/compliance?department_id=x", ""), 400, "bad department")
	expect(do(hr, "GET", B+"/settings", ""), 200, "settings")
	expect(do(hr, "PUT", B+"/settings", `{"time_zone":"Asia/Kolkata"}`), 200, "put settings")
	expect(do(hr, "PUT", B+"/settings", "{"), 400, "bad settings")
	expect(do(ann, "PUT", B+"/settings", `{"time_zone":"UTC"}`), 403, "learner cannot set the zone")

	// server errors and the background loop
	e.st.fail["courses"] = errBoom
	expect(do(hr, "GET", B+"/courses", ""), 500, "internal error")
	delete(e.st.fail, "courses")
	ctxLoop, cancel := context.WithCancel(context.Background())
	cancel()
	var seen error
	e.st.fail["due"] = errBoom
	(&Module{svc: e.svc}).RunRecurrence(ctxLoop, func(err error) { seen = err })
	if seen == nil {
		t.Fatal("the loop reports a failed run and stops with its context")
	}
	_ = time.Now
}

func e2(e *env, user uuid.UUID) *gin.Engine { return router(e, user, "read", "manage") }
