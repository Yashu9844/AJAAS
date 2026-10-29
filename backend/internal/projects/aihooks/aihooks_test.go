package aihooks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

func TestCatalogueIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range Catalogue() {
		if tool.Name == "" || tool.Description == "" || seen[tool.Name] {
			t.Fatalf("name and description are required, names unique: %+v", tool)
		}
		seen[tool.Name] = true
		if tool.Risk != RiskRead && tool.Risk != RiskWrite && tool.Risk != RiskDestructive {
			t.Errorf("%s: risk", tool.Name)
		}
		if tool.Requires != "" && !api.ValidPerm(string(tool.Requires)) {
			t.Errorf("%s: unknown permission %s", tool.Name, tool.Requires)
		}
		if tool.Risk == RiskDestructive && !tool.Confirm {
			t.Errorf("%s: destructive tools must ask for confirmation", tool.Name)
		}
		if tool.Risk == RiskRead && (tool.Route.Method != "GET" || tool.Idempotent) {
			t.Errorf("%s: read tools are GET without idempotency keys", tool.Name)
		}
		if tool.InputSchema["type"] != "object" {
			t.Errorf("%s: schema", tool.Name)
		}
	}
}

func TestToolsAreFilteredByWhatTheUserMayDo(t *testing.T) {
	e := newEnv(t)
	owner, viewer, stranger := e.user(false), e.user(false), e.user(false)
	p, _ := e.task(owner, "AIT")
	_, _ = e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: viewer.UserID, Role: "viewer"})

	names := func(a api.Actor) map[string]bool {
		tools, err := e.svc.ToolsFor(bg, a, &p.ID)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, tl := range tools {
			out[tl.Name] = true
		}
		return out
	}
	ownerTools, viewerTools := names(owner), names(viewer)
	if !ownerTools["delete_task"] || !ownerTools["create_task"] || !ownerTools["get_task"] {
		t.Fatalf("owner tools: %v", ownerTools)
	}
	if viewerTools["create_task"] || viewerTools["delete_task"] || viewerTools["add_comment"] || !viewerTools["get_task"] || !viewerTools["search"] {
		t.Fatalf("a viewer is offered reads only: %v", viewerTools)
	}
	if _, err := e.svc.ToolsFor(bg, stranger, &p.ID); err == nil {
		t.Fatal("a stranger cannot even list the tools of a private project (404)")
	}
	all, err := e.svc.ToolsFor(bg, stranger, nil)
	if err != nil || len(all) != len(Catalogue()) {
		t.Fatalf("without a project the full catalogue is shown with its requirements: %v %d", err, len(all))
	}
}

func TestProjectContextIsCompactAndPermissionAware(t *testing.T) {
	e := newEnv(t)
	owner, viewer, stranger := e.user(false), e.user(false), e.user(false)
	p, tid := e.task(owner, "CTX")
	_, _ = e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: viewer.UserID, Role: "viewer"})
	e.db.Exec(`UPDATE tasks SET due_at = now() - interval '1 day' WHERE id = ?`, tid)
	e.db.Exec(`INSERT INTO labels (id, tenant_id, project_id, name) VALUES (gen_random_uuid(), ?, ?, 'bug')`, e.tenant, p.ID)

	pack, err := e.svc.ProjectContext(bg, viewer, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pack)
	if pack.Kind != "project" || pack.ActingAs != viewer.UserID || pack.ApproxTokens < 1 || pack.ApproxTokens > 2000 {
		t.Fatalf("pack: %+v", pack)
	}
	counts := pack.Data["counts"].(countsRow)
	if counts.Open != 1 || counts.Overdue != 1 || counts.Unassigned != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	perms := pack.Data["permissions"].([]string)
	if strings.Contains(strings.Join(perms, ","), "task.create") || !strings.Contains(strings.Join(perms, ","), "task.read") {
		t.Fatalf("the pack tells the model what the viewer may do: %v", perms)
	}
	if !strings.Contains(string(raw), `"bug"`) {
		t.Fatalf("labels are included: %s", raw)
	}
	if _, err = e.svc.ProjectContext(bg, stranger, p.ID); err == nil {
		t.Fatal("no context for a project the user cannot read")
	}
}

func TestTaskContextByKeyHidesUnreadableRelationsAndCapsText(t *testing.T) {
	e := newEnv(t)
	owner, other := e.user(false), e.user(false)
	p, tid := e.task(owner, "TKC")
	q, tid2 := e.task(other, "PRIV") // a private project the owner cannot read
	e.db.Exec(`UPDATE tasks SET description = ? WHERE id = ?`, strings.Repeat("d", 5000), tid)
	e.db.Exec(`INSERT INTO task_relations (id, tenant_id, source_task_id, target_task_id, type, created_by) VALUES (gen_random_uuid(), ?, ?, ?, 'related', ?)`, e.tenant, tid, tid2, owner.UserID)
	e.db.Exec(`INSERT INTO comments (id, tenant_id, task_id, author_user_id, body) VALUES (gen_random_uuid(), ?, ?, ?, ?)`, e.tenant, tid, owner.UserID, strings.Repeat("c", 900))

	pack, err := e.svc.TaskContext(bg, owner, "TKC-1")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pack)
	task := pack.Data["task"].(taskRow)
	if task.Key != "TKC-1" || len([]rune(task.Desc)) != descMax || len(pack.Truncated) == 0 {
		t.Fatalf("description is capped and flagged: %d %v", len([]rune(task.Desc)), pack.Truncated)
	}
	if strings.Contains(string(raw), "PRIV-1") {
		t.Fatalf("a relation into a project the caller cannot read is invisible: %s", raw)
	}
	comments := pack.Data["latest_comments"].([]commentRow)
	if len(comments) != 1 || len([]rune(comments[0].Excerpt)) != commentMax {
		t.Fatalf("comments are excerpts: %+v", comments)
	}
	for _, ref := range []string{"nope", "TKC-99", "TKC-x", uuid.NewString()} {
		if _, err = e.svc.TaskContext(bg, owner, ref); err == nil {
			t.Errorf("%s: unknown tasks are 404", ref)
		}
	}
	if _, err = e.svc.TaskContext(bg, owner, q.Key+"-1"); err == nil {
		t.Fatal("a task in a project the caller cannot read is 404")
	}
	if _, err = e.svc.TaskContext(bg, owner, tid.String()); err != nil {
		t.Fatalf("by id: %v", err)
	}
	_ = p
}

func TestDocumentsFeedIsIncrementalWithTombstones(t *testing.T) {
	e := newEnv(t)
	owner, stranger := e.user(false), e.user(false)
	p, tid := e.task(owner, "DOC")
	e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank)
		SELECT gen_random_uuid(), ?, ?, 2, 'Second', column_id, ?, 'W' FROM tasks WHERE id = ?`, e.tenant, p.ID, owner.UserID, tid)
	e.db.Exec(`INSERT INTO comments (id, tenant_id, task_id, author_user_id, body) VALUES (gen_random_uuid(), ?, ?, ?, 'hello')`, e.tenant, tid, owner.UserID)

	docs, next, err := e.svc.Documents(bg, owner, DocQuery{ProjectID: p.ID, Kind: "task", Cursor: "", Limit: 1})
	if err != nil || len(docs) != 1 || next == "" || docs[0].Hash == "" {
		t.Fatalf("page 1: %v %q %v", docs, next, err)
	}
	more, next2, err := e.svc.Documents(bg, owner, DocQuery{ProjectID: p.ID, Kind: "task", Cursor: next, Limit: 5})
	if err != nil || len(more) != 1 || next2 != "" || more[0].ID == docs[0].ID {
		t.Fatalf("page 2 continues after the cursor: %v %v", more, err)
	}
	com, _, err := e.svc.Documents(bg, owner, DocQuery{ProjectID: p.ID, Kind: "comment", Cursor: "", Limit: 10})
	if err != nil || len(com) != 1 || com[0].Text != "hello" || com[0].Kind != "comment" {
		t.Fatalf("comments feed: %v %v", com, err)
	}
	e.db.Exec(`UPDATE tasks SET deleted_at = now(), updated_at = now() WHERE id = ?`, tid)
	all, _, _ := e.svc.Documents(bg, owner, DocQuery{ProjectID: p.ID, Kind: "", Cursor: "", Limit: 50})
	var tomb bool
	for _, d := range all {
		if d.ID == tid.String() && d.Deleted && d.Text == "" {
			tomb = true
		}
	}
	if !tomb {
		t.Fatalf("a deleted task becomes a tombstone without text: %+v", all)
	}
	if _, _, err = e.svc.Documents(bg, owner, DocQuery{ProjectID: p.ID, Kind: "wiki", Cursor: "", Limit: 10}); err == nil {
		t.Fatal("unknown kind")
	}
	if _, _, err = e.svc.Documents(bg, owner, DocQuery{ProjectID: p.ID, Kind: "task", Cursor: "%%", Limit: 10}); err == nil {
		t.Fatal("bad cursor")
	}
	if _, _, err = e.svc.Documents(bg, stranger, DocQuery{ProjectID: p.ID, Kind: "task", Cursor: "", Limit: 10}); err == nil {
		t.Fatal("no feed for a project the caller cannot read")
	}
	ids, err := e.svc.ReadableProjects(bg, owner)
	if err != nil || len(ids) != 1 || ids[0] != p.ID {
		t.Fatalf("readable projects: %v %v", ids, err)
	}
}
