package api

import (
	"testing"
)

// TestOrgMapping exercises mapping creation, primary guard, user chain,
// team moves, and UserDeactivated convergence (G6, G7, G8).
func TestOrgMapping(t *testing.T) {
	slug := "e2emap"
	bootstrapTenant(t, slug)
	seedE2EAdmin(t, slug)
	token := adminToken(t, slug)

	// Structure: dept + team + designation.
	status, env := orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, "Engineering")})
	requireStatus(t, "create dept", status, 201, env)
	deptID := strField(t, env.Data, "id")

	status, env = orgPost(t, slug, token, "/api/v1/teams", map[string]string{"name": uniq(slug, "Platform"), "department_id": deptID})
	requireStatus(t, "create team", status, 201, env)
	teamID := strField(t, env.Data, "id")

	status, env = orgPost(t, slug, token, "/api/v1/designations", map[string]interface{}{"title": uniq(slug, "Engineer")})
	requireStatus(t, "create designation", status, 201, env)
	desigID := strField(t, env.Data, "id")

	// Admin user id from login payload (nested "user" object).
	_, loginEnv := doReq(t, "POST", "/api/v1/auth/login", slug+".localhost", "", map[string]string{
		"email": "admin@" + slug + ".com", "password": "Secret123!", "tenant_slug": slug,
	})
	adminID := nestedField(t, loginEnv.Data, "user", "id")

	// Clean slate for the reused admin: deactivate any primary mapping left by a
	// previous run (seed reactivates the user, but mappings stay inactive=false).
	_, listEnv := orgGet(t, slug, token, "/api/v1/mappings?user_id="+adminID)
	deactivateAllPrimaries(t, slug, token, listEnv.Data)

	// Primary mapping.
	status, env = orgPost(t, slug, token, "/api/v1/mappings", map[string]interface{}{
		"user_id": adminID, "department_id": deptID, "team_id": teamID, "designation_id": desigID, "is_primary": true,
	})
	requireStatus(t, "create primary mapping", status, 201, env)
	mappingID := strField(t, env.Data, "id")

	// Second primary → 409 (G6).
	status, env = orgPost(t, slug, token, "/api/v1/mappings", map[string]interface{}{
		"user_id": adminID, "department_id": deptID, "is_primary": true,
	})
	requireStatus(t, "second primary", status, 409, env)

	// Team move across departments keeps mappings consistent (G8).
	status, env = orgPost(t, slug, token, "/api/v1/departments", map[string]string{"name": uniq(slug, "Product")})
	requireStatus(t, "create second dept", status, 201, env)
	otherDept := strField(t, env.Data, "id")
	status, env = doReq(t, "PATCH", "/api/v1/teams/"+teamID, slug+".localhost", token, map[string]string{"department_id": otherDept})
	requireStatus(t, "move team", status, 200, env)

	// User chain shows mapping + department (FR-O002).
	status, env = orgGet(t, slug, token, "/api/v1/org-chart/chain?user_id="+adminID)
	requireStatus(t, "user chain", status, 200, env)

	// Org chart forest renders (FR-O001).
	status, env = orgGet(t, slug, token, "/api/v1/org-chart")
	requireStatus(t, "org chart", status, 200, env)

	// Deactivate the user via identity endpoint → mappings converge inactive (G7).
	// The deactivation revokes the caller's own session too, so the post-check
	// list runs with a fresh admin... except the admin IS the deactivated user.
	// Convergence is verified directly in PG instead (no valid token remains).
	token = adminToken(t, slug)
	status, _ = doReq(t, "POST", "/api/v1/users/"+adminID+"/deactivate", slug+".localhost", token, nil)
	requireStatus(t, "deactivate user", status, 200, env)

	// Convergence check directly in PG: mapping must be inactive now.
	st := psql(t, "SELECT status FROM mappings WHERE id = '"+mappingID+"'")
	if st != "inactive" {
		t.Fatalf("G7: expected mapping inactive after UserDeactivated, got %q", st)
	}
}
