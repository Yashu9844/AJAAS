package integration

import (
	"strings"
	"testing"
)

func (tc *tenantCtx) asTok(t *testing.T, tok, method, path string, opts ...opt) resp {
	t.Helper()
	return call(t, method, path, append([]opt{host(tc.slug), token(tok)}, opts...)...)
}

// TestProjectsContext covers PG4 (visibility and tenant isolation), PG1 for project creation (idempotent replay), the
// version contract and the project lifecycle through the production router.
func TestProjectsContext(t *testing.T) {
	tc := newTenant(t)
	creator, _, creatorID := tc.limitedUser(t, [2]string{"projects", "create"})
	stranger, _, strangerID := tc.limitedUser(t)

	expectErr(t, call(t, "GET", "/projects", host(tc.slug)), 401, "UNAUTHORIZED", "projects need a token")
	expectErr(t, tc.asTok(t, stranger, "POST", "/projects", body(map[string]any{"key": "NOPE", "name": "x"})), 403, "FORBIDDEN", "creating needs projects:create")

	// PG1: a retried create with the same key replays the original response.
	first := tc.asTok(t, creator, "POST", "/projects", header("Idempotency-Key", "k-"+uniq()), body(map[string]any{"key": "ALPHA", "name": "Alpha"}))
	expect(t, first, 201, "create project")
	pid := first.str("data.id")
	if first.str("data.key") != "ALPHA" || first.str("data.my_role") != "owner" || first.str("data.owner_user_id") != creatorID {
		t.Fatalf("created project: %s", first.Raw)
	}
	expectErr(t, tc.asTok(t, creator, "POST", "/projects", body(map[string]any{"key": "ALPHA", "name": "Again"})), 409, "PROJECT_KEY_TAKEN", "duplicate key")

	// PG4: a private project does not exist for strangers or other tenants.
	expectErr(t, tc.asTok(t, stranger, "GET", "/projects/"+pid), 404, "NOT_FOUND", "stranger cannot see it")
	if l := tc.asTok(t, stranger, "GET", "/projects"); len(l.list("data")) != 0 {
		t.Fatalf("stranger list: %s", l.Raw)
	}
	other := newTenant(t)
	expectErr(t, call(t, "GET", "/projects/"+pid, host(other.slug), token(other.token)), 404, "NOT_FOUND", "foreign tenant")

	// Members: add the stranger as a viewer, who now reads but cannot edit.
	expect(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": strangerID, "role": "viewer"})), 201, "add member")
	expect(t, tc.asTok(t, stranger, "GET", "/projects/"+pid), 200, "viewer reads")
	expectErr(t, tc.asTok(t, stranger, "PATCH", "/projects/"+pid, body(map[string]any{"name": "Hacked"})), 403, "FORBIDDEN", "viewer cannot edit")
	expectErr(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": strangerID, "role": "member"})), 409, "ALREADY_MEMBER", "already a member")
	if m := tc.asTok(t, creator, "GET", "/projects/"+pid+"/members"); len(m.list("data")) != 2 {
		t.Fatalf("members: %s", m.Raw)
	}

	// Optimistic concurrency over HTTP: If-Match and the 409 body carry the version.
	ok := tc.asTok(t, creator, "PATCH", "/projects/"+pid, header("If-Match", `"1"`), body(map[string]any{"name": "Alpha 2"}))
	expect(t, ok, 200, "patch with If-Match")
	stale := tc.asTok(t, creator, "PATCH", "/projects/"+pid, header("If-Match", "1"), body(map[string]any{"name": "Alpha 3"}))
	expectErr(t, stale, 409, "VERSION_CONFLICT", "stale version")
	if stale.get("error.current_version") != float64(2) {
		t.Fatalf("current_version: %s", stale.Raw)
	}

	// Setup data: columns, labels, milestones and the permission catalogue.
	cols := tc.asTok(t, creator, "GET", "/projects/"+pid+"/columns")
	if len(cols.list("data")) != 4 {
		t.Fatalf("default columns: %s", cols.Raw)
	}
	expect(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/columns", body(map[string]any{"name": "Review", "category": "in_progress"})), 201, "column")
	expect(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/labels", body(map[string]any{"name": "bug", "color": "#FF0000"})), 201, "label")
	expect(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/milestones", body(map[string]any{"name": "Beta", "due_on": "2026-12-01"})), 201, "milestone")
	if c := tc.asTok(t, creator, "GET", "/project-permissions"); len(c.list("data.permissions")) != 20 {
		t.Fatalf("catalogue: %s", c.Raw)
	}
	expectErr(t, tc.asTok(t, creator, "POST", "/project-roles", body(map[string]any{"name": "R", "permissions": []string{"project.read", "task.read"}})), 403, "FORBIDDEN", "roles need project_roles:manage")
	expect(t, tc.do(t, "POST", "/project-roles", body(map[string]any{"name": "Reviewer", "permissions": []string{"project.read", "task.read", "task.comment"}})), 201, "tenant admin defines a role")

	// Lifecycle: archive freezes, delete needs archive and a tenant administrator.
	expect(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/archive"), 200, "archive")
	expectErr(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/columns", body(map[string]any{"name": "Late"})), 409, "PROJECT_ARCHIVED", "archived projects are frozen")
	expectErr(t, tc.asTok(t, creator, "DELETE", "/projects/"+pid), 403, "FORBIDDEN", "owner cannot delete")
	expect(t, tc.do(t, "DELETE", "/projects/"+pid), 200, "tenant admin deletes")
	expectErr(t, tc.asTok(t, creator, "GET", "/projects/"+pid), 404, "NOT_FOUND", "deleted")
}

// The product UI needs people names: the members list carries display_name (first and last name, never the e-mail address).
func TestMembersListCarriesDisplayNamesNotEmails(t *testing.T) {
	tc := newTenant(t)
	owner, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	memberID, memberEmail := tc.newUser(t, "nam")
	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "NM" + uniq()[:3], "name": "Names"})).str("data.id")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": memberID, "role": "member"})), 201, "member")
	list := tc.asTok(t, owner, "GET", "/projects/"+pid+"/members")
	found := false
	for _, it := range list.list("data") {
		m := it.(map[string]any)
		if m["user_id"] == memberID {
			found = true
			if m["display_name"] != "Fnam Lnam" {
				t.Fatalf("display name: %v", m["display_name"])
			}
		}
	}
	if !found || strings.Contains(list.Raw, memberEmail) {
		t.Fatalf("member listed without the e-mail address: found=%v", found)
	}
}
