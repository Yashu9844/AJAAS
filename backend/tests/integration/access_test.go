package integration

import (
	"strings"
	"testing"
	"time"
)

// Access model goldens (docs/ARCHITECTURE.md §17, M0 plan B0): predefined roles, scopes, inheritance, per-user
// grants and blocks, anti-escalation, automatic roles, isolation.

// rolesByKey maps template_key → role id for the tenant's predefined roles.
func (tc *tenantCtx) rolesByKey(t *testing.T) map[string]map[string]any {
	t.Helper()
	r := tc.do(t, "GET", "/roles", query("per_page", "100"))
	expect(t, r, 200, "list roles")
	out := map[string]map[string]any{}
	for _, it := range r.list("data") {
		m := it.(map[string]any)
		if k, ok := m["template_key"].(string); ok {
			out[k] = m
		}
	}
	return out
}

// tokenFor creates a user holding roleIDs and logs them in.
func (tc *tenantCtx) tokenFor(t *testing.T, tag string, roleIDs ...string) (tok, userID string) {
	t.Helper()
	userID, email := tc.newUser(t, tag, roleIDs...)
	return login(t, tc.slug, email, password).str("data.access_token"), userID
}

func scopedRole(t *testing.T, tc *tenantCtx, grants map[string]string, parent string) string {
	t.Helper()
	var gs []map[string]string
	for perm, scope := range grants {
		p := strings.SplitN(perm, ":", 2)
		gs = append(gs, map[string]string{"permission_id": tc.permID(t, p[0], p[1]), "scope": scope})
	}
	b := map[string]any{"name": "r-" + uniq(), "grants": gs}
	if parent != "" {
		b["parent_role_id"] = parent
	}
	r := tc.do(t, "POST", "/roles", body(b))
	expect(t, r, 201, "create scoped role")
	return r.str("data.id")
}

func TestAccessPredefinedRoles(t *testing.T) {
	tc := newTenant(t)
	roles := tc.rolesByKey(t)
	for _, k := range []string{"employee", "manager", "hr_manager", "hr_admin", "executive", "payroll_admin", "payroll_approver",
		"finance", "recruiter", "interviewer", "it_admin", "auditor"} {
		if roles[k] == nil {
			t.Fatalf("predefined role %s missing", k)
		}
	}
	chain := [][2]string{{"executive", "hr_admin"}, {"hr_admin", "hr_manager"}, {"hr_manager", "manager"}, {"manager", "employee"}}
	for _, c := range chain {
		if roles[c[0]]["parent_role_id"] != roles[c[1]]["id"] {
			t.Fatalf("%s must inherit %s", c[0], c[1])
		}
	}
	if roles["employee"]["is_automatic"] != true || roles["manager"]["is_automatic"] != true || roles["hr_admin"]["is_automatic"] != false {
		t.Fatal("employee and manager are automatic")
	}
	mgr := tc.do(t, "GET", "/roles/"+roles["manager"]["id"].(string))
	if !mgr.has("data.permissions", "scope", "direct_reports") {
		t.Fatalf("manager defaults carry team scopes: %s", mgr.Raw)
	}
	// predefined roles cannot be deleted; an edit takes the role out of template management; clone works
	expectErr(t, tc.do(t, "DELETE", "/roles/"+roles["hr_admin"]["id"].(string)), 403, "FORBIDDEN", "delete predefined role")
	expect(t, tc.do(t, "POST", "/roles/"+roles["auditor"]["id"].(string)+"/permissions",
		body(map[string]any{"grants": []map[string]string{{"permission_id": tc.permID(t, "users", "read"), "scope": "own"}}})), 200, "edit auditor")
	if tc.do(t, "GET", "/roles/"+roles["auditor"]["id"].(string)).get("data.template_managed") != false {
		t.Fatal("edited predefined role is no longer template-managed")
	}
	cl := tc.do(t, "POST", "/roles/"+roles["hr_manager"]["id"].(string)+"/clone", body(map[string]string{"name": "Regional HR"}))
	expect(t, cl, 201, "clone role")
	if cl.get("data.template_key") != nil || cl.get("data.parent_role_id") != roles["manager"]["id"] {
		t.Fatalf("clone copies grants and parent, not the template key: %s", cl.Raw)
	}
}

func TestAccessScopesInheritanceAndCycles(t *testing.T) {
	tc := newTenant(t)
	own := scopedRole(t, tc, map[string]string{"users:read": "own"}, "")
	tok, _ := tc.tokenFor(t, "own", own)
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 403, "FORBIDDEN", "own scope never passes a tenant-wide route")

	parent := scopedRole(t, tc, map[string]string{"users:read": "tenant"}, "")
	child := scopedRole(t, tc, map[string]string{"roles:read": "tenant"}, parent)
	ctok, cid := tc.tokenFor(t, "child", child)
	expect(t, call(t, "GET", "/users", host(tc.slug), token(ctok)), 200, "inherited users:read")
	ex := tc.do(t, "GET", "/users/"+cid+"/access")
	expect(t, ex, 200, "explain")
	if !ex.has("data.permissions", "permission", "users:read") || !strings.Contains(ex.Raw, `"kind":"inherited"`) {
		t.Fatalf("explain shows the inherited source: %s", ex.Raw)
	}
	// making the parent inherit from its child is a cycle
	expectErr(t, tc.do(t, "PATCH", "/roles/"+parent, body(map[string]string{"parent_role_id": child})), 409, "CONFLICT", "cycle")
	// a parent other roles inherit from cannot be deleted
	expectErr(t, tc.do(t, "DELETE", "/roles/"+parent), 409, "CONFLICT", "delete parent")
}

func TestAccessUserGrantsAndBlocks(t *testing.T) {
	tc := newTenant(t)
	tok, uid := tc.tokenFor(t, "plain")
	users := tc.permID(t, "users", "read")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 403, "FORBIDDEN", "no access yet")

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	expectErr(t, tc.do(t, "POST", "/users/"+uid+"/grants", body(map[string]any{"permission_id": users, "expires_at": past})), 400, "VALIDATION_ERROR", "expired grant")
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	g := tc.do(t, "POST", "/users/"+uid+"/grants", body(map[string]any{"permission_id": users, "expires_at": future, "reason": "cover"}))
	expect(t, g, 201, "grant")
	expect(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 200, "granted access applies immediately")
	expect(t, tc.do(t, "DELETE", "/users/"+uid+"/grants/"+users), 200, "revoke")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 403, "FORBIDDEN", "revoked access is gone immediately")

	// a block beats a role
	readers := scopedRole(t, tc, map[string]string{"users:read": "tenant"}, "")
	expect(t, tc.do(t, "POST", "/users/"+uid+"/roles", body(map[string]any{"role_ids": []string{readers}})), 200, "assign role")
	expect(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 200, "role access")
	expect(t, tc.do(t, "POST", "/users/"+uid+"/blocks", body(map[string]any{"permission_id": users, "reason": "audit"})), 201, "block")
	expectErr(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 403, "FORBIDDEN", "blocked")
	if ex := tc.do(t, "GET", "/users/"+uid+"/access"); !ex.has("data.permissions", "blocked", "true") {
		t.Fatalf("explain shows the block: %s", ex.Raw)
	}
	expect(t, tc.do(t, "DELETE", "/users/"+uid+"/blocks/"+users), 200, "unblock")
	expect(t, call(t, "GET", "/users", host(tc.slug), token(tok)), 200, "unblocked")
	expectErr(t, tc.do(t, "POST", "/users/"+tc.adminID+"/blocks", body(map[string]any{"permission_id": users})), 409, "CONFLICT", "tenant_admin cannot be blocked")
	expect(t, call(t, "GET", "/auth/me/access", host(tc.slug), token(tok)), 200, "explain self")
}

func TestAccessAntiEscalation(t *testing.T) {
	tc := newTenant(t)
	// an access manager holding users:read only at own scope
	mgr := scopedRole(t, tc, map[string]string{"access:manage": "tenant", "access:read": "tenant", "roles:create": "tenant", "users:read": "own"}, "")
	tok, _ := tc.tokenFor(t, "am", mgr)
	uid, _ := tc.newUser(t, "victim")
	as := func(method, path string, b any) resp { return call(t, method, path, host(tc.slug), token(tok), body(b)) }
	expectErr(t, as("POST", "/users/"+uid+"/grants", map[string]any{"permission_id": tc.permID(t, "users", "read"), "scope": "tenant"}), 403, "FORBIDDEN", "wider than held")
	expect(t, as("POST", "/users/"+uid+"/grants", map[string]any{"permission_id": tc.permID(t, "users", "read"), "scope": "own"}), 201, "same scope")
	expectErr(t, as("POST", "/users/"+uid+"/grants", map[string]any{"permission_id": tc.permID(t, "payroll", "approve")}), 403, "FORBIDDEN", "not held")
	expectErr(t, as("POST", "/roles", map[string]any{"name": "x" + uniq(), "parent_role_id": tc.rolesByKey(t)["hr_admin"]["id"]}), 403, "FORBIDDEN", "inherit more than held")
}

func TestAccessAutomaticRoles(t *testing.T) {
	tc := newTenant(t)
	roles := tc.rolesByKey(t)
	mgrID, mgrEmail := tc.newUser(t, "boss")
	repID, _ := tc.newUser(t, "rep")
	expect(t, tc.do(t, "POST", "/employees", body(empBody(mgrID, "B-"+strings.ToUpper(uniq()), nil))), 201, "boss profile")
	expect(t, tc.do(t, "POST", "/employees", body(empBody(repID, "R-"+strings.ToUpper(uniq()), nil))), 201, "rep profile")
	d := tc.do(t, "POST", "/departments", body(map[string]any{"name": "Sales " + uniq()}))
	expect(t, d, 201, "department")
	m := tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": repID, "department_id": d.str("data.id"), "is_primary": true, "manager_user_id": mgrID}))
	expect(t, m, 201, "reporting line")

	// login syncs the user's automatic roles
	tok := login(t, tc.slug, mgrEmail, password).str("data.access_token")
	me := call(t, "GET", "/auth/me", host(tc.slug), token(tok))
	if !me.has("data.roles", "name", "Manager") || !me.has("data.roles", "name", "Employee") || !me.has("data.roles", "source", "automatic") {
		t.Fatalf("boss holds automatic Manager and Employee: %s", me.Raw)
	}
	// automatic roles cannot be assigned or removed by hand
	expectErr(t, tc.do(t, "POST", "/users/"+repID+"/roles", body(map[string]any{"role_ids": []string{roles["manager"]["id"].(string)}})), 409, "ROLE_AUTOMATIC", "manual assign")
	expectErr(t, tc.do(t, "DELETE", "/users/"+mgrID+"/roles/"+roles["manager"]["id"].(string)), 409, "ROLE_AUTOMATIC", "manual remove")

	// the reporting line ends → the tenant sync removes Manager, keeps Employee
	expect(t, tc.do(t, "POST", "/mappings/"+m.str("data.id")+"/deactivate"), 200, "end reporting line")
	s := tc.do(t, "POST", "/access/automatic-roles/sync")
	expect(t, s, 200, "sync")
	if s.get("data.removed").(float64) < 1 || s.get("data.added").(float64) < 1 {
		t.Fatalf("sync adds rep's Employee and removes boss's Manager: %s", s.Raw)
	}
	ex := tc.do(t, "GET", "/users/"+mgrID+"/access")
	if strings.Contains(ex.Raw, `"role_name":"Manager"`) || !strings.Contains(ex.Raw, `"role_name":"Employee"`) {
		t.Fatalf("after sync the boss is an Employee only: %s", ex.Raw)
	}
}

func TestAccessTenantIsolation(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	uid, _ := a.newUser(t, "a")
	expectErr(t, b.do(t, "GET", "/users/"+uid+"/access"), 404, "NOT_FOUND", "foreign user explain")
	expectErr(t, b.do(t, "POST", "/users/"+uid+"/grants", body(map[string]any{"permission_id": b.permID(t, "users", "read")})), 404, "NOT_FOUND", "foreign grant")
	expectErr(t, call(t, "GET", "/users/"+uid+"/access", host(a.slug), token(b.token)), 403, "", "foreign token")
}
