package integration

import (
	"fmt"
	"sync"
	"testing"
)

// taskFixture is a project with an owner, a member and a viewer, all limited users of one tenant.
type taskFixture struct {
	tc                          *tenantCtx
	owner, member, viewer       string
	ownerID, memberID, viewerID string
	pid, todo, done, prog       string
}

func newTaskFixture(t *testing.T, key string) *taskFixture {
	t.Helper()
	tc := newTenant(t)
	f := &taskFixture{tc: tc}
	f.owner, _, f.ownerID = tc.limitedUser(t, [2]string{"projects", "create"})
	f.member, _, f.memberID = tc.limitedUser(t)
	f.viewer, _, f.viewerID = tc.limitedUser(t)
	p := tc.asTok(t, f.owner, "POST", "/projects", body(map[string]any{"key": key, "name": "Tasks " + key}))
	expect(t, p, 201, "create project")
	f.pid = p.str("data.id")
	expect(t, tc.asTok(t, f.owner, "POST", "/projects/"+f.pid+"/members", body(map[string]any{"user_id": f.memberID, "role": "member"})), 201, "member")
	expect(t, tc.asTok(t, f.owner, "POST", "/projects/"+f.pid+"/members", body(map[string]any{"user_id": f.viewerID, "role": "viewer"})), 201, "viewer")
	for _, c := range tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/columns").list("data") {
		m := c.(map[string]any)
		switch m["key"] {
		case "todo":
			f.todo = m["id"].(string)
		case "done":
			f.done = m["id"].(string)
		case "in-progress":
			f.prog = m["id"].(string)
		}
	}
	return f
}

func (f *taskFixture) as(t *testing.T, tok, method, path string, opts ...opt) resp {
	t.Helper()
	return f.tc.asTok(t, tok, method, path, opts...)
}

func (f *taskFixture) task(t *testing.T, title string, extra map[string]any) string {
	t.Helper()
	b := map[string]any{"title": title}
	for k, v := range extra {
		b[k] = v
	}
	r := f.as(t, f.member, "POST", "/projects/"+f.pid+"/tasks", body(b))
	expect(t, r, 201, "create task "+title)
	return r.str("data.id")
}

// TestProjectTasksCore covers PG1 (idempotent create), PG4 (visibility), TK-03 versions, moves, tickets and archive.
func TestProjectTasksCore(t *testing.T) {
	f := newTaskFixture(t, "TSK")
	base := "/projects/" + f.pid + "/tasks"
	key := "idem-" + uniq()
	first := f.as(t, f.member, "POST", base, header("Idempotency-Key", key), body(map[string]any{"title": "Ship it", "priority": "high"}))
	expect(t, first, 201, "create")
	replay := f.as(t, f.member, "POST", base, header("Idempotency-Key", key), body(map[string]any{"title": "Ship it", "priority": "high"}))
	expect(t, replay, 201, "replay")
	if replay.str("data.id") != first.str("data.id") || f.tc.countTasks(t, f.pid) != 1 {
		t.Fatalf("PG1: replay must return the original task: %s", replay.Raw)
	}
	id := first.str("data.id")
	if first.str("data.key") != "TSK-1" || first.str("data.column_category") != "todo" {
		t.Fatalf("created: %s", first.Raw)
	}
	expectErr(t, f.as(t, f.viewer, "POST", base, body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "viewer cannot create")
	expectErr(t, f.as(t, f.member, "POST", base, body(map[string]any{"title": ""})), 400, "VALIDATION_ERROR", "empty title")
	stranger, _, _ := f.tc.limitedUser(t)
	expectErr(t, f.as(t, stranger, "GET", "/tasks/"+id), 404, "NOT_FOUND", "PG4 stranger")
	expectErr(t, f.as(t, stranger, "GET", "/tickets/TSK-1"), 404, "NOT_FOUND", "PG4 stranger ticket")
	other := newTenant(t)
	expectErr(t, call(t, "GET", "/tasks/"+id, host(other.slug), token(other.token)), 404, "NOT_FOUND", "foreign tenant")
	expect(t, f.as(t, f.viewer, "GET", "/tasks/"+id), 200, "viewer reads")
	expect(t, f.as(t, f.viewer, "GET", "/tickets/tsk-1"), 200, "ticket lookup")

	ok := f.as(t, f.member, "PATCH", "/tasks/"+id, header("If-Match", `"1"`), body(map[string]any{"title": "Shipped", "due_at": "2031-05-06T07:08:09Z"}))
	expect(t, ok, 200, "patch")
	stale := f.as(t, f.member, "PATCH", "/tasks/"+id, header("If-Match", "1"), body(map[string]any{"title": "Late"}))
	expectErr(t, stale, 409, "VERSION_CONFLICT", "stale")
	if stale.get("error.current_version") != float64(2) {
		t.Fatalf("current_version: %s", stale.Raw)
	}
	expectErr(t, f.as(t, f.member, "PATCH", "/tasks/"+id, body(map[string]any{"title": "x"})), 400, "VALIDATION_ERROR", "version required")

	mv := f.as(t, f.member, "POST", "/tasks/"+id+"/move", body(map[string]any{"column_id": f.done}))
	expect(t, mv, 200, "move to done")
	if mv.str("data.completed_at") == "" || mv.str("data.column_category") != "done" {
		t.Fatalf("done: %s", mv.Raw)
	}
	second := f.task(t, "Second", map[string]any{"column_id": f.done})
	expect(t, f.as(t, f.member, "POST", "/tasks/"+id+"/reorder", body(map[string]any{"after_id": second})), 200, "reorder")
	expectErr(t, f.as(t, f.member, "POST", "/tasks/"+id+"/reorder", body(map[string]any{})), 400, "VALIDATION_ERROR", "reorder needs a neighbour")

	expect(t, f.as(t, f.member, "POST", "/tasks/"+id+"/archive"), 200, "archive")
	if l := f.as(t, f.member, "GET", base); len(l.list("data")) != 1 {
		t.Fatalf("archived leaves the list: %s", l.Raw)
	}
	if l := f.as(t, f.member, "GET", base, query("archived", "true")); len(l.list("data")) != 1 {
		t.Fatalf("archived list: %s", l.Raw)
	}
	expect(t, f.as(t, f.member, "POST", "/tasks/"+id+"/restore"), 200, "restore")
	expectErr(t, f.as(t, f.member, "DELETE", "/tasks/"+id), 403, "FORBIDDEN", "member cannot delete")
	expect(t, f.as(t, f.owner, "DELETE", "/tasks/"+id), 200, "manager deletes")
	expectErr(t, f.as(t, f.owner, "GET", "/tasks/"+id), 404, "NOT_FOUND", "deleted")

	expect(t, f.as(t, f.owner, "POST", "/projects/"+f.pid+"/archive"), 200, "archive project")
	expectErr(t, f.as(t, f.member, "POST", base, body(map[string]any{"title": "late"})), 409, "PROJECT_ARCHIVED", "frozen")
}

func (tc *tenantCtx) countTasks(t *testing.T, pid string) int64 {
	t.Helper()
	var n int64
	testDB.Raw(`SELECT count(*) FROM projects.tasks WHERE project_id = ?`, pid).Scan(&n)
	return n
}

// TestTasksListingAndSubtasks covers TK-13 paging, filters, subtasks and PG5 over HTTP.
func TestTasksListingAndSubtasks(t *testing.T) {
	f := newTaskFixture(t, "LST")
	base := "/projects/" + f.pid + "/tasks"
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f.as(t, f.member, "POST", base, body(map[string]any{"title": fmt.Sprintf("t%02d", i), "priority": []string{"low", "high"}[i%2]}))
		}(i)
	}
	wg.Wait()
	if n := f.tc.countTasks(t, f.pid); n != 12 {
		t.Fatalf("created %d", n)
	}
	var nums int64
	testDB.Raw(`SELECT count(DISTINCT number) FROM projects.tasks WHERE project_id = ? AND number BETWEEN 1 AND 12`, f.pid).Scan(&nums)
	if nums != 12 {
		t.Fatalf("PG5: numbers 1..12 must each exist once, got %d", nums)
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 10; page++ {
		opts := []opt{query("limit", "5", "sort", "number")}
		if cursor != "" {
			opts = append(opts, query("cursor", cursor))
		}
		r := f.as(t, f.viewer, "GET", base, opts...)
		expect(t, r, 200, "page")
		for _, it := range r.list("data") {
			id := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate across pages: %s", id)
			}
			seen[id] = true
		}
		cursor = r.str("meta.next_cursor")
		if cursor == "" {
			break
		}
	}
	if len(seen) != 12 {
		t.Fatalf("paged %d of 12", len(seen))
	}
	if r := f.as(t, f.viewer, "GET", base, query("priority", "high")); len(r.list("data")) != 6 {
		t.Fatalf("priority filter: %s", r.Raw)
	}
	expectErr(t, f.as(t, f.viewer, "GET", base, query("sort", "nope")), 400, "VALIDATION_ERROR", "bad sort")

	parent := f.task(t, "Parent", nil)
	kid := f.task(t, "Kid", map[string]any{"parent_task_id": parent})
	subs := f.as(t, f.member, "GET", "/tasks/"+parent+"/subtasks")
	if len(subs.list("data")) != 1 || subs.str("data.0.id") != kid {
		t.Fatalf("subtasks: %s", subs.Raw)
	}
	expectErr(t, f.as(t, f.member, "POST", "/tasks/"+parent+"/move", body(map[string]any{"column_id": f.done})), 409, "OPEN_SUBTASKS", "open subtasks")
	expect(t, f.as(t, f.member, "POST", "/tasks/"+kid+"/move", body(map[string]any{"column_id": f.done})), 200, "finish the kid")
	done := f.as(t, f.member, "POST", "/tasks/"+parent+"/move", body(map[string]any{"column_id": f.done}))
	expect(t, done, 200, "parent done")
	if done.get("data.progress") != float64(100) {
		t.Fatalf("progress: %s", done.Raw)
	}
}

// TestTaskRelationsAssigneesBulkAndMove covers PG3, assignees, bulk partial failure, move-project and activity.
func TestTaskRelationsAssigneesBulkAndMove(t *testing.T) {
	f := newTaskFixture(t, "REL")
	a, b := f.task(t, "a", nil), f.task(t, "b", nil)
	expect(t, f.as(t, f.member, "POST", "/tasks/"+a+"/relations", body(map[string]any{"target_task_id": b, "type": "blocks"})), 201, "blocks")
	expectErr(t, f.as(t, f.member, "POST", "/tasks/"+b+"/relations", body(map[string]any{"target_task_id": a, "type": "blocks"})), 409, "DEPENDENCY_CYCLE", "PG3")
	if r := f.as(t, f.viewer, "GET", "/tasks/"+b+"/relations"); len(r.list("data")) != 1 || r.str("data.0.direction") != "incoming" {
		t.Fatalf("relations: %s", r.Raw)
	}
	expectErr(t, f.as(t, f.member, "POST", "/tasks/"+b+"/move", body(map[string]any{"column_id": f.done})), 409, "TASK_BLOCKED", "blocked")
	expectErr(t, f.as(t, f.member, "POST", "/tasks/"+b+"/move", body(map[string]any{"column_id": f.done, "force": true})), 403, "FORBIDDEN", "member cannot force")
	expect(t, f.as(t, f.owner, "POST", "/tasks/"+b+"/move", body(map[string]any{"column_id": f.done, "force": true})), 200, "manager forces")

	as := f.as(t, f.member, "POST", "/tasks/"+a+"/assignees", body(map[string]any{"user_ids": []string{f.memberID}}))
	expect(t, as, 200, "assign")
	if as.str("data.assignees.0.user_id") != f.memberID || as.get("data.assignees.0.active") != true {
		t.Fatalf("assignees expose the active flag: %s", as.Raw)
	}
	expectErr(t, f.as(t, f.member, "POST", "/tasks/"+a+"/assignees", body(map[string]any{"user_ids": []string{zeroUUID}})), 400, "VALIDATION_ERROR", "non-member")
	expect(t, f.as(t, f.member, "DELETE", "/tasks/"+a+"/assignees/"+f.memberID), 200, "unassign")

	bulk := f.as(t, f.member, "POST", "/projects/"+f.pid+"/tasks/bulk", body(map[string]any{"task_ids": []string{a, zeroUUID}, "priority": "urgent"}))
	expect(t, bulk, 200, "bulk")
	if bulk.get("data.succeeded") != float64(1) || bulk.get("data.failed") != float64(1) || bulk.str("data.results.1.error.code") != "NOT_FOUND" {
		t.Fatalf("bulk partial: %s", bulk.Raw)
	}

	q := f.as(t, f.owner, "POST", "/projects", body(map[string]any{"key": "DST", "name": "Destination"}))
	expect(t, q, 201, "second project")
	mp := f.as(t, f.owner, "POST", "/tasks/"+a+"/move-project", body(map[string]any{"project_id": q.str("data.id")}))
	expect(t, mp, 200, "move project")
	if mp.str("data.key") != "DST-1" {
		t.Fatalf("new number: %s", mp.Raw)
	}
	expect(t, f.as(t, f.owner, "GET", "/tickets/REL-1"), 200, "old ticket id still resolves")
	expectErr(t, f.as(t, f.member, "GET", "/tasks/"+a), 404, "NOT_FOUND", "member of the old project cannot read it")

	act := f.as(t, f.owner, "GET", "/tasks/"+a+"/activity", query("order", "asc"))
	expect(t, act, 200, "activity")
	if len(act.list("data")) < 4 || act.str("data.0.type") != "created" {
		t.Fatalf("activity: %s", act.Raw)
	}
	pa := f.as(t, f.owner, "GET", "/projects/"+f.pid+"/activity", query("limit", "2"))
	expect(t, pa, 200, "project activity")
	if len(pa.list("data")) != 2 || pa.str("meta.next_cursor") == "" {
		t.Fatalf("project activity paging: %s", pa.Raw)
	}
}
