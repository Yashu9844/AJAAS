package recurrence

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

// call sends a request through the recurrence routes with the actor already authenticated.
func call(t *testing.T, e *env, a api.Actor, method, path string, body any, hdr ...string) (int, map[string]any) {
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
	base := "/projects/" + f.p.ID.String() + "/recurrences"
	if code, _ := call(t, e, f.owner, "POST", base, "{"); code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d", code)
	}
	code, out := call(t, e, f.owner, "POST", base, Input{TemplateTaskID: f.tpl, Frequency: Daily})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	id := out["data"].(map[string]any)["id"].(string)
	if code, out = call(t, e, f.viewr, "GET", base, nil); code != http.StatusOK || len(out["data"].([]any)) != 1 {
		t.Fatalf("list: %d", code)
	}
	if code, _ = call(t, e, f.owner, "PATCH", base+"/"+id, map[string]any{"active": false}, "If-Match", `"1"`); code != http.StatusOK {
		t.Fatalf("patch: %d", code)
	}
	if code, _ = call(t, e, f.owner, "PATCH", base+"/"+id, "{"); code != http.StatusBadRequest {
		t.Fatalf("patch body: %d", code)
	}
	if code, _ = call(t, e, f.owner, "PATCH", base+"/not-a-uuid", map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("patch id: %d", code)
	}
	if code, out = call(t, e, f.owner, "POST", base+"/"+id+"/run-now", nil); code != http.StatusCreated || out["data"].(map[string]any)["task_id"] == nil {
		t.Fatalf("run-now: %d %v", code, out)
	}
	if code, _ = call(t, e, f.owner, "GET", "/projects/bad/recurrences", nil); code != http.StatusBadRequest {
		t.Fatalf("project id: %d", code)
	}
	if code, _ = call(t, e, f.owner, "DELETE", base+"/"+id, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = call(t, e, f.owner, "DELETE", base+"/"+uuid.NewString(), nil); code != http.StatusNotFound {
		t.Fatalf("delete unknown: %d", code)
	}
	var unauth = gin.New()
	e.svc.Register(unauth.Group(""))
	w := httptest.NewRecorder()
	unauth.ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no actor: %d", w.Code)
	}
}

func TestOptDate(t *testing.T) {
	var u Update
	if err := json.Unmarshal([]byte(`{"until_on":"2026-05-01"}`), &u); err != nil || !u.UntilOn.Set || u.UntilOn.Val == nil {
		t.Fatalf("set: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"until_on":null}`), &u); err != nil || !u.UntilOn.Set {
		t.Fatalf("null: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"until_on":"nope"}`), &u); err == nil {
		t.Fatal("bad date")
	}
	if err := json.Unmarshal([]byte(`{"until_on":5}`), &u); err == nil {
		t.Fatal("bad type")
	}
}
