package integration

import (
	"strings"
	"testing"
)

// Golden tests that are not already pinned by the feature suites. Module goldens are documented in
// docs/modules/*/golden-tests.md and mapped to tests in docs/dev-test/GOLDEN_MAP.md. Never edit these to make red pass.

// M1-G8: moving a team across departments keeps its mappings consistent and is audited.
func TestGoldenM1_TeamMoveKeepsMappingsConsistent(t *testing.T) {
	tc := newTenant(t)
	d1, d2 := tc.mkDept(t, "G8a"), tc.mkDept(t, "G8b")
	team := tc.do(t, "POST", "/teams", body(map[string]string{"name": "Mover", "department_id": d1}))
	expect(t, team, 201, "team")
	tid := team.str("data.id")
	uid, _ := tc.newUser(t, "mv")
	m := tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": uid, "department_id": d1, "team_id": tid, "is_primary": true}))
	expect(t, m, 201, "mapping")
	mid := m.str("data.id")

	expect(t, tc.do(t, "PATCH", "/teams/"+tid, body(map[string]string{"department_id": d2})), 200, "move team")
	if tc.do(t, "GET", "/teams/"+tid).str("data.department_id") != d2 {
		t.Fatal("team did not move")
	}
	g := tc.do(t, "GET", "/mappings/"+mid)
	if g.str("data.team_id") != tid || g.str("data.department_id") != d2 {
		t.Fatalf("mapping must follow its team to the new department (no mixed dept/team): %s", g.Raw)
	}
	if !tc.do(t, "GET", "/audit-logs", query("per_page", "100")).has("data", "action", "team.updated") {
		t.Fatal("team move not audited")
	}
}

// M1-G9: every org state change leaves an audit row.
func TestGoldenM1_AuditOnEveryOrgWrite(t *testing.T) {
	tc := newTenant(t)
	d := tc.mkDept(t, "Aud")
	tc.do(t, "PATCH", "/departments/"+d, body(map[string]string{"description": "x"}))
	team := tc.do(t, "POST", "/teams", body(map[string]string{"name": "AT", "department_id": d}))
	tid := team.str("data.id")
	tc.do(t, "PATCH", "/teams/"+tid, body(map[string]string{"description": "x"}))
	des := tc.do(t, "POST", "/designations", body(map[string]string{"title": "AD " + uniq()}))
	gid := des.str("data.id")
	tc.do(t, "PATCH", "/designations/"+gid, body(map[string]string{"description": "x"}))
	uid, _ := tc.newUser(t, "aud")
	m := tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": uid, "department_id": d}))
	tc.do(t, "PATCH", "/mappings/"+m.str("data.id"), body(map[string]any{"is_primary": true}))
	tc.do(t, "POST", "/mappings/"+m.str("data.id")+"/deactivate")
	tc.do(t, "POST", "/designations/"+gid+"/deactivate")
	tc.do(t, "POST", "/teams/"+tid+"/deactivate")
	tc.do(t, "POST", "/departments/"+d+"/deactivate")

	logs := tc.do(t, "GET", "/audit-logs", query("per_page", "100"))
	for _, want := range []string{
		"department.created", "department.updated", "department.deactivated",
		"team.created", "team.updated", "team.deactivated",
		"designation.created", "designation.updated", "designation.deactivated",
		"mapping.created", "mapping.updated", "mapping.deactivated",
	} {
		if !logs.has("data", "action", want) {
			t.Errorf("no audit row for %s", want)
		}
	}
	for _, it := range logs.list("data") {
		raw := strings.ToLower(it.(map[string]any)["action"].(string))
		if strings.Contains(raw, "password") && strings.Contains(raw, "hash") {
			t.Fatal("audit must not carry secrets")
		}
	}
}

// M0-G / M1-G10 / M2: no secret or hash ever appears in any payload.
func TestGoldenNoSecretLeakage(t *testing.T) {
	tc := newTenant(t)
	uid, email := tc.newUser(t, "leak")
	eid := tc.mkEmployee(t, "leak")
	tc.do(t, "PUT", "/employees/"+eid+"/statutory", body(map[string]string{"tax_id": "ABCDE1234F", "bank_account_number": "123456789012", "bank_name": "B"}))
	l := login(t, tc.slug, email, password)
	forbidden := []string{"password_hash", "passwordhash", "token_hash", "tokenhash", "$2a$", "$2b$", "jwt_secret", "refresh_tokens", "mfa_secret"}
	payloads := []resp{
		l,
		tc.do(t, "GET", "/users", query("per_page", "100")),
		tc.do(t, "GET", "/users/"+uid),
		tc.do(t, "GET", "/roles"),
		tc.do(t, "GET", "/permissions"),
		tc.do(t, "GET", "/audit-logs", query("per_page", "100")),
		tc.do(t, "GET", "/employees", query("per_page", "100")),
		tc.do(t, "GET", "/employees/"+eid),
		tc.do(t, "GET", "/employees/"+eid+"/statutory"),
		call(t, "GET", "/tenants", platform()),
		call(t, "GET", "/auth/me", token(tc.token)),
		tc.do(t, "GET", "/departments"),
		tc.do(t, "GET", "/org-chart"),
	}
	for i, p := range payloads {
		low := strings.ToLower(p.Raw)
		for _, f := range forbidden {
			if strings.Contains(low, f) {
				t.Fatalf("payload %d leaks %q: %.200s", i, f, p.Raw)
			}
		}
	}
	// masked statutory data never carries the raw values either
	if strings.Contains(payloads[8].Raw, "ABCDE1234F") || strings.Contains(payloads[8].Raw, "123456789012") {
		t.Fatal("raw PII in masked statutory response")
	}
}

// M2-G4: a user id from ANOTHER tenant is rejected as not found when creating an employee.
func TestGoldenM2_EmployeeForForeignTenantUser(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	foreign, _ := b.newUser(t, "foreign")
	expectErr(t, a.do(t, "POST", "/employees", body(empBody(foreign, "G4-"+uniq(), nil))), 404, "NOT_FOUND", "user from another tenant")
}

// M2-G2: onboarding is atomic — profile, employment, contact and timeline all exist after a single 201.
func TestGoldenM2_AtomicOnboarding(t *testing.T) {
	tc := newTenant(t)
	id := tc.mkEmployee(t, "atomic", map[string]any{"personal_email": "a@b.co", "work_phone": "+91"})
	g := tc.do(t, "GET", "/employees/"+id)
	if g.get("data.employment") == nil || g.get("data.contact") == nil || g.str("data.employment.employment_type") == "" {
		t.Fatalf("profile/employment/contact must all exist: %s", g.Raw)
	}
	if !timelineHas(tc.do(t, "GET", "/employees/"+id+"/timeline"), "hired") {
		t.Fatal("timeline must exist")
	}
	// a failed onboarding leaves nothing behind
	uid, _ := tc.newUser(t, "atomic2")
	before := tc.do(t, "GET", "/employees", query("per_page", "100"))
	expectErr(t, tc.do(t, "POST", "/employees", body(empBody(uid, "bad code!", nil))), 400, "VALIDATION_ERROR", "invalid")
	after := tc.do(t, "GET", "/employees", query("per_page", "100"))
	if len(before.list("data")) != len(after.list("data")) {
		t.Fatal("failed onboarding must not leave a partial profile")
	}
	// the user is still free to be onboarded afterwards
	expect(t, tc.do(t, "POST", "/employees", body(empBody(uid, "OK-"+uniq(), nil))), 201, "retry succeeds")
}

// M1-G2: rejected hierarchy changes leave no partial write.
func TestGoldenM1_RejectedHierarchyChangeLeavesNoTrace(t *testing.T) {
	tc := newTenant(t)
	p := tc.mkDept(t, "G2p")
	c := tc.mkDept(t, "G2c", p)
	beforeP := tc.do(t, "GET", "/departments/"+p).Raw
	beforeC := tc.do(t, "GET", "/departments/"+c).Raw
	expectErr(t, tc.do(t, "PATCH", "/departments/"+p, body(map[string]string{"parent_department_id": c, "name": "Hijacked"})), 409, "CONFLICT", "cycle")
	expectErr(t, tc.do(t, "PATCH", "/departments/"+p, body(map[string]string{"parent_department_id": p, "name": "Hijacked"})), 400, "VALIDATION_ERROR", "self parent")
	if tc.do(t, "GET", "/departments/"+p).Raw != beforeP || tc.do(t, "GET", "/departments/"+c).Raw != beforeC {
		t.Fatal("a rejected hierarchy change modified data (partial write)")
	}
}
