package task

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

type httpEnv struct {
	*env
	r *gin.Engine
}

func newHTTPEnv(t *testing.T) *httpEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := newEnv(t)
	r := gin.New()
	g := r.Group("/api/v1", func(c *gin.Context) {
		if u, err := uuid.Parse(c.GetHeader("X-User")); err == nil {
			c.Set("user_id", u)
			c.Set("tenant_id", e.tenant)
		}
	})
	e.svc.Register(g)
	return &httpEnv{env: e, r: r}
}

type reply struct {
	Status int
	Body   map[string]any
}

func (h *httpEnv) do(a api.Actor, method, path string, body any, hdr ...string) reply {
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", a.UserID.String())
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	out := reply{Status: w.Code}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

func (r reply) data(key string) any {
	d, _ := r.Body["data"].(map[string]any)
	return d[key]
}

func (r reply) code() string {
	e, _ := r.Body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (r reply) list() []any {
	l, _ := r.Body["data"].([]any)
	return l
}

func TestHTTPTaskLifecycle(t *testing.T) {
	h := newHTTPEnv(t)
	owner := h.user()
	p := h.project(owner, "WEB")
	member := h.member(owner, p, "member")
	base := "/projects/" + p.ID.String()
	if r := h.r; r != nil {
		req := httptest.NewRequest(http.MethodGet, "/api/v1"+base+"/tasks", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("no actor: %d", w.Code)
		}
	}
	created := h.do(owner, "POST", base+"/tasks", map[string]any{"title": "Web task", "priority": "high", "assignee_ids": []string{member.UserID.String()}})
	if created.Status != 201 || created.data("key") != "WEB-1" {
		t.Fatalf("create: %+v", created)
	}
	id := created.data("id").(string)
	if r := h.do(owner, "POST", base+"/tasks", "not an object"); r.Status != 400 {
		t.Fatalf("malformed body: %d", r.Status)
	}
	if r := h.do(owner, "GET", "/tasks/"+id, nil); r.Status != 200 || r.data("title") != "Web task" {
		t.Fatalf("get: %+v", r)
	}
	if r := h.do(owner, "GET", "/tasks/not-a-uuid", nil); r.Status != 400 {
		t.Fatalf("bad id: %d", r.Status)
	}
	if r := h.do(owner, "GET", "/tickets/web-1", nil); r.Status != 200 || r.data("id") != id {
		t.Fatalf("ticket: %+v", r)
	}
	if r := h.do(owner, "GET", "/tickets/WEB-9", nil); r.Status != 404 {
		t.Fatalf("missing ticket: %d", r.Status)
	}
	patched := h.do(member, "PATCH", "/tasks/"+id, map[string]any{"title": "Renamed", "due_at": "2031-01-02T03:04:05Z"}, "If-Match", `"1"`)
	if patched.Status != 200 || patched.data("version") != float64(2) {
		t.Fatalf("patch with If-Match: %+v", patched)
	}
	stale := h.do(member, "PATCH", "/tasks/"+id, map[string]any{"title": "Late"}, "If-Match", "1")
	if stale.Status != 409 || stale.code() != "VERSION_CONFLICT" || stale.Body["error"].(map[string]any)["current_version"] != float64(2) {
		t.Fatalf("stale: %+v", stale)
	}
	if r := h.do(member, "PATCH", "/tasks/"+id, map[string]any{"title": "x"}); r.Status != 400 {
		t.Fatalf("version required: %d", r.Status)
	}
	done := h.col(p, "done").String()
	if r := h.do(member, "POST", "/tasks/"+id+"/move", map[string]any{"column_id": done}); r.Status != 200 || r.data("column_category") != "done" {
		t.Fatalf("move: %+v", r)
	}
	if r := h.do(member, "POST", "/tasks/"+id+"/reorder", map[string]any{"column_id": done}); r.Status != 400 {
		t.Fatalf("reorder needs a neighbour: %d", r.Status)
	}
	second := h.do(owner, "POST", base+"/tasks", map[string]any{"title": "Second", "column_id": done}).data("id").(string)
	if r := h.do(member, "POST", "/tasks/"+id+"/reorder", map[string]any{"after_id": second}); r.Status != 200 {
		t.Fatalf("reorder: %+v", r)
	}
	if r := h.do(member, "POST", "/tasks/"+id+"/duplicate", nil); r.Status != 201 || r.data("title") != "Renamed (copy)" {
		t.Fatalf("duplicate without body: %+v", r)
	}
	if r := h.do(member, "POST", "/tasks/"+id+"/duplicate", map[string]any{"copy_assignees": true}); r.Status != 201 || len(r.data("assignee_ids").([]any)) != 1 {
		t.Fatalf("duplicate with body: %+v", r)
	}
	if r := h.do(member, "POST", "/tasks/"+id+"/archive", nil); r.Status != 200 || r.data("archived_at") == nil {
		t.Fatalf("archive: %+v", r)
	}
	if r := h.do(member, "POST", "/tasks/"+id+"/restore", nil); r.Status != 200 || r.data("archived_at") != nil {
		t.Fatalf("restore: %+v", r)
	}
	if r := h.do(member, "DELETE", "/tasks/"+id, nil); r.Status != 403 {
		t.Fatalf("delete needs task.delete: %d", r.Status)
	}
	if r := h.do(owner, "DELETE", "/tasks/"+id, nil); r.Status != 200 || r.data("deleted") != true {
		t.Fatalf("delete: %+v", r)
	}
}

func TestHTTPListLinksAndActivity(t *testing.T) {
	h := newHTTPEnv(t)
	owner := h.user()
	p, q := h.project(owner, "WLK"), h.project(owner, "WLQ")
	member := h.member(owner, p, "member")
	bug := h.label(owner, p, "bug")
	base := "/projects/" + p.ID.String()
	a := h.do(owner, "POST", base+"/tasks", map[string]any{"title": "alpha"}).data("id").(string)
	b := h.do(owner, "POST", base+"/tasks", map[string]any{"title": "beta", "parent_task_id": a}).data("id").(string)
	list := h.do(owner, "GET", base+"/tasks?limit=1&sort=number&q=a&overdue=false&blocked=false&top_level=true&assignee=me&category=todo&priority=none&due_from=2020-01-01&due_to=2040-01-01T00:00:00Z", nil)
	if list.Status != 200 || len(list.list()) != 0 {
		t.Fatalf("filtered list: %+v", list)
	}
	list = h.do(owner, "GET", base+"/tasks?limit=1&sort=number", nil)
	meta := list.Body["meta"].(map[string]any)
	if len(list.list()) != 1 || meta["next_cursor"] == nil || meta["limit"] != float64(1) {
		t.Fatalf("paged list: %+v", list)
	}
	next := h.do(owner, "GET", base+"/tasks?limit=1&sort=number&cursor="+meta["next_cursor"].(string), nil)
	if next.Status != 200 || len(next.list()) != 1 {
		t.Fatalf("next page: %+v", next)
	}
	for _, bad := range []string{"column_id=x", "assignee=x", "overdue=maybe", "blocked=1", "due_from=yesterday", "due_to=x", "sort=zzz", "label_id=x", "milestone_id=x", "sprint_id=x", "parent_id=x"} {
		if r := h.do(owner, "GET", base+"/tasks?"+bad, nil); r.Status != 400 {
			t.Errorf("%s: %d", bad, r.Status)
		}
	}
	if r := h.do(owner, "GET", base+"/tasks?assignee="+member.UserID.String()+"&label_id="+bug.ID.String(), nil); r.Status != 200 {
		t.Fatalf("id filters: %d", r.Status)
	}
	if r := h.do(owner, "GET", "/tasks/"+a+"/subtasks", nil); r.Status != 200 || len(r.list()) != 1 {
		t.Fatalf("subtasks: %+v", r)
	}
	if r := h.do(owner, "POST", "/tasks/"+a+"/assignees", map[string]any{"user_ids": []string{member.UserID.String()}}); r.Status != 200 {
		t.Fatalf("assign: %+v", r)
	}
	if r := h.do(owner, "DELETE", "/tasks/"+a+"/assignees/"+member.UserID.String(), nil); r.Status != 200 {
		t.Fatalf("unassign: %+v", r)
	}
	if r := h.do(owner, "POST", "/tasks/"+a+"/labels", map[string]any{"label_ids": []string{bug.ID.String()}}); r.Status != 200 {
		t.Fatalf("label: %+v", r)
	}
	if r := h.do(owner, "DELETE", "/tasks/"+a+"/labels/"+bug.ID.String(), nil); r.Status != 200 {
		t.Fatalf("unlabel: %+v", r)
	}
	rel := h.do(owner, "POST", "/tasks/"+a+"/relations", map[string]any{"target_task_id": b, "type": "blocks"})
	if rel.Status != 201 {
		t.Fatalf("relation: %+v", rel)
	}
	if r := h.do(owner, "POST", "/tasks/"+b+"/relations", map[string]any{"target_task_id": a, "type": "blocks"}); r.Status != 409 || r.code() != "DEPENDENCY_CYCLE" {
		t.Fatalf("cycle: %+v", r)
	}
	if r := h.do(owner, "GET", "/tasks/"+b+"/relations", nil); r.Status != 200 || len(r.list()) != 1 {
		t.Fatalf("relations: %+v", r)
	}
	if r := h.do(owner, "DELETE", "/tasks/"+a+"/relations/"+rel.data("id").(string), nil); r.Status != 200 {
		t.Fatalf("remove relation: %+v", r)
	}
	if r := h.do(owner, "GET", "/tasks/"+a+"/activity?order=asc&limit=2", nil); r.Status != 200 || len(r.list()) != 2 {
		t.Fatalf("task activity: %+v", r)
	}
	if r := h.do(owner, "GET", base+"/activity", nil); r.Status != 200 || len(r.list()) == 0 {
		t.Fatalf("project activity: %+v", r)
	}
	bulk := h.do(owner, "POST", base+"/tasks/bulk", map[string]any{"task_ids": []string{a, b}, "priority": "low"})
	if bulk.Status != 200 || bulk.data("succeeded") != float64(2) {
		t.Fatalf("bulk: %+v", bulk)
	}
	mp := h.do(owner, "POST", "/tasks/"+a+"/move-project", map[string]any{"project_id": q.ID.String()})
	if mp.Status != 409 || mp.code() != "TASK_HAS_HIERARCHY" {
		t.Fatalf("move-project: %+v", mp)
	}
	if r := h.do(owner, "POST", "/tasks/"+b+"/move-project", map[string]any{"project_id": "nope"}); r.Status != 400 {
		t.Fatalf("bad project id: %d", r.Status)
	}
}
