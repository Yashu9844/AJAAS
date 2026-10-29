package automation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

// doCall sends a request through the rule routes with the actor already authenticated.
func doCall(t *testing.T, e *env, a api.Actor, method, path string, body any, hdr ...string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("", func(c *gin.Context) { c.Set("tenant_id", a.TenantID); c.Set("user_id", a.UserID) })
	e.svc.Register(g)
	var rd bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&rd).Encode(body)
	}
	req := httptest.NewRequest(method, path, &rd)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestRoutes(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("HT")
	base := "/projects/" + f.p.ID.String() + "/rules"
	in := map[string]any{"name": "Via HTTP", "trigger": "task.created", "actions": []any{map[string]any{"type": "set_priority", "priority": "high"}}}
	if code, _ := doCall(t, e, f.owner, "POST", base, "{"); code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d", code)
	}
	code, out := doCall(t, e, f.owner, "POST", base, in)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	id := out["data"].(map[string]any)["id"].(string)
	if code, out = doCall(t, e, f.viewr, "GET", base, nil); code != http.StatusOK || len(out["data"].([]any)) != 1 {
		t.Fatalf("list: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "PATCH", base+"/"+id, map[string]any{"active": false}, "If-Match", `"1"`); code != http.StatusOK {
		t.Fatalf("patch: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "PATCH", base+"/"+id, map[string]any{"active": true}, "If-Match", `"1"`); code != http.StatusConflict {
		t.Fatalf("stale If-Match: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "PATCH", base+"/"+id, "{"); code != http.StatusBadRequest {
		t.Fatalf("patch body: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "PATCH", base+"/bad", map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("rule id: %d", code)
	}
	if code, out = doCall(t, e, f.owner, "GET", base+"/"+id+"/runs?limit=5", nil); code != http.StatusOK || out["meta"] == nil {
		t.Fatalf("runs: %d", code)
	}
	task := e.task(f.p, "todo", "none", f.owner.UserID)
	if code, out = doCall(t, e, f.owner, "POST", base+"/"+id+"/test", map[string]any{"task_id": task}); code != http.StatusOK || out["data"].(map[string]any)["matches"] != true {
		t.Fatalf("test: %d %v", code, out)
	}
	if code, _ = doCall(t, e, f.owner, "POST", base+"/"+id+"/test", "{"); code != http.StatusBadRequest {
		t.Fatalf("test body: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "GET", "/projects/bad/rules", nil); code != http.StatusBadRequest {
		t.Fatalf("project id: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "DELETE", base+"/"+id, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = doCall(t, e, f.owner, "DELETE", base+"/"+uuid.NewString(), nil); code != http.StatusNotFound {
		t.Fatalf("delete unknown: %d", code)
	}
	unauth := gin.New()
	e.svc.Register(unauth.Group(""))
	w := httptest.NewRecorder()
	unauth.ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no actor: %d", w.Code)
	}
}

func TestJSONTypes(t *testing.T) {
	var c Conditions
	if err := c.Scan(`[{"field":"priority","value":"low"}]`); err != nil || len(c) != 1 {
		t.Fatalf("scan string: %v", err)
	}
	if err := c.Scan([]byte(`[]`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Scan(5); err == nil {
		t.Fatal("bad source")
	}
	if v, _ := Conditions(nil).Value(); v != "[]" {
		t.Fatalf("nil conditions store an empty array: %v", v)
	}
	if v, _ := (Actions{{Type: ActionDue}}).Value(); v == "[]" {
		t.Fatal("actions are stored")
	}
	if v, _ := Actions(nil).Value(); v != "[]" {
		t.Fatal("nil actions")
	}
	var a Actions
	if err := a.Scan(`[{"type":"set_priority","priority":"low"}]`); err != nil || len(a) != 1 {
		t.Fatal(err)
	}
}
