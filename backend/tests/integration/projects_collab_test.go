package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// collabFixture is a private project with an owner, a member, a stranger (no access) and one task, created through the
// production router; the task row is inserted by SQL because the task context is exercised by its own tests.
type collabFixture struct {
	tc                          *tenantCtx
	owner, member, stranger     string // tokens
	memberID, strangerID, ownID string
	pid, task                   string
}

func newCollabFixture(t *testing.T) *collabFixture {
	tc := newTenant(t)
	f := &collabFixture{tc: tc}
	f.owner, _, f.ownID = tc.limitedUser(t, [2]string{"projects", "create"})
	f.member, _, f.memberID = tc.limitedUser(t)
	f.stranger, _, f.strangerID = tc.limitedUser(t)
	key := "C" + strings.ToUpper(uniq())
	if len(key) > 10 {
		key = key[:10]
	}
	p := tc.asTok(t, f.owner, "POST", "/projects", body(map[string]any{"key": key, "name": "Collab"}))
	expect(t, p, 201, "create project")
	f.pid = p.str("data.id")
	expect(t, tc.asTok(t, f.owner, "POST", "/projects/"+f.pid+"/members", body(map[string]any{"user_id": f.memberID, "role": "member"})), 201, "add member")
	cols := tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/columns")
	col := cols.list("data")[0].(map[string]any)["id"].(string)
	f.task = uuid.NewString()
	err := testDB.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank, estimate_minutes)
		VALUES (?, ?, ?, 1, 'Collab task', ?, ?, 'n', 120)`, f.task, tc.id, f.pid, col, f.ownID).Error
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestCollabComments covers CM-01..CM-03 and PG4, PG11, PG12 through the router.
func TestCollabComments(t *testing.T) {
	f := newCollabFixture(t)
	tc, task := f.tc, "/tasks/"+f.task
	secret := "INTEGRATION-SECRET-" + uniq()
	text := fmt.Sprintf("%s @[Member](user:%s) @[Stranger](user:%s)", secret, f.memberID, f.strangerID)

	expectErr(t, call(t, "GET", task+"/comments", host(tc.slug)), 401, "UNAUTHORIZED", "needs a token")
	expectErr(t, tc.asTok(t, f.stranger, "GET", task+"/comments"), 404, "NOT_FOUND", "PG4: no access means 404")
	expectErr(t, tc.asTok(t, f.stranger, "POST", task+"/comments", body(map[string]any{"body": "x"})), 404, "NOT_FOUND", "stranger cannot comment")
	other := newTenant(t)
	expectErr(t, call(t, "GET", task+"/comments", host(other.slug), token(other.token)), 404, "NOT_FOUND", "foreign tenant")

	c := tc.asTok(t, f.owner, "POST", task+"/comments", body(map[string]any{"body": text}))
	expect(t, c, 201, "create comment")
	cid := c.str("data.id")
	ms := c.list("data.mentions")
	if len(ms) != 1 || ms[0] != f.memberID {
		t.Fatalf("PG11: only the member who can read stays: %s", c.Raw)
	}
	expectErr(t, tc.asTok(t, f.owner, "POST", task+"/comments", body(map[string]any{"body": ""})), 400, "VALIDATION_ERROR", "empty body")
	expectErr(t, tc.asTok(t, f.owner, "POST", task+"/comments", rawBody("{")), 400, "VALIDATION_ERROR", "malformed json")
	expectErr(t, tc.asTok(t, f.member, "PATCH", "/comments/"+cid, body(map[string]any{"body": "mine now"})), 403, "FORBIDDEN", "only the author edits")
	up := tc.asTok(t, f.owner, "PATCH", "/comments/"+cid, body(map[string]any{"body": "edited " + secret}))
	expect(t, up, 200, "edit")
	if up.get("data.edited_at") == nil {
		t.Fatalf("edited_at: %s", up.Raw)
	}
	l := tc.asTok(t, f.member, "GET", task+"/comments", query("limit", "10"))
	expect(t, l, 200, "member lists")
	if len(l.list("data")) != 1 || l.get("meta.limit") != float64(10) {
		t.Fatalf("list: %s", l.Raw)
	}
	// Watchers: the commenter and the mentioned member watch automatically; users can opt in and out.
	w := tc.asTok(t, f.member, "GET", task+"/watchers")
	expect(t, w, 200, "watchers")
	if !w.has("data", "user_id", f.ownID) || !w.has("data", "user_id", f.memberID) || w.has("data", "user_id", f.strangerID) {
		t.Fatalf("auto watchers: %s", w.Raw)
	}
	expect(t, tc.asTok(t, f.member, "DELETE", task+"/watch"), 200, "unwatch")
	if tc.asTok(t, f.member, "GET", task+"/watch").get("data.watching") != false {
		t.Fatal("unwatched")
	}
	expect(t, tc.asTok(t, f.member, "PUT", task+"/watch"), 200, "watch")
	if tc.asTok(t, f.member, "GET", task+"/watch").str("data.reason") != "explicit" {
		t.Fatal("explicit watch")
	}
	// CM-01: a delete leaves a tombstone; PG12: events and activity never hold the text.
	expect(t, tc.asTok(t, f.owner, "DELETE", "/comments/"+cid), 200, "delete")
	tomb := tc.asTok(t, f.owner, "GET", task+"/comments")
	if tomb.get("data.0.deleted") != true || tomb.get("data.0.body") != "" {
		t.Fatalf("tombstone: %s", tomb.Raw)
	}
	var leaks int64
	testDB.Raw(`SELECT (SELECT count(*) FROM projects_events_outbox WHERE payload::text LIKE ?) + (SELECT count(*) FROM activity WHERE data::text LIKE ?)`,
		"%"+secret+"%", "%"+secret+"%").Scan(&leaks)
	if leaks != 0 {
		t.Fatal("comment text leaked into events or activity")
	}
	var commented, mentioned int64
	testDB.Raw(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.commented' AND payload::text LIKE ?`, "%"+cid+"%").Scan(&commented)
	testDB.Raw(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.mentioned' AND payload::text LIKE ? AND payload::text NOT LIKE ?`,
		"%"+f.memberID+"%", "%"+f.strangerID+"%").Scan(&mentioned)
	if commented != 1 || mentioned != 1 {
		t.Fatalf("events: commented=%d mentioned=%d", commented, mentioned)
	}
}

// TestCollabTime covers TM-01..TM-03 and PG6 through the router.
func TestCollabTime(t *testing.T) {
	f := newCollabFixture(t)
	tc, task := f.tc, "/tasks/"+f.task
	start := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	expectErr(t, tc.asTok(t, f.stranger, "POST", task+"/time", body(map[string]any{"started_at": start, "minutes": 30})), 404, "NOT_FOUND", "no access")
	expectErr(t, tc.asTok(t, f.member, "POST", task+"/time", body(map[string]any{"started_at": start, "minutes": 0})), 400, "VALIDATION_ERROR", "minutes")
	e := tc.asTok(t, f.member, "POST", task+"/time", body(map[string]any{"started_at": start, "minutes": 45, "note": "pairing"}))
	expect(t, e, 201, "log time")
	eid := e.str("data.id")
	expect(t, tc.asTok(t, f.member, "PATCH", "/time-entries/"+eid, body(map[string]any{"minutes": 50})), 200, "edit")
	expectErr(t, tc.asTok(t, f.stranger, "DELETE", "/time-entries/"+eid), 404, "NOT_FOUND", "stranger")

	// PG6: one running timer per user.
	expect(t, tc.asTok(t, f.member, "POST", "/time-entries/timer/start", body(map[string]any{"task_id": f.task})), 201, "start")
	expectErr(t, tc.asTok(t, f.member, "POST", "/time-entries/timer/start", body(map[string]any{"task_id": f.task})), 409, "TIMER_RUNNING", "second timer")
	if tc.asTok(t, f.member, "GET", "/time-entries/timer").str("data.task_id") != f.task {
		t.Fatal("running timer")
	}
	expect(t, tc.asTok(t, f.member, "POST", "/time-entries/timer/stop"), 200, "stop")
	expectErr(t, tc.asTok(t, f.member, "POST", "/time-entries/timer/stop"), 404, "NOT_FOUND", "nothing to stop")
	if r := tc.asTok(t, f.member, "GET", "/time-entries/timer"); r.Status != 200 || r.get("data") != nil {
		t.Fatalf("no timer: %s", r.Raw)
	}

	// TM-02: totals per task (with the estimate) and per user.
	rep := tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/time/report")
	expect(t, rep, 200, "report")
	if rep.get("data.estimate_minutes") != float64(120) || rep.get("data.total_minutes").(float64) < 50 || len(rep.list("data.rows")) != 1 {
		t.Fatalf("report: %s", rep.Raw)
	}
	byUser := tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/time/report", query("group_by", "user"))
	if !byUser.has("data.rows", "key", f.memberID) {
		t.Fatalf("per user: %s", byUser.Raw)
	}
	expectErr(t, tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/time/report", query("group_by", "x")), 400, "VALIDATION_ERROR", "group_by")
	expectErr(t, tc.asTok(t, f.stranger, "GET", "/projects/"+f.pid+"/time/report"), 404, "NOT_FOUND", "no access")

	// A project can turn time tracking off: logging answers 409, reports still work.
	expect(t, tc.asTok(t, f.owner, "PATCH", "/projects/"+f.pid, body(map[string]any{"time_tracking_enabled": false})), 200, "disable tracking")
	expectErr(t, tc.asTok(t, f.member, "POST", task+"/time", body(map[string]any{"started_at": start, "minutes": 5})), 409, "TIME_TRACKING_DISABLED", "disabled")
	expectErr(t, tc.asTok(t, f.member, "POST", "/time-entries/timer/start", body(map[string]any{"task_id": f.task})), 409, "TIME_TRACKING_DISABLED", "disabled timer")
	expect(t, tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/time/report"), 200, "report still works")
}

// TestCollabAttachmentsWithoutStorage: the integration app has no object storage, so every attachment endpoint answers
// 503 STORAGE_UNAVAILABLE; the real-S3 round trip lives in the collab package test.
func TestCollabAttachmentsWithoutStorage(t *testing.T) {
	f := newCollabFixture(t)
	tc, task := f.tc, "/tasks/"+f.task
	id := uuid.NewString()
	init := body(map[string]any{"filename": "a.txt", "size": 3, "content_type": "text/plain"})
	expectErr(t, tc.asTok(t, f.member, "POST", task+"/attachments", init), 503, "STORAGE_UNAVAILABLE", "init")
	expectErr(t, tc.asTok(t, f.member, "GET", task+"/attachments"), 503, "STORAGE_UNAVAILABLE", "list")
	expectErr(t, tc.asTok(t, f.member, "POST", "/attachments/"+id+"/complete"), 503, "STORAGE_UNAVAILABLE", "complete")
	expectErr(t, tc.asTok(t, f.member, "GET", "/attachments/"+id+"/download"), 503, "STORAGE_UNAVAILABLE", "download")
	expectErr(t, tc.asTok(t, f.member, "DELETE", "/attachments/"+id), 503, "STORAGE_UNAVAILABLE", "delete")
	expectErr(t, call(t, "POST", task+"/attachments", host(tc.slug), init), 401, "UNAUTHORIZED", "needs a token")
	expectErr(t, tc.asTok(t, f.member, "GET", "/attachments/not-a-uuid/download"), 400, "VALIDATION_ERROR", "bad id")
}
