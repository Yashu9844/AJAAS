package api

import (
	"fmt"
	"testing"
)

// TestOrgLifecycle exercises the department/team/designation lifecycle end to
// end (G5): create → duplicate 409 → deactivate guard 409 → force cascade.
func TestOrgLifecycle(t *testing.T) {
	slug := "e2elife"
	bootstrapTenant(t, slug)
	seedE2EAdmin(t, slug)
	token := adminToken(t, slug)

	// Department create.
	deptName := uniq(slug, "Engineering")
	status, env := orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": deptName})
	requireStatus(t, "create department", status, 201, env)
	deptID := strField(t, env.Data, "id")

	// Duplicate name → 409 (FR-D002, case-insensitive).
	status, env = orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": deptName})
	requireStatus(t, "duplicate department", status, 409, env)

	// Team create under the department.
	status, env = orgPost(t, slug, token, "/api/v1/teams", map[string]string{"name": uniq(slug, "Platform"), "department_id": deptID})
	requireStatus(t, "create team", status, 201, env)
	teamID := strField(t, env.Data, "id")

	// Same team name in another department is allowed (FR-T002 scope).
	status, env = orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, "Product")})
	requireStatus(t, "create second department", status, 201, env)
	otherDept := strField(t, env.Data, "id")
	status, env = orgPost(t, slug, token, "/api/v1/teams", map[string]string{"name": uniq(slug, "Platform"), "department_id": otherDept})
	requireStatus(t, "same team name other department", status, 201, env)

	// Designation create.
	status, env = orgPost(t, slug, token, "/api/v1/designations", map[string]interface{}{"title": uniq(slug, "Senior Engineer"), "level": 3})
	requireStatus(t, "create designation", status, 201, env)
	desigID := strField(t, env.Data, "id")

	// Deactivate blocked while teams reference it (FR-D007, G5).
	status, env = orgPost(t, slug, token, "/api/v1/departments/"+deptID+"/deactivate", nil)
	requireStatus(t, "deactivate guarded", status, 409, env)

	// Force cascade deactivates children (still 200).
	status, env = orgPost(t, slug, token, "/api/v1/departments/"+deptID+"/deactivate?force=true", map[string]string{"reason": "reorg"})
	requireStatus(t, "force deactivate", status, 200, env)

	// Double deactivate → 409.
	status, env = orgPost(t, slug, token, "/api/v1/departments/"+deptID+"/deactivate", nil)
	requireStatus(t, "double deactivate", status, 409, env)

	// Team deactivate (no mappings) succeeds; designation deactivate succeeds.
	status, env = orgPost(t, slug, token, "/api/v1/teams/"+teamID+"/deactivate", nil)
	requireStatus(t, "deactivate team", status, 200, env)
	status, env = orgPost(t, slug, token, "/api/v1/designations/"+desigID+"/deactivate", nil)
	requireStatus(t, "deactivate designation", status, 200, env)

	_ = fmt.Sprint()
}
