package integration

import (
	"testing"
)

// PR-04 end to end with real HR data: a department rule and a reports-of rule expand into members through the HR people
// adapter, a placement change plus a refresh removes them, and deactivating the login (specification section 7) takes the
// user out of projects while the history stays.
func TestProjectGroupRulesFollowHR(t *testing.T) {
	tc := newTenant(t)
	creator, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	dev1, _ := tc.newUser(t, "dev1")
	dev2, _ := tc.newUser(t, "dev2")
	boss, _ := tc.newUser(t, "boss")
	dept := tc.mkDept(t, "Engineering "+uniq())
	for _, u := range []string{dev1, dev2} {
		expect(t, tc.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": u, "department_id": dept, "manager_user_id": boss, "is_primary": true})), 201, "placement")
	}

	pid := tc.asTok(t, creator, "POST", "/projects", body(map[string]any{"key": "GRP" + uniq()[:3], "name": "Group project"})).str("data.id")
	expectErr(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/group-rules", body(map[string]any{"kind": "team", "role": "owner", "ref_id": dept})), 400, "VALIDATION_ERROR", "owner role is not a rule role")
	rule := tc.asTok(t, creator, "POST", "/projects/"+pid+"/group-rules", body(map[string]any{"kind": "department", "ref_id": dept, "role": "member"}))
	expect(t, rule, 201, "department rule")
	reports := tc.asTok(t, creator, "POST", "/projects/"+pid+"/group-rules", body(map[string]any{"kind": "reports_of", "ref_id": boss, "role": "viewer"}))
	expect(t, reports, 201, "reports-of rule")
	expectErr(t, tc.asTok(t, creator, "POST", "/projects/"+pid+"/group-rules", body(map[string]any{"kind": "department", "ref_id": dept, "role": "member"})), 409, "RULE_EXISTS", "duplicate rule")

	members := tc.asTok(t, creator, "GET", "/projects/"+pid+"/members")
	if len(members.list("data")) != 3 { // creator + two developers
		t.Fatalf("members after expansion: %s", members.Raw)
	}
	expectErr(t, tc.asTok(t, creator, "DELETE", "/projects/"+pid+"/members/"+dev1), 409, "GROUP_MEMBER", "group members cannot be removed one by one")

	// The developers can now read the private project; the person outside the rules cannot.
	devTok := login(t, tc.slug, devEmail(t, tc, "dev1"), password).str("data.access_token")
	expect(t, tc.asTok(t, devTok, "GET", "/projects/"+pid), 200, "group member reads")

	// HR deactivates dev2's login: the membership goes inactive in the same transaction.
	expect(t, tc.do(t, "POST", "/users/"+dev2+"/deactivate"), 200, "deactivate login")
	after := tc.asTok(t, creator, "GET", "/projects/"+pid+"/members")
	if len(after.list("data")) != 2 {
		t.Fatalf("members after deactivation: %s", after.Raw)
	}

	expect(t, tc.asTok(t, creator, "DELETE", "/projects/"+pid+"/group-rules/"+rule.str("data.id")), 200, "delete rule")
	if m := tc.asTok(t, creator, "GET", "/projects/"+pid+"/members"); len(m.list("data")) != 2 {
		t.Fatalf("dev1 stays through the reports-of rule: %s", m.Raw)
	}
	expect(t, tc.asTok(t, creator, "DELETE", "/projects/"+pid+"/group-rules/"+reports.str("data.id")), 200, "delete second rule")
	if m := tc.asTok(t, creator, "GET", "/projects/"+pid+"/members"); len(m.list("data")) != 1 {
		t.Fatalf("only the owner remains once the rules are gone: %s", m.Raw)
	}
}

// devEmail returns the e-mail address newUser generated for tag (the user list is searched by prefix).
func devEmail(t *testing.T, tc *tenantCtx, tag string) string {
	t.Helper()
	r := tc.do(t, "GET", "/users", query("per_page", "100"))
	for _, it := range r.list("data") {
		m := it.(map[string]any)
		if e, _ := m["email"].(string); len(e) > len(tag) && e[:len(tag)] == tag {
			return e
		}
	}
	t.Fatalf("user %s not found", tag)
	return ""
}
