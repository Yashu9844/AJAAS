package api

import (
	"testing"
)

// TestOrgHierarchy exercises cycle rejection, depth cap, and subtree moves (G2, G3).
func TestOrgHierarchy(t *testing.T) {
	slug := "e2ehier"
	bootstrapTenant(t, slug)
	seedE2EAdmin(t, slug)
	token := adminToken(t, slug)

	// Root + child.
	status, env := orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, "HQ")})
	requireStatus(t, "create root", status, 201, env)
	rootID := strField(t, env.Data, "id")

	status, env = orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, "Region"), "parent_department_id": rootID})
	requireStatus(t, "create child", status, 201, env)
	childID := strField(t, env.Data, "id")

	// Self-parent → 400 (G2).
	status, env = doReq(t, "PATCH", "/api/v1/departments/"+childID, slug+".localhost", token, map[string]string{"parent_department_id": childID})
	requireStatus(t, "self parent", status, 400, env)

	// Move root under child → 409 cycle (G2).
	status, env = doReq(t, "PATCH", "/api/v1/departments/"+rootID, slug+".localhost", token, map[string]string{"parent_department_id": childID})
	requireStatus(t, "cycle move", status, 409, env)

	// TooDeep at 12th level → 422 (G3). Root depth 0 + Region depth 1 + 9
	// more levels (depths 2..10, walk 10 == cap → allowed); the 12th node
	// walks 11 > cap → HIERARCHY_TOO_DEEP.
	parent := childID
	for i := 0; i < 9; i++ {
		status, env = orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, deptName(i)), "parent_department_id": parent})
		requireStatus(t, "chain build", status, 201, env)
		parent = strField(t, env.Data, "id")
	}
	status, env = orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, "TooDeep"), "parent_department_id": parent})
	requireStatus(t, "depth cap", status, 422, env)
	if env.Error == nil || env.Error.Code != "HIERARCHY_TOO_DEEP" {
		t.Fatalf("expected HIERARCHY_TOO_DEEP, got %+v", env.Error)
	}
}

func deptName(i int) string {
	names := []string{"L3a", "L4a", "L5a", "L6a", "L7a", "L8b", "L9b", "L10b", "L11b"}
	return names[i]
}
