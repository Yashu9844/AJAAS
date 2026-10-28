package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// planFixture is a scrum project with an owner, a viewer and a stranger; tasks are inserted by SQL because the task
// context has its own tests.
type planFixture struct {
	tc                      *tenantCtx
	owner, viewer, stranger string
	ownID, viewerID         string
	pid, todo, done         string
	n                       int
}

func newPlanFixture(t *testing.T) *planFixture {
	tc := newTenant(t)
	f := &planFixture{tc: tc}
	f.owner, _, f.ownID = tc.limitedUser(t, [2]string{"projects", "create"})
	f.viewer, _, f.viewerID = tc.limitedUser(t)
	f.stranger, _, _ = tc.limitedUser(t)
	key := "P" + strings.ToUpper(uniq())
	if len(key) > 10 {
		key = key[:10]
	}
	p := tc.asTok(t, f.owner, "POST", "/projects", body(map[string]any{"key": key, "name": "Planning", "mode": "scrum"}))
	expect(t, p, 201, "create scrum project")
	f.pid = p.str("data.id")
	expect(t, tc.asTok(t, f.owner, "POST", "/projects/"+f.pid+"/members", body(map[string]any{"user_id": f.viewerID, "role": "viewer"})), 201, "add viewer")
	for _, c := range tc.asTok(t, f.owner, "GET", "/projects/"+f.pid+"/columns").list("data") {
		m := c.(map[string]any)
		switch m["category"] {
		case "todo":
			f.todo = m["id"].(string)
		case "done":
			f.done = m["id"].(string)
		}
	}
	return f
}

// task inserts a task row (optionally assigned to a user) and returns its id.
func (f *planFixture) task(t *testing.T, title, col, assignee string, due *time.Time) string {
	t.Helper()
	f.n++
	id := uuid.NewString()
	err := testDB.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank, due_at, estimate_minutes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 30)`, id, f.tc.id, f.pid, f.n, title, col, f.ownID, "r"+strings.Repeat("0", 5)+string(rune('a'+f.n)), due).Error
	if err != nil {
		t.Fatal(err)
	}
	if assignee != "" {
		if err := testDB.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES (?, ?, ?, ?)`, id, assignee, f.tc.id, f.ownID).Error; err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// TestPlanningSprintsAndPlan covers SP-01..SP-03 and PG6 through the production router.
func TestPlanningSprintsAndPlan(t *testing.T) {
	f := newPlanFixture(t)
	tc, pid := f.tc, f.pid
	expectErr(t, call(t, "GET", "/projects/"+pid+"/sprints", host(tc.slug)), 401, "UNAUTHORIZED", "needs a token")
	expectErr(t, tc.asTok(t, f.stranger, "GET", "/projects/"+pid+"/sprints"), 404, "NOT_FOUND", "PG4: private project")
	expectErr(t, tc.asTok(t, f.viewer, "POST", "/projects/"+pid+"/sprints", body(map[string]any{"name": "S"})), 403, "FORBIDDEN", "viewer cannot manage sprints")

	s1 := tc.asTok(t, f.owner, "POST", "/projects/"+pid+"/sprints", body(map[string]any{"name": "Sprint 1", "start_on": "2026-03-02", "end_on": "2026-03-13"}))
	expect(t, s1, 201, "create sprint")
	s2 := tc.asTok(t, f.owner, "POST", "/projects/"+pid+"/sprints", body(map[string]any{"name": "Sprint 2", "start_on": "2026-03-16", "end_on": "2026-03-27"}))
	sid1, sid2 := s1.str("data.id"), s2.str("data.id")
	a := f.task(t, "Alpha", f.todo, "", nil)
	b := f.task(t, "Beta", f.done, "", nil)

	plan := tc.asTok(t, f.owner, "POST", "/projects/"+pid+"/plan", body(map[string]any{"task_ids": []string{a, b}, "sprint_id": sid1, "versions": map[string]int{a: 1}}))
	expect(t, plan, 200, "plan")
	if plan.str("data.0.status") != "moved" || plan.str("data.1.status") != "moved" {
		t.Fatalf("plan results: %s", plan.Raw)
	}
	stale := tc.asTok(t, f.owner, "POST", "/projects/"+pid+"/plan", body(map[string]any{"task_ids": []string{a}, "sprint_id": sid2, "versions": map[string]int{a: 1}}))
	if stale.str("data.0.status") != "conflict" {
		t.Fatalf("stale version must conflict per task: %s", stale.Raw)
	}
	expectErr(t, tc.asTok(t, f.viewer, "POST", "/projects/"+pid+"/plan", body(map[string]any{"task_ids": []string{a}})), 403, "FORBIDDEN", "viewer cannot plan")

	expect(t, tc.asTok(t, f.owner, "POST", "/sprints/"+sid1+"/start"), 200, "start")
	expectErr(t, tc.asTok(t, f.owner, "POST", "/sprints/"+sid2+"/start"), 409, "SPRINT_ACTIVE", "PG6: second active sprint")
	live := tc.asTok(t, f.owner, "GET", "/sprints/"+sid1+"/report")
	if live.get("data.live") != true || live.get("data.report.committed_count") != float64(2) {
		t.Fatalf("live report: %s", live.Raw)
	}
	expectErr(t, tc.asTok(t, f.owner, "DELETE", "/sprints/"+sid1), 409, "SPRINT_STATE", "active sprints cannot be deleted")
	done := tc.asTok(t, f.owner, "POST", "/sprints/"+sid1+"/complete", body(map[string]any{"move_unfinished_to": sid2}))
	expect(t, done, 200, "complete")
	if done.get("data.report.completed_count") != float64(1) || done.get("data.report.moved_unfinished_count") != float64(1) {
		t.Fatalf("report: %s", done.Raw)
	}
	var moved string
	testDB.Raw(`SELECT sprint_id::text FROM tasks WHERE id = ?`, a).Row().Scan(&moved)
	if moved != sid2 {
		t.Fatalf("unfinished task must move to the next sprint, got %s", moved)
	}
	if bl := tc.asTok(t, f.viewer, "GET", "/projects/"+pid+"/backlog"); bl.Status != 200 || len(bl.list("data")) != 0 {
		t.Fatalf("backlog: %s", bl.Raw)
	}
	list := tc.asTok(t, f.viewer, "GET", "/projects/"+pid+"/sprints", query("status", "completed"))
	if len(list.list("data")) != 1 {
		t.Fatalf("sprint list: %s", list.Raw)
	}
}

// TestPlanningViewsSearchAndFeed covers VW-01..VW-06, PG9 and the token-authenticated iCalendar route.
func TestPlanningViewsSearchAndFeed(t *testing.T) {
	f := newPlanFixture(t)
	tc, pid := f.tc, f.pid
	soon := time.Now().UTC().AddDate(0, 0, 2)
	f.task(t, "Zebra; migration, plan", f.todo, f.viewerID, &soon)
	f.task(t, "Other task", f.todo, "", nil)

	board := tc.asTok(t, f.viewer, "GET", "/projects/"+pid+"/board", query("per_column", "1"))
	expect(t, board, 200, "board")
	if len(board.list("data.columns")) != 4 {
		t.Fatalf("board: %s", board.Raw)
	}
	cal := tc.asTok(t, f.viewer, "GET", "/projects/"+pid+"/calendar", query("from", soon.Format("2006-01-02"), "to", soon.Format("2006-01-02")))
	if len(cal.list("data.tasks")) != 1 {
		t.Fatalf("calendar: %s", cal.Raw)
	}
	expectErr(t, tc.asTok(t, f.viewer, "GET", "/projects/"+pid+"/calendar", query("from", "2026-01-01", "to", "2026-06-01")), 400, "VALIDATION_ERROR", "window over 62 days")
	expect(t, tc.asTok(t, f.viewer, "GET", "/projects/"+pid+"/gantt"), 200, "gantt")

	mw := tc.asTok(t, f.viewer, "GET", "/my-work")
	if mw.get("data.counts.total") != float64(1) || len(mw.list("data.groups.this_week")) != 1 {
		t.Fatalf("my-work: %s", mw.Raw)
	}
	if c := tc.asTok(t, f.viewer, "GET", "/my-work/counts"); c.get("data.total") != float64(1) {
		t.Fatalf("counts: %s", c.Raw)
	}
	expect(t, tc.asTok(t, f.viewer, "GET", "/calendar", query("from", soon.Format("2006-01-02"), "to", soon.Format("2006-01-02"))), 200, "my calendar")

	// PG9: other tenants and strangers find nothing; members find the task by text and by ticket id.
	if s := tc.asTok(t, f.viewer, "GET", "/search", query("q", "zebra")); len(s.list("data")) != 1 {
		t.Fatalf("search: %s", s.Raw)
	}
	if s := tc.asTok(t, f.stranger, "GET", "/search", query("q", "zebra")); len(s.list("data")) != 0 {
		t.Fatalf("stranger search: %s", s.Raw)
	}
	other := newTenant(t)
	if s := call(t, "GET", "/search", host(other.slug), token(other.token), query("q", "zebra")); s.Status != 200 || len(s.list("data")) != 0 {
		t.Fatalf("PG9 foreign tenant search: %s", s.Raw)
	}
	expectErr(t, tc.asTok(t, f.viewer, "GET", "/search", query("q", "")), 400, "VALIDATION_ERROR", "empty query")

	view := tc.asTok(t, f.viewer, "POST", "/saved-views", body(map[string]any{"name": "Mine", "layout": "list", "project_id": pid, "filters": map[string]any{"mine": true}}))
	expect(t, view, 201, "saved view")
	expectErr(t, tc.asTok(t, f.viewer, "POST", "/saved-views", body(map[string]any{"name": "Bad", "layout": "list", "filters": map[string]any{"sql": "1=1"}})), 400, "VALIDATION_ERROR", "unknown filter")
	expectErr(t, tc.asTok(t, f.viewer, "POST", "/saved-views", body(map[string]any{"name": "Shared", "layout": "list", "project_id": pid, "shared": true})), 403, "FORBIDDEN", "sharing needs view.share")
	if l := tc.asTok(t, f.owner, "GET", "/saved-views"); len(l.list("data")) != 0 {
		t.Fatalf("personal views are private: %s", l.Raw)
	}

	tok := tc.asTok(t, f.viewer, "POST", "/calendar-tokens", body(map[string]any{"label": "phone"}))
	expect(t, tok, 201, "create token")
	raw := tok.str("data.token")
	feed := call(t, "GET", "/calendar/feed.ics", query("token", raw)) // no bearer token, no tenant host
	if feed.Status != 200 || !strings.HasPrefix(feed.Header.Get("Content-Type"), "text/calendar") || !strings.Contains(feed.Raw, `Zebra\; migration\, plan`) {
		t.Fatalf("feed: %d %s", feed.Status, feed.Raw)
	}
	if strings.Contains(feed.Raw, "Other task") {
		t.Fatal("feed must list assigned tasks only")
	}
	expectErr(t, call(t, "GET", "/calendar/feed.ics", query("token", "bogus")), 404, "NOT_FOUND", "unknown token")
	expect(t, tc.asTok(t, f.viewer, "DELETE", "/calendar-tokens/"+tok.str("data.id")), 200, "revoke")
	expectErr(t, call(t, "GET", "/calendar/feed.ics", query("token", raw)), 404, "NOT_FOUND", "revoked token")
	limited := ip("198.51.100.77")
	var last resp
	for i := 0; i < 35; i++ {
		last = call(t, "GET", "/calendar/feed.ics", query("token", "bogus"), limited)
	}
	if last.Status != 429 {
		t.Fatalf("feed must be rate limited per IP, last status %d", last.Status)
	}
}
