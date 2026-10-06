package integration

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------- platform (tenant) routes

func TestTenantRoutesRequirePlatformKey(t *testing.T) {
	expectErr(t, call(t, "GET", "/tenants"), 401, "UNAUTHORIZED", "no key")
	expectErr(t, call(t, "POST", "/tenants", body(map[string]string{"name": "x", "slug": "nokey"})), 401, "UNAUTHORIZED", "create without key")
	expectErr(t, call(t, "GET", "/tenants", header("X-Platform-Key", "wrong")), 403, "FORBIDDEN", "wrong key")
	expect(t, call(t, "GET", "/tenants", platform()), 200, "right key")
}

func TestTenantLifecycle(t *testing.T) {
	slug := "life" + uniq()
	c := call(t, "POST", "/tenants", platform(), body(map[string]any{"name": "Life", "slug": slug, "plan": "pro", "domain": "life.example.com"}))
	expect(t, c, 201, "create")
	id := c.str("data.id")
	if c.str("data.status") != "active" || c.str("data.plan") != "pro" || c.str("data.domain") != "life.example.com" {
		t.Fatalf("unexpected create body: %s", c.Raw)
	}

	g := call(t, "GET", "/tenants/"+id, platform())
	expect(t, g, 200, "get")
	if g.str("data.slug") != slug {
		t.Fatalf("slug mismatch: %s", g.Raw)
	}

	u := call(t, "PATCH", "/tenants/"+id, platform(), body(map[string]string{"name": "Renamed", "plan": "enterprise"}))
	expect(t, u, 200, "update")
	g = call(t, "GET", "/tenants/"+id, platform())
	if g.str("data.name") != "Renamed" || g.str("data.plan") != "enterprise" {
		t.Fatalf("update not persisted: %s", g.Raw)
	}

	l := call(t, "GET", "/tenants", platform(), query("per_page", "100"))
	if !l.has("data", "id", id) || l.get("meta.total_items") == nil {
		t.Fatalf("list lacks tenant / meta: %s", l.Raw)
	}

	expect(t, call(t, "POST", "/tenants/"+id+"/suspend", platform()), 200, "suspend")
	if call(t, "GET", "/tenants/"+id, platform()).str("data.status") != "suspended" {
		t.Fatal("not suspended")
	}
	expectErr(t, call(t, "GET", "/users", host(slug)), 403, "TENANT_SUSPENDED", "suspended host")
	expect(t, call(t, "POST", "/tenants/"+id+"/activate", platform()), 200, "activate")
	expectErr(t, call(t, "GET", "/users", host(slug)), 401, "UNAUTHORIZED", "active host reaches auth")

	expectErr(t, call(t, "GET", "/tenants/"+zeroUUID, platform()), 404, "NOT_FOUND", "unknown tenant")
	expectErr(t, call(t, "GET", "/tenants/not-a-uuid", platform()), 400, "VALIDATION_ERROR", "bad id")
	expectErr(t, call(t, "POST", "/tenants", platform(), body(map[string]string{"name": "dup", "slug": slug})), 409, "CONFLICT", "duplicate slug")
	expectErr(t, call(t, "POST", "/tenants", platform(), body(map[string]string{"name": "r", "slug": "admin"})), 409, "CONFLICT", "reserved slug")
	expectErr(t, call(t, "POST", "/tenants", platform(), body(map[string]string{"name": "r", "slug": "Bad_Slug!"})), 400, "VALIDATION_ERROR", "slug format")
	expectErr(t, call(t, "POST", "/tenants", platform(), body(map[string]string{})), 400, "VALIDATION_ERROR", "empty body")
}

func TestTenantAdminProvisioningIsAtomic(t *testing.T) {
	slug := "atom" + uniq()
	bad := map[string]any{"name": "Atomic", "slug": slug, "admin": map[string]any{"email": "a@x.test", "password": "short", "first_name": "A", "last_name": "B"}}
	expectErr(t, call(t, "POST", "/tenants", platform(), body(bad)), 400, "VALIDATION_ERROR", "weak admin password")
	good := map[string]any{"name": "Atomic", "slug": slug, "admin": map[string]any{"email": "a@" + slug + ".test", "password": password, "first_name": "A", "last_name": "B"}}
	r := call(t, "POST", "/tenants", platform(), body(good))
	expect(t, r, 201, "same slug succeeds after the failed attempt")
	if r.str("data.admin_user_id") == "" {
		t.Fatal("admin_user_id missing")
	}
	l := login(t, slug, "a@"+slug+".test", password)
	me := call(t, "GET", "/auth/me", token(l.str("data.access_token")))
	expect(t, me, 200, "me")
	if me.get("data.is_tenant_admin") != true {
		t.Fatalf("provisioned admin is not tenant_admin: %s", me.Raw)
	}
}

func TestPlatformRoutesDisabledWithoutKeyIsCoveredByMiddlewareUnit(t *testing.T) {
	// fail-closed behaviour (503 when no key is configured) is unit-tested in identity/middleware; here we only make
	// sure the configured key is not accepted as a tenant JWT.
	tc := newTenant(t)
	expectErr(t, call(t, "GET", "/tenants", token(tc.token)), 401, "UNAUTHORIZED", "a tenant JWT is not a platform key")
}

// ---------------------------------------------------------------- authentication

func TestAuthLoginErrors(t *testing.T) {
	tc := newTenant(t)
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail, "password": "nope"})), 401, "INVALID_CREDENTIALS", "wrong password")
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": "ghost" + uniq(), "email": "a@b.co", "password": "x"})), 401, "INVALID_CREDENTIALS", "unknown tenant is indistinguishable")
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": tc.slug, "email": "nobody@x.test", "password": "x"})), 401, "INVALID_CREDENTIALS", "unknown user is indistinguishable")
	r := call(t, "POST", "/auth/login", body(map[string]string{}))
	expectErr(t, r, 400, "VALIDATION_ERROR", "empty login")
	if len(r.list("error.details")) != 3 {
		t.Fatalf("want 3 field details, got %s", r.Raw)
	}
	expectErr(t, call(t, "POST", "/auth/login", rawBody("{bad")), 400, "VALIDATION_ERROR", "malformed json")
}

func TestLoginRateLimitCountsFailuresOnly(t *testing.T) {
	tc := newTenant(t)
	same := ip("203.0.113.9")
	for i := 0; i < 15; i++ { // successes never exhaust the budget
		expect(t, call(t, "POST", "/auth/login", same, body(map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail, "password": password})), 200, "success")
		if i == 3 {
			break // bcrypt is slow; four successes are enough to prove refunds
		}
	}
	bad := map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail, "password": "wrong"}
	for i := 0; i < 10; i++ {
		expect(t, call(t, "POST", "/auth/login", same, body(bad)), 401, "failure within budget")
	}
	r := call(t, "POST", "/auth/login", same, body(bad))
	expectErr(t, r, 429, "RATE_LIMITED", "11th failure")
	if r.Header.Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
	// the block is per IP: another client is unaffected
	expect(t, call(t, "POST", "/auth/login", ip("203.0.113.10"), body(map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail, "password": password})), 200, "other ip")
}

func TestRefreshRotationReuseAndLogout(t *testing.T) {
	tc := newTenant(t)
	rt1 := tc.refresh

	r2 := call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": rt1}))
	expect(t, r2, 200, "refresh")
	rt2, at2 := r2.str("data.refresh_token"), r2.str("data.access_token")
	if rt2 == "" || rt2 == rt1 || at2 == "" {
		t.Fatalf("tokens must rotate: %s", r2.Raw)
	}
	expect(t, call(t, "GET", "/users", host(tc.slug), token(at2)), 200, "new access token works")

	expectErr(t, call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": rt1})), 401, "TOKEN_REUSED", "reuse of a rotated token")
	// reuse detection killed every session of the user
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(at2)), 401, "SESSION_EXPIRED", "sessions revoked after reuse")
	expectErr(t, call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": rt2})), 401, "TOKEN_REUSED", "rt2 revoked too")
	expectErr(t, call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": "garbage"})), 401, "UNAUTHORIZED", "garbage")
	expectErr(t, call(t, "POST", "/auth/refresh", body(map[string]string{})), 400, "VALIDATION_ERROR", "empty")

	// logout
	l := login(t, tc.slug, tc.adminEmail, password)
	at, rt := l.str("data.access_token"), l.str("data.refresh_token")
	expect(t, call(t, "GET", "/users", host(tc.slug), token(at)), 200, "before logout")
	expect(t, call(t, "POST", "/auth/logout", token(at)), 200, "logout")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(at)), 401, "SESSION_EXPIRED", "access token after logout")
	expectErr(t, call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": rt})), 401, "UNAUTHORIZED", "refresh token after logout is a plain 401")
	// ...and it must NOT have killed the user's other sessions (logout is not a theft signal)
	other := login(t, tc.slug, tc.adminEmail, password)
	expect(t, call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": other.str("data.refresh_token")})), 200, "other session unaffected")
	expectErr(t, call(t, "POST", "/auth/logout"), 401, "UNAUTHORIZED", "logout needs a token")
}

func TestAccessTokenValidation(t *testing.T) {
	tc := newTenant(t)
	expectErr(t, call(t, "GET", "/users", host(tc.slug)), 401, "UNAUTHORIZED", "no header")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), header("Authorization", "Basic abc")), 401, "INVALID_TOKEN_FORMAT", "wrong scheme")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token("not.a.jwt")), 401, "INVALID_TOKEN", "garbage")
	expectErr(t, call(t, "GET", "/users", host("ghost"+uniq()), token(tc.token)), 404, "NOT_FOUND", "unknown tenant host")
	expectErr(t, call(t, "GET", "/users"), 400, "MISSING_SUBDOMAIN", "no subdomain")
	// token signed with another key
	forged := tc.token[:len(tc.token)-4] + "AAAA"
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(forged)), 401, "INVALID_TOKEN", "bad signature")
}

func TestForgotAndResetPassword(t *testing.T) {
	tc := newTenant(t)
	// generic answer, no user enumeration
	expect(t, call(t, "POST", "/auth/forgot-password", body(map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail})), 200, "known user")
	expect(t, call(t, "POST", "/auth/forgot-password", body(map[string]string{"tenant_slug": tc.slug, "email": "ghost@x.test"})), 200, "unknown user")
	expectErr(t, call(t, "POST", "/auth/forgot-password", body(map[string]string{"email": "bad"})), 400, "VALIDATION_ERROR", "invalid body")
	expectErr(t, call(t, "POST", "/auth/reset-password", body(map[string]string{"token": "nope", "new_password": "NewPassw0rd!", "confirm_password": "NewPassw0rd!"})), 400, "INVALID_TOKEN", "bad token")
	expectErr(t, call(t, "POST", "/auth/reset-password", body(map[string]string{"token": "x", "new_password": "NewPassw0rd!", "confirm_password": "different"})), 400, "VALIDATION_ERROR", "mismatch")
}

func TestAuthMe(t *testing.T) {
	tc := newTenant(t)
	r := call(t, "GET", "/auth/me", token(tc.token))
	expect(t, r, 200, "me")
	if r.str("data.user_id") != tc.adminID || r.get("data.is_tenant_admin") != true || !r.has("data.roles", "name", "tenant_admin") {
		t.Fatalf("unexpected me: %s", r.Raw)
	}
	lim, _, uid := tc.limitedUser(t, [2]string{"users", "read"}, [2]string{"roles", "read"})
	m := call(t, "GET", "/auth/me", token(lim))
	expect(t, m, 200, "limited me")
	perms := m.list("data.permissions")
	if m.get("data.is_tenant_admin") != false || len(perms) != 2 || m.str("data.user_id") != uid {
		t.Fatalf("limited me wrong: %s", m.Raw)
	}
	expectErr(t, call(t, "GET", "/auth/me"), 401, "UNAUTHORIZED", "needs token")
}

// ---------------------------------------------------------------- users

func TestUserLifecycle(t *testing.T) {
	tc := newTenant(t)
	member := tc.roleID(t, "member")
	id, email := tc.newUser(t, "u", member)

	g := tc.do(t, "GET", "/users/"+id)
	expect(t, g, 200, "get")
	if g.str("data.email") != email || !g.has("data.roles", "name", "member") || g.str("data.status") != "active" {
		t.Fatalf("read-back wrong (roles must be populated): %s", g.Raw)
	}

	expect(t, tc.do(t, "PATCH", "/users/"+id, body(map[string]string{"first_name": "Changed", "phone": "+911234567890", "avatar_url": "https://img.example.com/a.png"})), 200, "update")
	g = tc.do(t, "GET", "/users/"+id)
	if g.str("data.first_name") != "Changed" || g.str("data.phone") != "+911234567890" || !g.has("data.roles", "name", "member") {
		t.Fatalf("update not persisted: %s", g.Raw)
	}

	l := tc.do(t, "GET", "/users", query("per_page", "100"))
	if !l.has("data", "id", id) {
		t.Fatalf("list lacks user: %s", l.Raw)
	}
	// every listed user carries its roles
	for _, it := range l.list("data") {
		if it.(map[string]any)["roles"] == nil {
			t.Fatalf("list item without roles: %v", it)
		}
	}

	expectErr(t, tc.do(t, "POST", "/users", body(map[string]string{"email": email, "password": password, "first_name": "D", "last_name": "D"})), 409, "CONFLICT", "duplicate email")
	expectErr(t, tc.do(t, "POST", "/users", body(map[string]string{"email": "nope", "password": password, "first_name": "D", "last_name": "D"})), 400, "VALIDATION_ERROR", "bad email")
	expectErr(t, tc.do(t, "POST", "/users", body(map[string]string{"email": "x@y.test", "password": "short", "first_name": "D", "last_name": "D"})), 400, "VALIDATION_ERROR", "short password")
	expectErr(t, tc.do(t, "POST", "/users", body(map[string]any{"email": "z@y.test", "password": password, "first_name": "D", "last_name": "D", "role_ids": []string{zeroUUID}})), 404, "NOT_FOUND", "unknown role")
	expectErr(t, tc.do(t, "GET", "/users/"+zeroUUID), 404, "NOT_FOUND", "unknown user")
	expectErr(t, tc.do(t, "GET", "/users/not-a-uuid"), 400, "VALIDATION_ERROR", "bad id")
	expectErr(t, tc.do(t, "PATCH", "/users/"+id, body(map[string]string{"avatar_url": "not a url"})), 400, "VALIDATION_ERROR", "bad avatar")

	// duplicate role ids in one request are tolerated (de-duplicated)
	_, _ = tc.newUser(t, "dup", member, member)

	login(t, tc.slug, email, password)
	expect(t, tc.do(t, "POST", "/users/"+id+"/deactivate"), 200, "deactivate")
	if tc.do(t, "GET", "/users/"+id).str("data.status") != "inactive" {
		t.Fatal("not inactive")
	}
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": tc.slug, "email": email, "password": password})), 403, "ACCOUNT_NOT_ACTIVE", "inactive login")
	expectErr(t, tc.do(t, "POST", "/users/"+id+"/deactivate"), 409, "CONFLICT", "deactivate twice")

	act := tc.do(t, "POST", "/users/"+id+"/activate")
	expect(t, act, 200, "activate")
	if act.str("data.status") != "active" || !act.has("data.roles", "name", "member") {
		t.Fatalf("activate response wrong: %s", act.Raw)
	}
	login(t, tc.slug, email, password)
	expectErr(t, tc.do(t, "POST", "/users/"+id+"/activate"), 409, "CONFLICT", "activate twice")
	expectErr(t, tc.do(t, "POST", "/users/"+zeroUUID+"/activate"), 404, "NOT_FOUND", "activate unknown")
	expectErr(t, tc.do(t, "POST", "/users/"+tc.adminID+"/deactivate"), 409, "CONFLICT", "self deactivation")
}

func TestDeactivatedUserSessionsAreRevoked(t *testing.T) {
	tc := newTenant(t)
	id, email := tc.newUser(t, "rev", tc.roleID(t, "tenant_admin"))
	l := login(t, tc.slug, email, password)
	at, rt := l.str("data.access_token"), l.str("data.refresh_token")
	expect(t, tc.do(t, "POST", "/users/"+id+"/deactivate"), 200, "deactivate")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(at)), 401, "SESSION_EXPIRED", "access token dead")
	expectErr(t, call(t, "POST", "/auth/refresh", body(map[string]string{"refresh_token": rt})), 401, "", "refresh token dead")
}

func TestAssignAndRemoveRoles(t *testing.T) {
	tc := newTenant(t)
	rr := tc.do(t, "POST", "/roles", body(map[string]any{"name": "ar-" + uniq()}))
	expect(t, rr, 201, "role")
	rid := rr.str("data.id")
	uid, _ := tc.newUser(t, "ar")

	expect(t, tc.do(t, "POST", "/users/"+uid+"/roles", body(map[string]any{"role_ids": []string{rid}})), 200, "assign")
	expect(t, tc.do(t, "POST", "/users/"+uid+"/roles", body(map[string]any{"role_ids": []string{rid}})), 200, "assign is idempotent")
	if !tc.do(t, "GET", "/users/"+uid).has("data.roles", "id", rid) {
		t.Fatal("role not shown on user")
	}
	expectErr(t, tc.do(t, "DELETE", "/roles/"+rid), 409, "CONFLICT", "role in use cannot be deleted")
	expectErr(t, tc.do(t, "POST", "/users/"+uid+"/roles", body(map[string]any{"role_ids": []string{zeroUUID}})), 404, "NOT_FOUND", "unknown role")
	expectErr(t, tc.do(t, "POST", "/users/"+zeroUUID+"/roles", body(map[string]any{"role_ids": []string{rid}})), 404, "NOT_FOUND", "unknown user")
	expectErr(t, tc.do(t, "POST", "/users/"+uid+"/roles", body(map[string]any{"role_ids": []string{}})), 400, "VALIDATION_ERROR", "empty list")

	expect(t, tc.do(t, "DELETE", "/users/"+uid+"/roles/"+rid), 200, "unassign")
	if tc.do(t, "GET", "/users/"+uid).has("data.roles", "id", rid) {
		t.Fatal("role still shown")
	}
	expectErr(t, tc.do(t, "DELETE", "/users/"+uid+"/roles/"+rid), 404, "NOT_FOUND", "unassign twice")
	expectErr(t, tc.do(t, "DELETE", "/users/"+zeroUUID+"/roles/"+rid), 404, "NOT_FOUND", "unknown user")
	expectErr(t, tc.do(t, "DELETE", "/users/"+uid+"/roles/"+zeroUUID), 404, "NOT_FOUND", "unknown role")
	expectErr(t, tc.do(t, "DELETE", "/users/"+uid+"/roles/bad"), 400, "VALIDATION_ERROR", "bad role id")
	expect(t, tc.do(t, "DELETE", "/roles/"+rid), 200, "role now deletable")

	// lock-out guard: the last tenant_admin cannot lose the role
	admin := tc.roleID(t, "tenant_admin")
	expectErr(t, tc.do(t, "DELETE", "/users/"+tc.adminID+"/roles/"+admin), 409, "CONFLICT", "last tenant_admin")
	// ...but with a second admin the first one can step down
	id2, _ := tc.newUser(t, "adm2", admin)
	expect(t, tc.do(t, "DELETE", "/users/"+id2+"/roles/"+admin), 200, "second admin may be demoted")
}

// ---------------------------------------------------------------- roles & permissions

func TestRolesAndPermissions(t *testing.T) {
	tc := newTenant(t)
	pRead, pRoles := tc.permID(t, "users", "read"), tc.permID(t, "roles", "read")

	list := tc.do(t, "GET", "/permissions", query("per_page", "100"))
	expect(t, list, 200, "permissions")
	if len(list.list("data")) < 18 {
		t.Fatalf("permission catalogue not seeded: %d", len(list.list("data")))
	}

	c := tc.do(t, "POST", "/roles", body(map[string]any{"name": "r-" + uniq(), "description": "d", "permission_ids": []string{pRead, pRead}}))
	expect(t, c, 201, "create")
	rid := c.str("data.id")
	if len(c.list("data.permissions")) != 1 || c.get("data.is_system") != false {
		t.Fatalf("create response wrong (perms populated, de-duplicated): %s", c.Raw)
	}
	g := tc.do(t, "GET", "/roles/"+rid)
	if !g.has("data.permissions", "id", pRead) {
		t.Fatalf("get lacks permissions: %s", g.Raw)
	}

	expect(t, tc.do(t, "PATCH", "/roles/"+rid, body(map[string]string{"description": "updated"})), 200, "update")
	g = tc.do(t, "GET", "/roles/"+rid)
	if g.str("data.description") != "updated" || len(g.list("data.permissions")) != 1 {
		t.Fatalf("update wrong: %s", g.Raw)
	}

	expect(t, tc.do(t, "POST", "/roles/"+rid+"/permissions", body(map[string]any{"permission_ids": []string{pRoles}})), 200, "assign")
	expect(t, tc.do(t, "POST", "/roles/"+rid+"/permissions", body(map[string]any{"permission_ids": []string{pRoles}})), 200, "assign idempotent")
	g = tc.do(t, "GET", "/roles/"+rid)
	if len(g.list("data.permissions")) != 2 {
		t.Fatalf("want 2 perms: %s", g.Raw)
	}
	l := tc.do(t, "GET", "/roles", query("per_page", "100"))
	if !l.has("data", "id", rid) {
		t.Fatal("list lacks role")
	}
	for _, it := range l.list("data") {
		if m := it.(map[string]any); m["id"] == rid && len(m["permissions"].([]any)) != 2 {
			t.Fatalf("list item lacks permissions: %v", m)
		}
	}

	expectErr(t, tc.do(t, "POST", "/roles", body(map[string]string{"name": c.str("data.name")})), 409, "CONFLICT", "duplicate name")
	expect(t, tc.do(t, "PATCH", "/roles/"+rid, body(map[string]string{"name": "renamed-" + uniq()})), 200, "rename")
	other := tc.do(t, "POST", "/roles", body(map[string]string{"name": "other-" + uniq()}))
	expectErr(t, tc.do(t, "PATCH", "/roles/"+rid, body(map[string]string{"name": other.str("data.name")})), 409, "CONFLICT", "rename to existing")

	// validation: nothing is written for a bad request
	before := len(tc.do(t, "GET", "/roles", query("per_page", "100")).list("data"))
	expectErr(t, tc.do(t, "POST", "/roles", body(map[string]any{"name": "ghost-" + uniq(), "permission_ids": []string{zeroUUID}})), 404, "NOT_FOUND", "unknown permission")
	expectErr(t, tc.do(t, "POST", "/roles", body(map[string]any{"name": "bad", "permission_ids": []string{"abc"}})), 400, "VALIDATION_ERROR", "non-uuid permission")
	if after := len(tc.do(t, "GET", "/roles", query("per_page", "100")).list("data")); after != before {
		t.Fatalf("failed requests left %d partial role(s) behind", after-before)
	}
	expectErr(t, tc.do(t, "POST", "/roles/"+rid+"/permissions", body(map[string]any{"permission_ids": []string{zeroUUID}})), 404, "NOT_FOUND", "assign unknown")
	expectErr(t, tc.do(t, "POST", "/roles/"+zeroUUID+"/permissions", body(map[string]any{"permission_ids": []string{pRead}})), 404, "NOT_FOUND", "unknown role")

	// remove a permission
	expect(t, tc.do(t, "DELETE", "/roles/"+rid+"/permissions/"+pRoles), 200, "remove permission")
	expectErr(t, tc.do(t, "DELETE", "/roles/"+rid+"/permissions/"+pRoles), 404, "NOT_FOUND", "remove twice")
	expectErr(t, tc.do(t, "DELETE", "/roles/"+zeroUUID+"/permissions/"+pRoles), 404, "NOT_FOUND", "unknown role")
	expectErr(t, tc.do(t, "DELETE", "/roles/"+rid+"/permissions/bad"), 400, "VALIDATION_ERROR", "bad perm id")

	// system roles are protected
	admin, member := tc.roleID(t, "tenant_admin"), tc.roleID(t, "member")
	expectErr(t, tc.do(t, "DELETE", "/roles/"+admin), 403, "FORBIDDEN", "delete system role")
	expectErr(t, tc.do(t, "PATCH", "/roles/"+member, body(map[string]string{"name": "renamed"})), 403, "FORBIDDEN", "rename system role")
	expectErr(t, tc.do(t, "DELETE", "/roles/"+admin+"/permissions/"+pRead), 403, "FORBIDDEN", "tenant_admin permissions are fixed")

	expect(t, tc.do(t, "DELETE", "/roles/"+rid), 200, "delete")
	expectErr(t, tc.do(t, "GET", "/roles/"+rid), 404, "NOT_FOUND", "gone")
	expectErr(t, tc.do(t, "DELETE", "/roles/"+rid), 404, "NOT_FOUND", "delete twice")
	expectErr(t, tc.do(t, "GET", "/roles/bad"), 400, "VALIDATION_ERROR", "bad id")
}

// ---------------------------------------------------------------- RBAC enforcement

func TestRBACEnforcement(t *testing.T) {
	tc := newTenant(t)
	lim, rid, uid := tc.limitedUser(t, [2]string{"users", "read"})

	expect(t, call(t, "GET", "/users", host(tc.slug), token(lim)), 200, "users:read")
	expectErr(t, call(t, "POST", "/users", host(tc.slug), token(lim), body(map[string]string{"email": "n@x.test", "password": password, "first_name": "N", "last_name": "N"})), 403, "FORBIDDEN", "users:create")
	expectErr(t, call(t, "PATCH", "/users/"+uid, host(tc.slug), token(lim), body(map[string]string{"first_name": "X"})), 403, "FORBIDDEN", "users:update")
	expectErr(t, call(t, "POST", "/users/"+uid+"/deactivate", host(tc.slug), token(lim)), 403, "FORBIDDEN", "users:delete")
	expectErr(t, call(t, "POST", "/users/"+uid+"/activate", host(tc.slug), token(lim)), 403, "FORBIDDEN", "activate needs users:update")
	expectErr(t, call(t, "GET", "/roles", host(tc.slug), token(lim)), 403, "FORBIDDEN", "roles:read")
	expectErr(t, call(t, "GET", "/permissions", host(tc.slug), token(lim)), 403, "FORBIDDEN", "permissions:read")
	expectErr(t, call(t, "GET", "/audit-logs", host(tc.slug), token(lim)), 403, "FORBIDDEN", "audit:read")
	expectErr(t, call(t, "GET", "/departments", host(tc.slug), token(lim)), 403, "FORBIDDEN", "organization:read")
	expectErr(t, call(t, "GET", "/employees", host(tc.slug), token(lim)), 403, "FORBIDDEN", "employee:read")

	tc.grant(t, rid, "roles", "read")
	expect(t, call(t, "GET", "/roles", host(tc.slug), token(lim)), 200, "roles:read granted — effective immediately")
	tc.grant(t, rid, "audit", "read")
	expect(t, call(t, "GET", "/audit-logs", host(tc.slug), token(lim)), 200, "audit:read granted")

	// permission removal takes effect too
	expect(t, tc.do(t, "DELETE", "/roles/"+rid+"/permissions/"+tc.permID(t, "roles", "read")), 200, "revoke")
	expectErr(t, call(t, "GET", "/roles", host(tc.slug), token(lim)), 403, "FORBIDDEN", "revoked")

	// a user holding no role at all is denied everywhere
	mid, memberEmail := tc.newUser(t, "norole")
	_ = mid
	nt := login(t, tc.slug, memberEmail, password).str("data.access_token")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(nt)), 403, "FORBIDDEN", "no roles")
}

// ---------------------------------------------------------------- audit trail, pagination, envelopes

func TestAuditTrail(t *testing.T) {
	tc := newTenant(t)
	id, _ := tc.newUser(t, "aud")
	tc.do(t, "PATCH", "/users/"+id, body(map[string]string{"first_name": "A"}))
	tc.do(t, "POST", "/users/"+id+"/deactivate")
	r := tc.do(t, "GET", "/audit-logs", query("per_page", "100"))
	expect(t, r, 200, "audit")
	for _, want := range []string{"user.created", "user.updated", "user.deactivated"} {
		if !r.has("data", "action", want) {
			t.Fatalf("audit trail lacks %s: %s", want, r.Raw)
		}
	}
	// newest first
	l := r.list("data")
	first, last := l[0].(map[string]any)["created_at"].(string), l[len(l)-1].(map[string]any)["created_at"].(string)
	if first < last {
		t.Fatal("audit trail must be newest first")
	}
	// failed requests are not audited
	n := len(tc.do(t, "GET", "/audit-logs", query("per_page", "100")).list("data"))
	tc.do(t, "PATCH", "/users/"+zeroUUID, body(map[string]string{"first_name": "A"}))
	if m := len(tc.do(t, "GET", "/audit-logs", query("per_page", "100")).list("data")); m != n {
		t.Fatalf("failed request was audited (%d -> %d)", n, m)
	}
	// tenant isolation of the trail
	other := newTenant(t)
	if other.do(t, "GET", "/audit-logs", query("per_page", "100")).has("data", "resource_id", id) {
		t.Fatal("audit trail leaked across tenants")
	}
}

func TestPaginationIsClampedEverywhere(t *testing.T) {
	tc := newTenant(t)
	for _, path := range []string{"/users", "/roles", "/permissions", "/audit-logs", "/departments", "/teams", "/designations", "/employees"} {
		r := tc.do(t, "GET", path, query("per_page", "0"))
		expect(t, r, 200, path+" per_page=0")
		if r.get("meta.per_page") != float64(20) {
			t.Fatalf("%s per_page=0 -> %v", path, r.get("meta.per_page"))
		}
		r = tc.do(t, "GET", path, query("per_page", "1000"))
		if r.get("meta.per_page") != float64(100) {
			t.Fatalf("%s per_page=1000 -> %v", path, r.get("meta.per_page"))
		}
		r = tc.do(t, "GET", path, query("page", "-3", "per_page", "abc"))
		if r.Status != 200 || r.get("meta.page") != float64(1) {
			t.Fatalf("%s junk pagination: %s", path, r.Raw)
		}
		if r.get("meta.total_items") == nil || r.get("meta.total_pages") == nil {
			t.Fatalf("%s meta lacks total_items/total_pages: %s", path, r.Raw)
		}
	}
	expect(t, call(t, "GET", "/tenants", platform(), query("per_page", "0")), 200, "tenants per_page=0")
}

func TestErrorEnvelopeIsUniform(t *testing.T) {
	tc := newTenant(t)
	cases := []resp{
		tc.do(t, "GET", "/users/not-a-uuid"),
		tc.do(t, "POST", "/users", rawBody("{oops")),
		tc.do(t, "POST", "/users", body(map[string]string{})),
		tc.do(t, "GET", "/roles/"+zeroUUID),
		tc.do(t, "GET", "/departments/not-a-uuid"),
		tc.do(t, "POST", "/departments", rawBody("{oops")),
		tc.do(t, "GET", "/employees/not-a-uuid"),
		tc.do(t, "POST", "/employees", body(map[string]string{})),
		call(t, "POST", "/auth/logout"),
		call(t, "POST", "/auth/login", body(map[string]string{})),
		call(t, "POST", "/tenants", platform(), body(map[string]string{})),
	}
	for i, r := range cases {
		if _, isObj := r.get("error").(map[string]any); !isObj || r.errCode() == "" || r.str("error.message") == "" {
			t.Fatalf("case %d (%d) is not the standard error envelope: %s", i, r.Status, r.Raw)
		}
		if strings.Contains(r.Raw, "Key: '") {
			t.Fatalf("case %d leaks Go validator internals: %s", i, r.Raw)
		}
	}
}

func TestCORSHeaders(t *testing.T) {
	r := call(t, "GET", "/health", header("Origin", "https://app.example.com"))
	if r.Header.Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("allowed origin not echoed: %v", r.Header)
	}
	r = call(t, "GET", "/health", header("Origin", "https://evil.test"))
	if r.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unlisted origin must not be allowed")
	}
	r = call(t, "OPTIONS", "/users", header("Origin", "https://app.example.com"))
	if r.Status != 204 || !strings.Contains(r.Header.Get("Access-Control-Allow-Headers"), "X-Platform-Key") {
		t.Fatalf("preflight wrong: %d %v", r.Status, r.Header)
	}
}

// ---------------------------------------------------------------- tenant isolation

func TestTenantIsolation(t *testing.T) {
	a, b := newTenant(t), newTenant(t)

	// a token is only valid on its own tenant host
	expectErr(t, call(t, "GET", "/users", host(b.slug), token(a.token)), 403, "FORBIDDEN", "A token on B host")
	expectErr(t, call(t, "GET", "/departments", host(b.slug), token(a.token)), 403, "FORBIDDEN", "A token on B host (org)")
	expectErr(t, call(t, "GET", "/employees", host(b.slug), token(a.token)), 403, "FORBIDDEN", "A token on B host (employees)")

	// records of B are invisible to A even by id
	buID, _ := b.newUser(t, "iso")
	expectErr(t, a.do(t, "GET", "/users/"+buID), 404, "NOT_FOUND", "user by id")
	expectErr(t, a.do(t, "PATCH", "/users/"+buID, body(map[string]string{"first_name": "x"})), 404, "NOT_FOUND", "patch user")
	expectErr(t, a.do(t, "POST", "/users/"+buID+"/deactivate"), 404, "NOT_FOUND", "deactivate user")
	if a.do(t, "GET", "/users", query("per_page", "100")).has("data", "id", buID) {
		t.Fatal("B user leaked into A list")
	}
	brole := b.roleID(t, "member")
	expectErr(t, a.do(t, "GET", "/roles/"+brole), 404, "NOT_FOUND", "role by id")
	expectErr(t, a.do(t, "POST", "/users/"+a.adminID+"/roles", body(map[string]any{"role_ids": []string{brole}})), 404, "NOT_FOUND", "assign foreign role")

	bd := b.do(t, "POST", "/departments", body(map[string]any{"name": "B dept"}))
	expect(t, bd, 201, "B dept")
	bdID := bd.str("data.id")
	expectErr(t, a.do(t, "GET", "/departments/"+bdID), 404, "NOT_FOUND", "dept by id")
	expectErr(t, a.do(t, "PATCH", "/departments/"+bdID, body(map[string]string{"name": "x"})), 404, "NOT_FOUND", "patch dept")
	expectErr(t, a.do(t, "POST", "/departments/"+bdID+"/deactivate"), 404, "NOT_FOUND", "deactivate dept")
	if a.do(t, "GET", "/departments", query("per_page", "100")).has("data", "id", bdID) {
		t.Fatal("B department leaked into A list")
	}
	// a foreign department/user cannot be referenced from A's records
	expectErr(t, a.do(t, "POST", "/teams", body(map[string]string{"name": "t", "department_id": bdID})), 404, "NOT_FOUND", "team in foreign dept")
	expectErr(t, a.do(t, "POST", "/mappings", body(map[string]any{"user_id": buID, "department_id": a.mkDept(t, "iso-own")})), 404, "NOT_FOUND", "mapping for foreign user")
	// the same email can exist in two tenants (uniqueness is per tenant)
	email := "same@shared.test"
	expect(t, a.do(t, "POST", "/users", body(map[string]string{"email": email, "password": password, "first_name": "S", "last_name": "A"})), 201, "A same email")
	expect(t, b.do(t, "POST", "/users", body(map[string]string{"email": email, "password": password, "first_name": "S", "last_name": "B"})), 201, "B same email")
	// logging in against the other tenant fails
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": b.slug, "email": a.adminEmail, "password": password})), 401, "INVALID_CREDENTIALS", "A creds on B")
	// A's employee-facing data
	ae := a.mkEmployee(t, "iso")
	expectErr(t, b.do(t, "GET", "/employees/"+ae), 404, "NOT_FOUND", "employee by id")
	expectErr(t, b.do(t, "GET", "/employees/"+ae+"/statutory"), 404, "NOT_FOUND", "statutory by id")
	expectErr(t, b.do(t, "GET", "/employees/"+ae+"/timeline"), 404, "NOT_FOUND", "timeline by id")
}
