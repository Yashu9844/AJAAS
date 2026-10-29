package webhooks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

type httpEnv struct {
	*env
	r *gin.Engine
}

func newHTTPEnv(t *testing.T) *httpEnv { return newHTTPEnvFrom(newEnv(t)) }

func newHTTPEnvFrom(e *env) *httpEnv {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1", func(c *gin.Context) {
		uid, err := uuid.Parse(c.GetHeader("X-User"))
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("tenant_id", e.tenant)
		c.Set("user_id", uid)
		c.Next()
	})
	e.svc.Register(g)
	e.svc.RegisterPublic(r.Group("/api/v1"))
	return &httpEnv{env: e, r: r}
}

type reply struct {
	Code int
	Raw  string
	Body map[string]any
}

func (r reply) str(key string) string {
	data, _ := r.Body["data"].(map[string]any)
	s, _ := data[key].(string)
	return s
}

func (r reply) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

// doRaw posts to the public route (no user) from a fixed client IP.
func (h *httpEnv) doRaw(path string, body []byte, headers ...string) reply {
	req := httptest.NewRequest(http.MethodPost, "/api/v1"+path, bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.50:4000"
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	out := reply{Code: w.Code, Raw: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

func (h *httpEnv) do(a api.Actor, method, path string, body any, headers ...string) reply {
	var rd io.Reader = http.NoBody
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if a.UserID != uuid.Nil {
		req.Header.Set("X-User", a.UserID.String())
	}
	req.RemoteAddr = "203.0.113.9:4444"
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	out := reply{Code: w.Code, Raw: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

func TestWebhookRoutes(t *testing.T) {
	h := newHTTPEnv(t)
	owner := h.user(false)
	p := h.project(owner, "RTE")
	base := "/projects/" + p.ID.String() + "/webhooks"

	if r := h.do(owner, "POST", base, map[string]any{"name": "x", "url": "http://10.0.0.1/"}); r.Code != 400 || r.errCode() != "VALIDATION_ERROR" {
		t.Fatalf("private url: %d %s", r.Code, r.Raw)
	}
	if r := h.do(owner, "POST", base, []byte("{")); r.Code != 400 {
		t.Fatalf("malformed json: %d", r.Code)
	}
	created := h.do(owner, "POST", base, map[string]any{"name": "ci", "url": "https://hooks.example.com/in", "event_types": []string{"task.created"}})
	if created.Code != 201 || !strings.HasPrefix(created.str("secret"), "whsec_") {
		t.Fatalf("create: %d %s", created.Code, created.Raw)
	}
	id := created.str("id")
	if r := h.do(owner, "GET", base, nil); r.Code != 200 || strings.Contains(r.Raw, "whsec_") || !strings.Contains(r.Raw, `"secret_hint"`) {
		t.Fatalf("list never shows the secret: %d %s", r.Code, r.Raw)
	}
	if r := h.do(owner, "PATCH", base+"/"+id, map[string]any{"name": "ci2", "active": false}); r.Code != 200 || r.str("name") != "ci2" {
		t.Fatalf("patch: %d %s", r.Code, r.Raw)
	}
	if r := h.do(owner, "PATCH", base+"/not-a-uuid", map[string]any{}); r.Code != 400 {
		t.Fatalf("bad id: %d", r.Code)
	}
	if r := h.do(owner, "POST", base+"/"+id+"/rotate-secret", nil); r.Code != 200 || r.str("secret") == created.str("secret") || r.str("secret") == "" {
		t.Fatalf("rotate: %d %s", r.Code, r.Raw)
	}
	h.doer.def = Result{Status: 200}
	h.do(owner, "PATCH", base+"/"+id, map[string]any{"active": true})
	ping := h.do(owner, "POST", base+"/"+id+"/test", nil)
	if ping.Code != 200 || ping.str("status") != StatusDelivered || ping.str("event_type") != pingEvent {
		t.Fatalf("test ping: %d %s", ping.Code, ping.Raw)
	}
	list := h.do(owner, "GET", base+"/"+id+"/deliveries?limit=1", nil)
	if list.Code != 200 || !strings.Contains(list.Raw, `"next_cursor":null`) || strings.Contains(list.Raw, "hooks.example.com") {
		t.Fatalf("deliveries: %d %s", list.Code, list.Raw)
	}
	did := ping.str("id")
	if r := h.do(owner, "POST", base+"/"+id+"/deliveries/"+did+"/redeliver", nil); r.Code != 200 || r.str("status") != StatusPending {
		t.Fatalf("redeliver: %d %s", r.Code, r.Raw)
	}
	if r := h.do(owner, "POST", base+"/"+id+"/deliveries/zzz/redeliver", nil); r.Code != 400 {
		t.Fatalf("bad delivery id: %d", r.Code)
	}
	stranger := h.user(false)
	if r := h.do(stranger, "GET", base, nil); r.Code != 404 {
		t.Fatalf("outsider: %d", r.Code)
	}
	if r := h.do(owner, "DELETE", base+"/"+id, nil); r.Code != 200 {
		t.Fatalf("delete: %d", r.Code)
	}
	if r := h.do(owner, "DELETE", base+"/"+id, nil); r.Code != 404 {
		t.Fatalf("delete twice: %d", r.Code)
	}
	if r := h.do(api.Actor{}, "GET", base, nil); r.Code != 401 {
		t.Fatalf("unauthenticated: %d", r.Code)
	}
}

func TestIntegrationRoutesAndTaskLinks(t *testing.T) {
	h := newHTTPEnv(t)
	owner := h.user(false)
	p := h.project(owner, "ITG")
	tid := h.task(owner, p, 1)
	base := "/projects/" + p.ID.String() + "/integrations"
	if r := h.do(owner, "POST", base, map[string]any{"provider": "svn", "name": "x"}); r.Code != 400 {
		t.Fatalf("provider: %d", r.Code)
	}
	if r := h.do(owner, "POST", base, []byte("{")); r.Code != 400 {
		t.Fatalf("json: %d", r.Code)
	}
	c := h.do(owner, "POST", base, map[string]any{"provider": "github", "name": "repo"})
	if c.Code != 201 || c.str("secret") == "" || !strings.HasPrefix(c.str("path"), "/integrations/github/") {
		t.Fatalf("create: %d %s", c.Code, c.Raw)
	}
	if r := h.do(owner, "POST", base, map[string]any{"provider": "gitea", "name": "REPO"}); r.Code != 409 || r.errCode() != "INTEGRATION_NAME_TAKEN" {
		t.Fatalf("duplicate: %d %s", r.Code, r.Raw)
	}
	if r := h.do(owner, "GET", base, nil); r.Code != 200 || strings.Contains(r.Raw, c.str("secret")) {
		t.Fatalf("list: %d", r.Code)
	}
	id := c.str("id")
	if r := h.do(owner, "POST", base+"/"+id+"/rotate-secret", nil); r.Code != 200 || r.str("secret") == c.str("secret") {
		t.Fatalf("rotate: %d %s", r.Code, r.Raw)
	}
	if r := h.do(owner, "GET", "/tasks/"+tid.String()+"/links", nil); r.Code != 200 || !strings.Contains(r.Raw, `"data":[]`) {
		t.Fatalf("links: %d %s", r.Code, r.Raw)
	}
	if r := h.do(h.user(false), "GET", "/tasks/"+tid.String()+"/links", nil); r.Code != 404 {
		t.Fatalf("links of a task the caller cannot read: %d", r.Code)
	}
	if r := h.do(owner, "DELETE", base+"/"+id, nil); r.Code != 200 {
		t.Fatalf("delete: %d", r.Code)
	}
	if r := h.do(owner, "DELETE", base+"/"+id, nil); r.Code != 404 {
		t.Fatalf("delete twice: %d", r.Code)
	}
}
