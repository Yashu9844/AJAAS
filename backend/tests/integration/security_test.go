package integration

import (
	"strings"
	"testing"
)

func TestSecurityHeadersAndBodyLimit(t *testing.T) {
	r := call(t, "GET", "/health")
	if r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("X-Frame-Options") != "DENY" || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("security headers missing: %v", r.Header)
	}
	big := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	expectErr(t, call(t, "POST", "/tenants", platform(), rawBody(big)), 413, "PAYLOAD_TOO_LARGE", "oversized body")
}

func TestInjectionAttemptsAreInert(t *testing.T) {
	tc := newTenant(t)
	evil := "x' OR '1'='1'; DROP TABLE users; --"
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": evil, "email": tc.adminEmail, "password": password})), 401, "INVALID_CREDENTIALS", "slug injection")
	expectErr(t, call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": tc.slug, "email": "a@b.co' OR 1=1 --", "password": evil})), 400, "VALIDATION_ERROR", "email must be a real email")
	expect(t, tc.do(t, "GET", "/employees", query("search", evil)), 200, "search injection")
	expect(t, tc.do(t, "GET", "/employees", query("status", evil)), 200, "status injection")
	// the users table is intact and the admin can still log in
	login(t, tc.slug, tc.adminEmail, password)
	// a hostile Host header cannot pick another tenant for an authenticated caller
	other := newTenant(t)
	expectErr(t, call(t, "GET", "/users", host(other.slug), token(tc.token)), 403, "FORBIDDEN", "token/host mismatch")
}

func TestPrivilegeEscalationIsBlocked(t *testing.T) {
	tc := newTenant(t)
	tok, _, uid := tc.limitedUser(t, [2]string{"users", "update"}, [2]string{"users", "read"}, [2]string{"users", "create"})
	as := func(method, path string, b any) resp {
		return call(t, method, path, host(tc.slug), token(tok), body(b))
	}
	adminRole := tc.roleID(t, "tenant_admin")

	// users:update must not be a path to tenant_admin
	expectErr(t, as("POST", "/users/"+uid+"/roles", map[string]any{"role_ids": []string{adminRole}}), 403, "FORBIDDEN", "self-assign tenant_admin")
	expectErr(t, as("POST", "/users", map[string]any{"email": "e@x.test", "password": password, "first_name": "E", "last_name": "V", "role_ids": []string{adminRole}}), 403, "FORBIDDEN", "create user with tenant_admin")
	if tc.do(t, "GET", "/users/"+uid).has("data.roles", "name", "tenant_admin") {
		t.Fatal("escalation succeeded")
	}

	// a role carrying a permission the caller lacks cannot be handed out
	strong := tc.do(t, "POST", "/roles", body(map[string]any{"name": "strong-" + uniq(), "permission_ids": []string{tc.permID(t, "users", "delete")}}))
	expect(t, strong, 201, "admin creates strong role")
	expectErr(t, as("POST", "/users/"+uid+"/roles", map[string]any{"role_ids": []string{strong.str("data.id")}}), 403, "FORBIDDEN", "role with foreign permission")
	expectErr(t, as("POST", "/users", map[string]any{"email": "e2@x.test", "password": password, "first_name": "E", "last_name": "V", "role_ids": []string{strong.str("data.id")}}), 403, "FORBIDDEN", "create user with strong role")

	// ...but a role within the caller's own permissions is fine (delegated administration still works)
	weak := tc.do(t, "POST", "/roles", body(map[string]any{"name": "weak-" + uniq(), "permission_ids": []string{tc.permID(t, "users", "read")}}))
	expect(t, weak, 201, "admin creates weak role")
	other, _ := tc.newUser(t, "deleg")
	expect(t, as("POST", "/users/"+other+"/roles", map[string]any{"role_ids": []string{weak.str("data.id")}}), 200, "assign role within own permissions")
	expect(t, as("POST", "/users", map[string]any{"email": "e3@x.test", "password": password, "first_name": "E", "last_name": "V", "role_ids": []string{weak.str("data.id")}}), 201, "create user with weak role")
	member := tc.roleID(t, "member")
	expect(t, as("POST", "/users/"+other+"/roles", map[string]any{"role_ids": []string{member}}), 200, "permission-less role is always assignable")

	// role administration cannot be used to mint permissions either
	rt, rrid, _ := tc.limitedUser(t, [2]string{"roles", "create"}, [2]string{"roles", "update"}, [2]string{"roles", "read"})
	ras := func(method, path string, b any) resp { return call(t, method, path, host(tc.slug), token(rt), body(b)) }
	expectErr(t, ras("POST", "/roles", map[string]any{"name": "mint-" + uniq(), "permission_ids": []string{tc.permID(t, "users", "delete")}}), 403, "FORBIDDEN", "create role with foreign permission")
	expectErr(t, ras("POST", "/roles/"+rrid+"/permissions", map[string]any{"permission_ids": []string{tc.permID(t, "users", "delete")}}), 403, "FORBIDDEN", "grant foreign permission to own role")
	expect(t, ras("POST", "/roles", map[string]any{"name": "ok-" + uniq(), "permission_ids": []string{tc.permID(t, "roles", "read")}}), 201, "create role within own permissions")
	expect(t, ras("POST", "/roles/"+rrid+"/permissions", map[string]any{"permission_ids": []string{tc.permID(t, "roles", "delete")}}), 403, "") // still not held

	// token claims cannot be forged by editing the payload (signature check)
	parts := strings.Split(tok, ".")
	forged := parts[0] + "." + parts[1] + "x." + parts[2]
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(forged)), 401, "INVALID_TOKEN", "forged payload")
}
