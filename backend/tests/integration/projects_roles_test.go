package integration

import (
	"testing"
)

// DPJ-49 end to end with the real M0: the role a user holds on a project decides what they can do there, a change to the
// role applies at once, a role on one project grants nothing on another, and M0 refuses to use a tenant role as a project
// role. Specification section 8; docs/modules/m0-identity-access/resource-roles.md.
func TestProjectRolesAreDecidedByIdentity(t *testing.T) {
	tc := newTenant(t)
	owner, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	user, _, userID := tc.limitedUser(t)
	pa := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "RA" + uniq()[:3], "name": "Roles A"})).str("data.id")
	pb := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "RB" + uniq()[:3], "name": "Roles B"})).str("data.id")

	// The predefined project roles exist and the manager of a project can list them without any M0 role management right.
	options := tc.asTok(t, owner, "GET", "/projects/"+pa+"/roles").list("data")
	if len(options) < 4 {
		t.Fatalf("assignable roles: %v", options)
	}
	expectErr(t, tc.asTok(t, user, "GET", "/projects/"+pa+"/roles"), 404, "NOT_FOUND", "a stranger cannot even see the project")

	// A viewer reads but cannot create tasks.
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pa+"/members", body(map[string]any{"user_id": userID, "role": "viewer"})), 201, "viewer")
	expect(t, tc.asTok(t, user, "GET", "/projects/"+pa), 200, "viewer reads")
	expectErr(t, tc.asTok(t, user, "POST", "/projects/"+pa+"/tasks", body(map[string]any{"title": "No"})), 403, "FORBIDDEN", "viewer cannot create")

	// A role change in the project applies on the next request.
	expect(t, tc.asTok(t, owner, "PATCH", "/projects/"+pa+"/members/"+userID, body(map[string]any{"role": "member"})), 200, "promote to member")
	expect(t, tc.asTok(t, user, "POST", "/projects/"+pa+"/tasks", body(map[string]any{"title": "Yes"})), 201, "member creates")

	// The role grants nothing on another project, even a readable-by-nobody private one.
	expectErr(t, tc.asTok(t, user, "GET", "/projects/"+pb), 404, "NOT_FOUND", "project B stays invisible")

	// Editing the role in M0 changes what holders can do at once: remove task:create from a custom role.
	read, taskRead, create := tc.permID(t, "project", "read"), tc.permID(t, "task", "read"), tc.permID(t, "task", "create")
	custom := tc.do(t, "POST", "/roles", body(map[string]any{"name": "Triage " + uniq(), "applies_to": "project", "permission_ids": []string{read, taskRead, create}}))
	expect(t, custom, 201, "custom project role")
	customID := custom.str("data.id")
	expect(t, tc.asTok(t, owner, "PATCH", "/projects/"+pa+"/members/"+userID, body(map[string]any{"role_id": customID})), 200, "assign custom role")
	expect(t, tc.asTok(t, user, "POST", "/projects/"+pa+"/tasks", body(map[string]any{"title": "Triage"})), 201, "custom role can create")
	expect(t, tc.do(t, "DELETE", "/roles/"+customID+"/permissions/"+create), 200, "role loses task:create")
	expectErr(t, tc.asTok(t, user, "POST", "/projects/"+pa+"/tasks", body(map[string]any{"title": "Late"})), 403, "FORBIDDEN", "the edit applies at once")

	// A tenant role is not a project role, and an unknown role is refused.
	expectErr(t, tc.asTok(t, owner, "PATCH", "/projects/"+pa+"/members/"+userID, body(map[string]any{"role_id": tc.roleID(t, "Employee")})), 400, "VALIDATION_ERROR", "tenant role as project role")
	expectErr(t, tc.asTok(t, owner, "PATCH", "/projects/"+pa+"/members/"+userID, body(map[string]any{"role_id": "5b0e0e0e-0000-4000-8000-000000000000"})), 400, "VALIDATION_ERROR", "unknown role")

	// Removing the member takes the access away.
	expect(t, tc.asTok(t, owner, "DELETE", "/projects/"+pa+"/members/"+userID), 200, "remove member")
	expectErr(t, tc.asTok(t, user, "GET", "/projects/"+pa), 404, "NOT_FOUND", "removed member lost access")
}
