package benefits

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/identity/access"
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
	hr := router(e, e.hr.user)
	ann := router(e, e.ann.user, "read", "manage", "read_sensitive")
	bob := router(e, e.bob.user, "read", "manage", "read_sensitive")
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
	const B = "/benefits"
	expect(do(router(e, uuid.Nil), "GET", B+"/options", ""), 401, "unauthenticated")
	expect(do(ann, "POST", B+"/plans", `{}`), 403, "a worker cannot manage plans")
	expect(do(ann, "GET", B+"/plans", ""), 403, "a worker cannot list plans")

	// plans
	expect(do(hr, "POST", B+"/plans", "{"), 400, "bad json")
	expect(do(hr, "POST", B+"/plans", `{"code":"x"}`), 400, "invalid plan")
	out := expect(do(hr, "POST", B+"/plans", `{"code":"med-26","name":"Medical","category":"health","plan_year":2026,"effective_from":"2026-01-01","levels":[{"code":"SELF","name":"Employee","employee_cost":500,"employer_cost":1500,"max_dependants":0},{"code":"FAMILY","name":"Family","employee_cost":1200,"employer_cost":2800,"max_dependants":2}]}`), 201, "plan")
	pid := idOf(out)
	expect(do(hr, "GET", B+"/plans?status=draft", ""), 200, "plans")
	expect(do(hr, "GET", B+"/plans?status=weird", ""), 400, "bad status")
	expect(do(hr, "GET", B+"/plans/"+pid, ""), 200, "plan")
	expect(do(hr, "GET", B+"/plans/x", ""), 400, "bad id")
	expect(do(hr, "PATCH", B+"/plans/"+pid, `{"provider":"Acme"}`), 200, "patch")
	expect(do(hr, "PATCH", B+"/plans/"+pid, "{"), 400, "bad patch")
	expect(do(hr, "POST", B+"/plans/"+pid+"/versions", `{"effective_from":"2026-07-01","levels":[{"code":"SELF","name":"Employee","employee_cost":650,"employer_cost":1500}]}`), 201, "version")
	expect(do(hr, "POST", B+"/plans/"+pid+"/versions", "{"), 400, "bad version")
	expect(do(hr, "POST", B+"/plans/"+pid+"/activate", ""), 200, "activate")
	expect(do(hr, "POST", B+"/plans/"+pid+"/activate", ""), 409, "activate twice")

	// windows and settings
	expect(do(hr, "POST", B+"/windows", "{"), 400, "bad window")
	out = expect(do(hr, "POST", B+"/windows", `{"name":"Annual","starts_on":"2026-06-01","ends_on":"2026-06-30"}`), 201, "window")
	wid := idOf(out)
	expect(do(hr, "GET", B+"/windows", ""), 200, "windows")
	expect(do(hr, "GET", B+"/settings", ""), 200, "settings")
	expect(do(hr, "PUT", B+"/settings", `{"new_hire_days":45}`), 200, "put settings")
	expect(do(hr, "PUT", B+"/settings", "{"), 400, "bad settings")

	// elections, dependants, life events
	out = expect(do(ann, "GET", B+"/options", ""), 200, "options")
	if !strings.Contains(out, `"open":true`) {
		t.Fatalf("options: %s", out)
	}
	expect(do(ann, "POST", B+"/dependants", "{"), 400, "bad dependant")
	out = expect(do(ann, "POST", B+"/dependants", `{"name":"Asha","relationship":"child","date_of_birth":"2018-04-02"}`), 201, "dependant")
	did := idOf(out)
	expect(do(ann, "GET", B+"/dependants", ""), 200, "dependants")
	expect(do(ann, "PATCH", B+"/dependants/"+did, `{"name":"Asha K"}`), 200, "patch dependant")
	expect(do(ann, "PATCH", B+"/dependants/"+did, "{"), 400, "bad patch")
	expect(do(ann, "POST", B+"/elections", "{"), 400, "bad election")
	out = expect(do(ann, "POST", B+"/elections", `{"plan_id":"`+pid+`","level_code":"FAMILY","dependant_ids":["`+did+`"]}`), 201, "elect")
	eid := idOf(out)
	expect(do(ann, "POST", B+"/elections", `{"plan_id":"`+pid+`","level_code":"SELF"}`), 409, "twice")
	expect(do(ann, "GET", B+"/elections/me", ""), 200, "mine")
	expect(do(ann, "GET", B+"/deductions/me?month=2026-07", ""), 200, "deductions")
	expect(do(ann, "GET", B+"/deductions/me?month=July", ""), 400, "bad month")
	expect(do(ann, "POST", B+"/dependants/"+did+"/remove", ""), 409, "covered dependant")
	out = expect(do(ann, "POST", B+"/elections/"+eid+"/change", `{"level_code":"SELF"}`), 201, "change")
	eid2 := idOf(out)
	expect(do(ann, "POST", B+"/elections/"+eid2+"/change", "{"), 400, "bad change")
	expect(do(ann, "POST", B+"/elections/x/change", `{}`), 400, "bad id")
	expect(do(ann, "POST", B+"/elections/waive", "{"), 400, "bad waive")
	expect(do(ann, "POST", B+"/elections/waive", `{"plan_id":"`+pid+`"}`), 201, "waive")
	expect(do(ann, "POST", B+"/dependants/"+did+"/remove", ""), 200, "remove")
	expect(do(bob, "POST", B+"/life-events", "{"), 400, "bad life event")
	expect(do(bob, "POST", B+"/life-events", `{"type":"marriage","event_date":"2026-06-01"}`), 201, "life event")
	expect(do(bob, "GET", B+"/life-events", ""), 200, "my life events")
	expect(do(bob, "POST", B+"/elections/"+eid2+"/end", ""), 404, "someone else's election")
	// sensitive reads
	expect(do(ann, "GET", B+"/elections", ""), 403, "a worker cannot list elections")
	expect(do(ann, "GET", B+"/employees/"+e.bob.emp.String()+"/dependants", ""), 403, "BG4 no dependants without read_sensitive")
	expect(do(ann, "GET", B+"/employees/"+e.bob.emp.String()+"/life-events", ""), 403, "BG4 no life events without read_sensitive")
	expect(do(hr, "GET", B+"/elections?plan_id="+pid+"&employee_id="+e.ann.emp.String()+"&status=ended", ""), 200, "elections")
	expect(do(hr, "GET", B+"/elections?plan_id=x", ""), 400, "bad plan filter")
	expect(do(hr, "GET", B+"/elections?employee_id=x", ""), 400, "bad employee filter")
	expect(do(hr, "GET", B+"/employees/"+e.ann.emp.String()+"/dependants", ""), 200, "hr dependants")
	expect(do(hr, "GET", B+"/employees/x/dependants", ""), 400, "bad id")
	expect(do(hr, "GET", B+"/employees/"+e.bob.emp.String()+"/life-events", ""), 200, "hr life events")
	out = expect(do(hr, "GET", B+"/elections", ""), 200, "all elections")
	var list struct{ Data []struct{ ID, Status string } }
	_ = json.Unmarshal([]byte(out), &list)
	var open string
	for _, x := range list.Data {
		if x.Status == ElWaived {
			open = x.ID
		}
	}
	expect(do(hr, "POST", B+"/elections/"+open+"/end", ""), 409, "a waiver cannot be ended")
	expect(do(hr, "GET", B+"/reports/enrolment", ""), 200, "report")
	expect(do(ann, "GET", B+"/reports/enrolment", ""), 403, "a worker cannot read the report")
	expect(do(hr, "POST", B+"/windows/"+wid+"/cancel", ""), 200, "cancel window")
	expect(do(hr, "POST", B+"/plans/"+pid+"/retire", ""), 200, "retire")

	e.st.fail["plans"] = errBoom
	expect(do(hr, "GET", B+"/plans", ""), 500, "internal error")
	delete(e.st.fail, "plans")
	_ = strings.ToLower
}
