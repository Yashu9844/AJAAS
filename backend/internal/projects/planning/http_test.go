package planning

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

type httpEnv struct {
	*env
	r *gin.Engine
}

func newHTTPEnv(t *testing.T) *httpEnv {
	e := newEnv(t)
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

func (h *httpEnv) do(a api.Actor, method, path string, body any, headers ...string) reply {
	var rd io.Reader = http.NoBody
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", a.UserID.String())
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	out := reply{Code: w.Code, Raw: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

func (r reply) data() map[string]any { d, _ := r.Body["data"].(map[string]any); return d }
func (r reply) list() []any          { d, _ := r.Body["data"].([]any); return d }

func (h *httpEnv) want(r reply, code int) reply {
	h.t.Helper()
	if r.Code != code {
		h.t.Fatalf("status %d, want %d: %s", r.Code, code, r.Raw)
	}
	return r
}

func TestHTTPSprintAndPlanFlow(t *testing.T) {
	h := newHTTPEnv(t)
	owner, viewer := h.user("create"), h.user()
	p := h.scrum(owner, "WEB")
	h.join(owner, p, viewer, "viewer")
	todo := h.col(p, "todo")
	task := h.task(p, tk{col: todo, est: ptr(10)})

	h.want(h.do(owner, "GET", "/projects/"+p.ID.String()+"/sprints", nil, "X-User", ""), 401)
	h.want(h.do(owner, "POST", "/projects/not-a-uuid/sprints", map[string]any{"name": "x"}), 400)
	h.want(h.do(owner, "POST", "/projects/"+p.ID.String()+"/sprints", nil), 400)
	h.want(h.do(viewer, "POST", "/projects/"+p.ID.String()+"/sprints", map[string]any{"name": "x"}), 403)
	sp := h.want(h.do(owner, "POST", "/projects/"+p.ID.String()+"/sprints", map[string]any{"name": "S1", "start_on": "2026-03-02", "end_on": "2026-03-13"}), 201).data()
	sid := sp["id"].(string)
	h.want(h.do(owner, "GET", "/projects/"+p.ID.String()+"/sprints?status=planned&limit=1", nil), 200)
	h.want(h.do(owner, "GET", "/sprints/"+sid, nil), 200)
	h.want(h.do(owner, "PATCH", "/sprints/"+sid, map[string]any{"goal": "g"}, "If-Match", `"1"`), 200)
	h.want(h.do(owner, "PATCH", "/sprints/"+sid, map[string]any{"goal": "late"}, "If-Match", `"1"`), 409)

	plan := h.want(h.do(owner, "POST", "/projects/"+p.ID.String()+"/plan", map[string]any{"task_ids": []string{task.String()}, "sprint_id": sid}), 200)
	if len(plan.list()) != 1 {
		t.Fatalf("plan: %s", plan.Raw)
	}
	h.want(h.do(owner, "POST", "/projects/"+p.ID.String()+"/plan", map[string]any{"task_ids": []string{}}), 400)
	h.want(h.do(owner, "GET", "/projects/"+p.ID.String()+"/backlog?limit=5", nil), 200)
	h.want(h.do(owner, "POST", "/sprints/"+sid+"/start", nil), 200)
	h.want(h.do(owner, "POST", "/sprints/"+sid+"/start", nil), 409)
	h.want(h.do(owner, "GET", "/sprints/"+sid+"/report", nil), 200)
	h.want(h.do(owner, "POST", "/sprints/"+sid+"/complete", map[string]any{"move_unfinished_to": "backlog"}), 200)
	rep := h.want(h.do(owner, "GET", "/sprints/"+sid+"/report", nil), 200).data()
	if rep["live"] != false {
		t.Fatalf("report: %v", rep)
	}
	del := h.want(h.do(owner, "POST", "/projects/"+p.ID.String()+"/sprints", map[string]any{"name": "S2"}), 201).data()["id"].(string)
	d := h.want(h.do(owner, "DELETE", "/sprints/"+del, nil), 200).data()
	if d["deleted"] != true {
		t.Fatalf("delete: %v", d)
	}
}

func TestHTTPViewsAndSearch(t *testing.T) {
	h := newHTTPEnv(t)
	owner := h.user("create")
	p := h.scrum(owner, "VWS")
	todo := h.col(p, "todo")
	h.task(p, tk{title: "Searchable giraffe", col: todo, due: at("2026-03-10T09:00:00Z"), start: "2026-03-08", assignee: &owner.UserID})
	pid := p.ID.String()

	b := h.want(h.do(owner, "GET", "/projects/"+pid+"/board?per_column=1&cursor_"+todo.String()+"=&cursor_junk=1", nil), 200).data()
	if len(b["columns"].([]any)) != 4 {
		t.Fatalf("board: %v", b)
	}
	h.want(h.do(owner, "GET", "/projects/"+pid+"/board?cursor_"+todo.String()+"=%25%25", nil), 400)
	h.want(h.do(owner, "GET", "/projects/"+pid+"/calendar?from=2026-03-01&to=2026-03-31", nil), 200)
	h.want(h.do(owner, "GET", "/projects/"+pid+"/calendar?from=2026-03-01&to=2026-06-31", nil), 400)
	h.want(h.do(owner, "GET", "/projects/"+pid+"/gantt?from=2026-03-01&to=2026-03-31&limit=10", nil), 200)
	h.want(h.do(owner, "GET", "/projects/"+pid+"/gantt", nil), 200)
	h.want(h.do(owner, "GET", "/projects/"+pid+"/gantt?from=2026-03-01", nil), 400)
	h.want(h.do(owner, "GET", "/calendar?from=2026-03-01&to=2026-03-31", nil), 200)
	h.want(h.do(owner, "GET", "/calendar", nil), 400)
	h.want(h.do(owner, "GET", "/my-work?per_group=5", nil), 200)
	h.want(h.do(owner, "GET", "/my-work?group=bogus", nil), 400)
	h.want(h.do(owner, "GET", "/my-work/counts", nil), 200)
	hits := h.want(h.do(owner, "GET", "/search?q=giraffe&limit=5", nil), 200).list()
	if len(hits) != 1 {
		t.Fatalf("search: %v", hits)
	}
	h.want(h.do(owner, "GET", "/search?q=giraffe&project_id=zzz", nil), 400)
	h.want(h.do(owner, "GET", "/search?q=", nil), 400)
	h.want(h.do(owner, "GET", "/search?q=giraffe&project_id="+uuid.NewString(), nil), 404)
}

func TestHTTPSavedViewsAndTokens(t *testing.T) {
	h := newHTTPEnv(t)
	owner := h.user("create")
	p := h.scrum(owner, "HSV")
	h.task(p, tk{title: "Feed me", col: h.col(p, "todo"), due: ptr(time.Now().UTC().AddDate(0, 0, 10)), assignee: &owner.UserID})

	v := h.want(h.do(owner, "POST", "/saved-views", map[string]any{"name": "Mine", "layout": "board", "project_id": p.ID, "shared": true,
		"filters": map[string]any{"mine": true}, "sort": []map[string]any{{"field": "due"}}}), 201).data()
	vid := v["id"].(string)
	h.want(h.do(owner, "POST", "/saved-views", map[string]any{"name": "Bad", "layout": "board", "filters": map[string]any{"drop": 1}}), 400)
	if l := h.want(h.do(owner, "GET", "/saved-views?project_id="+p.ID.String(), nil), 200).list(); len(l) != 1 {
		t.Fatalf("views: %v", l)
	}
	h.want(h.do(owner, "GET", "/saved-views?project_id=nope", nil), 400)
	h.want(h.do(owner, "PATCH", "/saved-views/"+vid, map[string]any{"name": "Renamed"}, "If-Match", "1"), 200)
	h.want(h.do(owner, "PATCH", "/saved-views/"+vid, map[string]any{"name": "Again"}, "If-Match", "1"), 409)
	h.want(h.do(owner, "DELETE", "/saved-views/"+vid, nil), 200)
	h.want(h.do(owner, "DELETE", "/saved-views/"+vid, nil), 404)

	tok := h.want(h.do(owner, "POST", "/calendar-tokens", map[string]any{"label": "phone"}), 201).data()
	raw, tid := tok["token"].(string), tok["id"].(string)
	h.want(h.do(owner, "POST", "/calendar-tokens", nil), 201)
	h.want(h.do(owner, "POST", "/calendar-tokens", "not json"), 400)
	if l := h.want(h.do(owner, "GET", "/calendar-tokens", nil), 200).list(); len(l) != 2 || strings.Contains(h.do(owner, "GET", "/calendar-tokens", nil).Raw, raw) {
		t.Fatalf("tokens must list without secrets: %v", l)
	}
	// The feed needs no bearer token (no X-User) and answers text/calendar.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/feed.ics?token="+raw, nil)
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "SUMMARY:[HSV-1] Feed me") {
		t.Fatalf("feed: %d %s", w.Code, w.Body)
	}
	h.want(h.do(owner, "DELETE", "/calendar-tokens/"+tid, nil), 200)
	w = httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("revoked feed: %d", w.Code)
	}
}

// Routes that need an authenticated user answer 401 without one, except the token-authenticated feed.
func TestHTTPRequiresAnActorExceptTheFeed(t *testing.T) {
	h := newHTTPEnv(t)
	for _, path := range []string{"/my-work", "/my-work/counts", "/search?q=x", "/saved-views", "/calendar-tokens", "/calendar?from=2026-03-01&to=2026-03-02"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		w := httptest.NewRecorder()
		h.r.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/feed.ics", nil)
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("feed without token: %d", w.Code)
	}
}
