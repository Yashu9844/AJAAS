package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jaas/jaas/internal/projects/automation/bridge"
	"github.com/jaas/jaas/internal/projects/collab"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/task"
)

// automationSuite builds a second automation engine on the integration database with the real task and collaboration
// services; the test application runs without background workers, so the test ticks it by hand.
func automationSuite() *bridge.Suite {
	proj := project.NewService(testDB, project.Ports{})
	tasks := task.NewService(testDB, proj, task.Ports{})
	cl := collab.NewService(testDB, collab.Ports{Authorizer: proj, Tasks: tasks})
	s := bridge.NewSuite(bridge.Deps{DB: testDB, Authz: proj, Tasks: tasks, Collab: cl})
	s.Engine.SetSettle(0)
	return s
}

func drain(t *testing.T, s *bridge.Suite) {
	t.Helper()
	for i := 0; i < 100; i++ {
		n, err := s.Engine.Tick(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
	}
}

func columnID(t *testing.T, tc *tenantCtx, tok, pid, category string) string {
	t.Helper()
	for _, c := range tc.asTok(t, tok, "GET", "/projects/"+pid+"/columns").list("data") {
		if m := c.(map[string]any); m["category"] == category {
			return m["id"].(string)
		}
	}
	t.Fatalf("no %s column", category)
	return ""
}

// Rules, recurring tasks and public links end to end through the production router.
func TestAutomationRecurrenceAndPublicLinksThroughTheRouter(t *testing.T) {
	tc := newTenant(t)
	owner, _, ownerID := tc.limitedUser(t, [2]string{"projects", "create"})
	member, _, memberID := tc.limitedUser(t)
	stranger, _, _ := tc.limitedUser(t)
	suite := automationSuite()

	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "AUT" + uniq()[:3], "name": "Automation"})).str("data.id")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": memberID, "role": "member"})), 201, "member")
	todo, progress, done := columnID(t, tc, owner, pid, "todo"), columnID(t, tc, owner, pid, "in_progress"), columnID(t, tc, owner, pid, "done")

	// --- rules
	rule := map[string]any{"name": "Urgent to done", "trigger": "task.moved",
		"conditions": []any{map[string]any{"field": "priority", "value": "urgent"}},
		"actions":    []any{map[string]any{"type": "move_to_column", "column_id": done}}}
	expectErr(t, tc.asTok(t, member, "POST", "/projects/"+pid+"/rules", body(rule)), 403, "FORBIDDEN", "managing rules needs project.update")
	expectErr(t, tc.asTok(t, stranger, "GET", "/projects/"+pid+"/rules"), 404, "NOT_FOUND", "outsiders see nothing")
	expectErr(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/rules", body(map[string]any{"name": "Bad", "trigger": "task.moved",
		"actions": []any{map[string]any{"type": "run_script"}}})), 400, "VALIDATION_ERROR", "closed action vocabulary")
	created := tc.asTok(t, owner, "POST", "/projects/"+pid+"/rules", body(rule))
	expect(t, created, 201, "create rule")
	rid := created.str("data.id")
	expectErr(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/rules", body(rule)), 409, "RULE_NAME_TAKEN", "names are unique")
	if l := tc.asTok(t, member, "GET", "/projects/"+pid+"/rules"); len(l.list("data")) != 1 {
		t.Fatalf("members can read rules: %s", l.Raw)
	}

	urgent := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", body(map[string]any{"title": "Fire", "priority": "urgent", "column_id": todo})).str("data.id")
	calm := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", body(map[string]any{"title": "Calm", "priority": "low", "column_id": todo})).str("data.id")
	dry := tc.asTok(t, owner, "POST", "/projects/"+pid+"/rules/"+rid+"/test", body(map[string]any{"task_id": urgent}))
	expect(t, dry, 200, "dry run")
	if dry.get("data.matches") != true || dry.get("data.actions.0.would_run") != true {
		t.Fatalf("dry run: %s", dry.Raw)
	}
	if tc.asTok(t, owner, "GET", "/tasks/"+urgent).str("data.column_id") != todo {
		t.Fatal("a dry run changes nothing")
	}
	for _, id := range []string{urgent, calm} {
		expect(t, tc.asTok(t, owner, "POST", "/tasks/"+id+"/move", body(map[string]any{"column_id": progress})), 200, "a person moves the task")
	}
	drain(t, suite)
	if col := tc.asTok(t, owner, "GET", "/tasks/"+urgent).str("data.column_id"); col != done {
		t.Fatalf("the urgent task is moved to Done by the rule, is in %s", col)
	}
	if col := tc.asTok(t, owner, "GET", "/tasks/"+calm).str("data.column_id"); col != progress {
		t.Fatal("other priorities are left alone")
	}
	runs := tc.asTok(t, owner, "GET", "/projects/"+pid+"/rules/"+rid+"/runs")
	expect(t, runs, 200, "runs")
	if len(runs.list("data")) != 1 || runs.str("data.0.status") != "ok" {
		t.Fatalf("exactly one ok run, no cascade: %s", runs.Raw)
	}
	var agent string
	testDB.Raw(`SELECT data->>'agent' FROM activity WHERE task_id = ? AND type = 'moved' ORDER BY seq DESC LIMIT 1`, urgent).Row().Scan(&agent)
	if !strings.HasPrefix(agent, "automation.") {
		t.Fatalf("the change is labelled as automation: %q", agent)
	}
	expect(t, tc.asTok(t, owner, "PATCH", "/projects/"+pid+"/rules/"+rid, body(map[string]any{"active": false})), 200, "disable")
	expect(t, tc.asTok(t, owner, "DELETE", "/projects/"+pid+"/rules/"+rid), 200, "delete rule")
	expectErr(t, tc.asTok(t, owner, "GET", "/projects/"+pid+"/rules/"+rid+"/runs"), 404, "NOT_FOUND", "deleted rule")

	// --- recurrence
	rec := tc.asTok(t, owner, "POST", "/projects/"+pid+"/recurrences", body(map[string]any{"template_task_id": calm, "frequency": "weekly"}))
	expect(t, rec, 201, "create recurrence")
	expectErr(t, tc.asTok(t, member, "POST", "/projects/"+pid+"/recurrences/"+rec.str("data.id")+"/run-now"), 403, "FORBIDDEN", "run-now needs project.update")
	run := tc.asTok(t, owner, "POST", "/projects/"+pid+"/recurrences/"+rec.str("data.id")+"/run-now")
	expect(t, run, 201, "run now")
	copyID := run.str("data.task_id")
	cp := tc.asTok(t, owner, "GET", "/tasks/"+copyID)
	if cp.str("data.title") != "Calm" || copyID == calm {
		t.Fatalf("the copy is titled like the template: %s", cp.Raw)
	}
	expectErr(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/recurrences", body(map[string]any{"template_task_id": calm, "frequency": "yearly"})), 400, "VALIDATION_ERROR", "frequency")
	if l := tc.asTok(t, owner, "GET", "/projects/"+pid+"/recurrences"); len(l.list("data")) != 1 {
		t.Fatalf("list: %s", l.Raw)
	}

	// --- public link
	link := tc.asTok(t, owner, "POST", "/projects/"+pid+"/public-links", body(map[string]any{}))
	expect(t, link, 201, "create link")
	tok := link.str("data.token")
	expectErr(t, tc.asTok(t, member, "POST", "/projects/"+pid+"/public-links", body(map[string]any{})), 403, "FORBIDDEN", "links need project.update")
	pub := call(t, "GET", "/public/projects/"+tok) // no bearer token, no tenant host
	expect(t, pub, 200, "public page")
	for _, banned := range []string{ownerID, memberID, tc.id, tc.adminID, pid, "assignee", "reporter", "comment", "user_id"} {
		if strings.Contains(pub.Raw, banned) {
			t.Errorf("the public JSON must not contain %q", banned)
		}
	}
	if pub.str("data.project.name") != "Automation" || len(pub.list("data.tasks")) < 2 || pub.Header.Get("X-Robots-Tag") != "noindex" ||
		pub.Header.Get("Cache-Control") != "no-store" || pub.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("public page: %d %s %v", pub.Status, pub.Raw, pub.Header)
	}
	for _, k := range []string{"key", "title", "column", "priority", "due_at", "labels", "progress"} {
		if _, ok := pub.get("data.tasks.0").(map[string]any)[k]; !ok {
			t.Errorf("task member %q missing", k)
		}
	}
	unknown := call(t, "GET", "/public/projects/"+strings.Repeat("a", 43))
	expectErr(t, unknown, 404, "NOT_FOUND", "unknown token")
	expect(t, tc.asTok(t, owner, "DELETE", "/projects/"+pid+"/public-links/"+link.str("data.id")), 200, "revoke")
	revoked := call(t, "GET", "/public/projects/"+tok)
	if revoked.Status != 404 || revoked.Raw != unknown.Raw {
		t.Fatalf("revoked and unknown tokens answer identically: %s vs %s", revoked.Raw, unknown.Raw)
	}
	limited := ip("198.51.100.99")
	var last resp
	for i := 0; i < 70; i++ {
		last = call(t, "GET", "/public/projects/bogus", limited)
	}
	if last.Status != 429 {
		t.Fatalf("the public route is rate limited per IP, last status %d", last.Status)
	}
}
