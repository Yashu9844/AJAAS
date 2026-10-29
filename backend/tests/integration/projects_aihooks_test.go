package integration

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jaas/jaas/internal/projects/aihooks"
)

// The AI tool catalogue is only useful if every tool points at a route that exists: this keeps it in step with the router.
func TestEveryAIToolRouteExists(t *testing.T) {
	mounted := map[string]bool{}
	for _, r := range engine.Routes() {
		mounted[r.Method+" "+strings.TrimPrefix(r.Path, "/api/v1")] = true
	}
	param := regexp.MustCompile(`\{(\w+)\}`)
	for _, tool := range aihooks.Catalogue() {
		path := param.ReplaceAllString(tool.Route.Path, ":$1")
		if !mounted[tool.Route.Method+" "+path] {
			t.Errorf("tool %s points at %s %s which is not mounted", tool.Name, tool.Route.Method, path)
		}
	}
}

// AI-readiness over HTTP: the tools offered, the context packs and the agent label all follow the caller own permissions.
func TestAIHooksFollowTheCallerPermissions(t *testing.T) {
	tc := newTenant(t)
	owner, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	viewerTok, _, viewerID := tc.limitedUser(t)
	stranger, _, _ := tc.limitedUser(t)

	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "AIH" + uniq()[:2], "name": "AI ready"})).str("data.id")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": viewerID, "role": "viewer"})), 201, "viewer")

	has := func(tok string) map[string]bool {
		r := tc.asTok(t, tok, "GET", "/ai-hooks/tools", query("project_id", pid))
		expect(t, r, 200, "tools")
		out := map[string]bool{}
		for _, it := range r.list("data") {
			out[it.(map[string]any)["name"].(string)] = true
		}
		return out
	}
	if o, v := has(owner), has(viewerTok); !o["create_task"] || !o["delete_task"] || v["create_task"] || v["delete_task"] || !v["get_task"] {
		t.Fatalf("owner %v viewer %v", o, v)
	}
	expectErr(t, tc.asTok(t, stranger, "GET", "/ai-hooks/tools", query("project_id", pid)), 404, "NOT_FOUND", "a stranger sees no tools of a private project")
	if all := tc.asTok(t, stranger, "GET", "/ai-hooks/tools"); len(all.list("data")) != len(aihooks.Catalogue()) {
		t.Fatal("the unfiltered catalogue lists every tool with its requirements")
	}

	// An AI agent acting for the owner: the label is recorded on activity and the permissions stay the owner's.
	task := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", header("X-JAAS-Agent", "claude-test"), body(map[string]any{"title": "Made by an agent"}))
	expect(t, task, 201, "task created by an agent")
	tid := task.str("data.id")
	var source, agent string
	testDB.Raw(`SELECT source, data->>'agent' FROM activity WHERE task_id = ? ORDER BY seq LIMIT 1`, tid).Row().Scan(&source, &agent)
	if source != "ai" || agent != "claude-test" {
		t.Fatalf("activity must show the agent: %q %q", source, agent)
	}
	plain := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", header("X-JAAS-Agent", "Not Valid!"), body(map[string]any{"title": "Bad label ignored"}))
	expect(t, plain, 201, "invalid label is ignored")
	testDB.Raw(`SELECT source FROM activity WHERE task_id = ? ORDER BY seq LIMIT 1`, plain.str("data.id")).Row().Scan(&source)
	if source != "user" {
		t.Fatalf("an invalid label only labels nothing: %q", source)
	}
	expectErr(t, tc.asTok(t, viewerTok, "POST", "/projects/"+pid+"/tasks", header("X-JAAS-Agent", "claude-test"), body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "an agent label never widens permissions")

	// Context packs.
	pack := tc.asTok(t, viewerTok, "GET", "/ai-hooks/context/projects/"+pid)
	expect(t, pack, 200, "project pack")
	if pack.str("data.kind") != "project" || pack.get("data.approx_tokens") == nil {
		t.Fatalf("pack: %s", pack.Raw)
	}
	key := task.str("data.key")
	tpack := tc.asTok(t, viewerTok, "GET", "/ai-hooks/context/tasks/"+key)
	expect(t, tpack, 200, "task pack by ticket key")
	expectErr(t, tc.asTok(t, stranger, "GET", "/ai-hooks/context/tasks/"+key), 404, "NOT_FOUND", "no pack for a task the caller cannot read")

	// Documents feed and readable projects.
	docs := tc.asTok(t, owner, "GET", "/ai-hooks/documents", query("project_id", pid))
	expect(t, docs, 200, "documents")
	if len(docs.list("data")) != 2 {
		t.Fatalf("both tasks are in the feed: %s", docs.Raw)
	}
	expectErr(t, tc.asTok(t, owner, "GET", "/ai-hooks/documents"), 400, "VALIDATION_ERROR", "project_id is required")
	expectErr(t, tc.asTok(t, stranger, "GET", "/ai-hooks/documents", query("project_id", pid)), 404, "NOT_FOUND", "no feed for strangers")
	if ids := tc.asTok(t, viewerTok, "GET", "/ai-hooks/readable-projects"); len(ids.list("data")) != 1 {
		t.Fatalf("readable projects: %s", ids.Raw)
	}
}
