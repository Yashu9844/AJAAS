package api

import (
	"testing"
)

// TestOrgRBAC verifies reads require organization:read and writes require
// organization:update (G4). A member-role user has neither by default.
func TestOrgRBAC(t *testing.T) {
	slug := "e2erbac"
	bootstrapTenant(t, slug)
	seedE2EAdmin(t, slug)
	adminTok := adminToken(t, slug)

	// Create an org structure with the admin first.
	status, env := orgPost(t, slug, adminTok, "/api/v1/departments", map[string]string{"name": uniq(slug, "Engineering")})
	requireStatus(t, "admin create dept", status, 201, env)

	// Create a member user via identity API and log in (email unique per run —
	// soft-deleted users keep emails reserved).
	member := memberEmail(slug)
	status, env = doReq(t, "POST", "/api/v1/users", slug+".localhost", adminTok, map[string]string{
		"email": member, "password": "Secret123!", "first_name": "Member", "last_name": "User",
	})
	requireStatus(t, "create member", status, 201, env)

	status, memEnv := doReq(t, "POST", "/api/v1/auth/login", slug+".localhost", "", map[string]string{
		"email": member, "password": "Secret123!", "tenant_slug": slug,
	})
	requireStatus(t, "member login", status, 200, memEnv)
	memberTok := strField(t, memEnv.Data, "access_token")

	// Member read without organization:read → 403.
	status, env = orgGet(t, slug, memberTok, "/api/v1/departments")
	requireStatus(t, "member read denied", status, 403, env)

	// Member write without organization:update → 403.
	status, env = orgPost(t, slug, memberTok, "/api/v1/departments", map[string]string{"name": "Nope"})
	requireStatus(t, "member write denied", status, 403, env)

	// Admin (tenant_admin bypass) still succeeds → 201.
	status, env = orgPost(t, slug, adminTok, "/api/v1/departments", map[string]string{"name": uniq(slug, "AdminDept")})
	requireStatus(t, "admin write allowed", status, 201, env)
}
