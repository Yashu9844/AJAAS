package collab

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

type httpFixture struct {
	*commentFixture
	r *gin.Engine
}

func newHTTPFixture(t *testing.T) *httpFixture {
	gin.SetMode(gin.ReleaseMode)
	h := &httpFixture{commentFixture: newCommentFixture(t), r: gin.New()}
	g := h.r.Group("", func(c *gin.Context) {
		if tid, err := uuid.Parse(c.GetHeader("X-Tenant")); err == nil {
			c.Set("tenant_id", tid)
		}
		if uid, err := uuid.Parse(c.GetHeader("X-User")); err == nil {
			c.Set("user_id", uid)
		}
	})
	h.svc.Register(g)
	return h
}

type reply struct {
	Code int
	Body map[string]any
}

func (r reply) field(path string) any {
	var cur any = r.Body
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func (h *httpFixture) do(who api.Actor, method, path string, body any) reply {
	var rd *bytes.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewReader([]byte(s))
	} else {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("X-Tenant", who.TenantID.String())
	req.Header.Set("X-User", who.UserID.String())
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	out := reply{Code: w.Code, Body: map[string]any{}}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

func (h *httpFixture) expect(r reply, code int, what string) {
	h.t.Helper()
	if r.Code != code {
		h.t.Fatalf("%s: want %d got %d %v", what, code, r.Code, r.Body)
	}
}

func TestHTTPRoutesEndToEnd(t *testing.T) {
	h := newHTTPFixture(t)
	task := "/tasks/" + h.task.String()
	// Unauthenticated callers are refused by the actor guard.
	req := httptest.NewRequest("GET", task+"/comments", nil)
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("no actor: %d", w.Code)
	}
	// Comments.
	h.expect(h.do(h.member, "POST", task+"/comments", "{bad"), 400, "malformed json")
	h.expect(h.do(h.member, "POST", "/tasks/not-a-uuid/comments", CommentInput{Body: "x"}), 400, "bad id")
	c := h.do(h.member, "POST", task+"/comments", CommentInput{Body: "hello " + mention(h.viewer.UserID)})
	h.expect(c, 201, "create comment")
	cid := c.field("data.id").(string)
	l := h.do(h.viewer, "GET", task+"/comments?limit=1", nil)
	h.expect(l, 200, "list comments")
	if l.field("meta.limit") != float64(1) || len(l.Body["data"].([]any)) != 1 {
		t.Fatalf("list: %v", l.Body)
	}
	h.expect(h.do(h.member, "PATCH", "/comments/"+cid, CommentInput{Body: "edited"}), 200, "edit comment")
	h.expect(h.do(h.owner, "PATCH", "/comments/"+cid, CommentInput{Body: "nope"}), 403, "foreign edit")
	h.expect(h.do(h.owner, "DELETE", "/comments/"+cid, nil), 200, "moderate delete")
	// Watchers.
	h.expect(h.do(h.viewer, "PUT", task+"/watch", nil), 200, "watch")
	if h.do(h.viewer, "GET", task+"/watch", nil).field("data.watching") != true {
		t.Fatal("watching")
	}
	h.expect(h.do(h.viewer, "GET", task+"/watchers", nil), 200, "watchers")
	h.expect(h.do(h.viewer, "DELETE", task+"/watch", nil), 200, "unwatch")
	h.expect(h.do(h.stranger, "GET", task+"/watchers", nil), 404, "stranger")
	// Time: manual entry, edit, delete, timer, report.
	start := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	e := h.do(h.member, "POST", task+"/time", map[string]any{"started_at": start, "minutes": 20, "note": "n"})
	h.expect(e, 201, "log time")
	eid := e.field("data.id").(string)
	h.expect(h.do(h.member, "GET", task+"/time?mine=true", nil), 200, "list time")
	h.expect(h.do(h.member, "PATCH", "/time-entries/"+eid, map[string]any{"minutes": 25}), 200, "edit time")
	h.expect(h.do(h.member, "POST", task+"/time", map[string]any{"started_at": "garbage", "minutes": 5}), 400, "bad time")
	h.expect(h.do(h.member, "POST", "/time-entries/timer/start", map[string]any{"task_id": h.task}), 201, "start timer")
	h.expect(h.do(h.member, "POST", "/time-entries/timer/start", map[string]any{"task_id": h.task}), 409, "second timer")
	if h.do(h.member, "GET", "/time-entries/timer", nil).field("data.task_id") != h.task.String() {
		t.Fatal("running timer")
	}
	h.expect(h.do(h.member, "POST", "/time-entries/timer/stop", nil), 200, "stop timer")
	none := h.do(h.member, "GET", "/time-entries/timer", nil)
	if none.Code != 200 || none.Body["data"] != nil {
		t.Fatalf("no timer: %v", none.Body)
	}
	h.expect(h.do(h.member, "POST", "/time-entries/timer/stop", nil), 404, "stop with none")
	rep := h.do(h.viewer, "GET", "/projects/"+h.pid.String()+"/time/report?group_by=user", nil)
	h.expect(rep, 200, "report")
	if rep.field("data.group_by") != "user" {
		t.Fatalf("report: %v", rep.Body)
	}
	h.expect(h.do(h.viewer, "GET", "/projects/"+h.pid.String()+"/time/report?group_by=x", nil), 400, "bad group")
	h.expect(h.do(h.member, "DELETE", "/time-entries/"+eid, nil), 200, "delete time")
	// Attachments.
	body := map[string]any{"filename": "a.txt", "size": 3, "content_type": "text/plain"}
	tk := h.do(h.member, "POST", task+"/attachments", body)
	h.expect(tk, 201, "init")
	aid := tk.field("data.attachment.id").(string)
	if tk.field("data.upload.method") != "PUT" || tk.field("data.attachment.storage_key") != nil {
		t.Fatalf("ticket: %v", tk.Body)
	}
	h.expect(h.do(h.member, "POST", "/attachments/"+aid+"/complete", nil), 409, "mismatch")
	key := StorageKey(h.tenant, h.pid, uuid.MustParse(aid))
	h.mem.Put(key, "text/plain", []byte("abc"))
	h.expect(h.do(h.member, "POST", "/attachments/"+aid+"/complete", nil), 200, "complete")
	h.expect(h.do(h.viewer, "GET", task+"/attachments", nil), 200, "list attachments")
	dl := h.do(h.viewer, "GET", "/attachments/"+aid+"/download", nil)
	h.expect(dl, 200, "download")
	if dl.field("data.url") == nil {
		t.Fatalf("download: %v", dl.Body)
	}
	h.expect(h.do(h.stranger, "GET", "/attachments/"+aid+"/download", nil), 404, "stranger download")
	h.expect(h.do(h.member, "DELETE", "/attachments/"+aid, nil), 200, "delete attachment")
}

func TestHTTPStorageUnavailable(t *testing.T) {
	h := newHTTPFixture(t)
	h.svc.p.Storage = nil
	r := h.do(h.member, "POST", "/tasks/"+h.task.String()+"/attachments", map[string]any{"filename": "a.txt", "size": 3, "content_type": "text/plain"})
	h.expect(r, 503, "init without storage")
	if r.field("error.code") != "STORAGE_UNAVAILABLE" {
		t.Fatalf("code: %v", r.Body)
	}
	h.expect(h.do(h.member, "GET", "/tasks/"+h.task.String()+"/attachments", nil), 503, "list")
	h.expect(h.do(h.member, "POST", "/attachments/"+uuid.NewString()+"/complete", nil), 503, "complete")
	h.expect(h.do(h.member, "GET", "/attachments/"+uuid.NewString()+"/download", nil), 503, "download")
	h.expect(h.do(h.member, "DELETE", "/attachments/"+uuid.NewString(), nil), 503, "delete")
}
