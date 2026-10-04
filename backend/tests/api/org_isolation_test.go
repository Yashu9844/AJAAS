package api

import (
	"testing"
)

// TestOrgIsolation verifies cross-tenant access returns 404 (never a 403 leak)
// and JWT-on-wrong-subdomain returns 403 (G1).
func TestOrgIsolation(t *testing.T) {
	slugA, slugB := "e2eisoa", "e2eisob"
	bootstrapTenant(t, slugA)
	bootstrapTenant(t, slugB)
	seedE2EAdmin(t, slugA)
	seedE2EAdmin(t, slugB)
	tokA := adminToken(t, slugA)
	tokB := adminToken(t, slugB)

	// Dept in tenant A.
	status, env := orgPost(t, slugA, tokA, "/api/v1/departments", map[string]string{"name": uniq(slugA, "Secret")})
	requireStatus(t, "create dept A", status, 201, env)
	deptA := strField(t, env.Data, "id")

	// Tenant-B token reading A's dept → 404 (G1).
	status, env = orgGet(t, slugB, tokB, "/api/v1/departments/"+deptA)
	requireStatus(t, "cross-tenant read", status, 404, env)

	// Tenant-A JWT on tenant-B subdomain → 403 (tid mismatch).
	status, env = orgGet(t, slugB, tokA, "/api/v1/departments")
	requireStatus(t, "wrong subdomain JWT", status, 403, env)
}
