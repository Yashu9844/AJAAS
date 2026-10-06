package integration

import (
	"fmt"
	"testing"
)

func (tc *tenantCtx) mkDept(t *testing.T, name string, parent ...string) string {
	t.Helper()
	b := map[string]any{"name": name + " " + uniq()}
	if len(parent) > 0 {
		b["parent_department_id"] = parent[0]
	}
	r := tc.do(t, "POST", "/departments", body(b))
	expect(t, r, 201, "create department "+name)
	return r.str("data.id")
}

func treeHas(r resp, id string) bool {
	var walk func(nodes []any) bool
	walk = func(nodes []any) bool {
		for _, n := range nodes {
			m := n.(map[string]any)
			if m["id"] == id {
				return true
			}
			if ch, ok := m["children"].([]any); ok && walk(ch) {
				return true
			}
		}
		return false
	}
	return walk(r.list("data"))
}

func TestDepartmentLifecycleAndHierarchy(t *testing.T) {
	tc := newTenant(t)
	code := "P" + uniq()
	p := tc.do(t, "POST", "/departments", body(map[string]any{"name": "Parent", "code": code, "description": "d"}))
	expect(t, p, 201, "create parent")
	pid := p.str("data.id")
	if p.get("data.depth") != float64(0) || p.str("data.status") != "active" || p.str("data.tenant_id") != tc.id {
		t.Fatalf("parent body wrong: %s", p.Raw)
	}
	c := tc.do(t, "POST", "/departments", body(map[string]any{"name": "Child", "parent_department_id": pid}))
	expect(t, c, 201, "create child")
	cid := c.str("data.id")
	g := tc.do(t, "GET", "/departments/"+cid)
	if g.get("data.depth") != float64(1) || g.str("data.parent_department_id") != pid || len(g.list("data.path")) != 1 || g.list("data.path")[0] != pid {
		t.Fatalf("child read-back wrong: %s", g.Raw)
	}

	expect(t, tc.do(t, "PATCH", "/departments/"+cid, body(map[string]string{"name": "Renamed", "description": "changed"})), 200, "update")
	g = tc.do(t, "GET", "/departments/"+cid)
	if g.str("data.name") != "Renamed" || g.str("data.description") != "changed" {
		t.Fatalf("update not persisted: %s", g.Raw)
	}
	l := tc.do(t, "GET", "/departments", query("per_page", "100"))
	if !l.has("data", "id", pid) || !l.has("data", "id", cid) {
		t.Fatal("list lacks departments")
	}

	// hierarchy protection
	expectErr(t, tc.do(t, "PATCH", "/departments/"+pid, body(map[string]string{"parent_department_id": cid})), 409, "CONFLICT", "cycle")
	expectErr(t, tc.do(t, "PATCH", "/departments/"+pid, body(map[string]string{"parent_department_id": pid})), 400, "VALIDATION_ERROR", "self parent")
	expectErr(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": "x", "parent_department_id": zeroUUID})), 404, "NOT_FOUND", "unknown parent")
	// re-parent child to a new root
	root2 := tc.mkDept(t, "Root2")
	expect(t, tc.do(t, "PATCH", "/departments/"+cid, body(map[string]string{"parent_department_id": root2})), 200, "reparent")
	if tc.do(t, "GET", "/departments/"+cid).str("data.parent_department_id") != root2 {
		t.Fatal("reparent not persisted")
	}

	// uniqueness & validation
	expectErr(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": "dup", "code": code})), 409, "CONFLICT", "duplicate code")
	expectErr(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": "Parent"})), 409, "CONFLICT", "duplicate name")
	expectErr(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": ""})), 400, "VALIDATION_ERROR", "empty name")
	expectErr(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": "n", "code": "A"})), 400, "VALIDATION_ERROR", "short code")
	expectErr(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": "n", "parent_department_id": "nope"})), 400, "VALIDATION_ERROR", "parent not uuid")
	expectErr(t, tc.do(t, "GET", "/departments/"+zeroUUID), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "PATCH", "/departments/"+zeroUUID, body(map[string]string{"name": "x"})), 404, "NOT_FOUND", "patch unknown")
	expectErr(t, tc.do(t, "POST", "/departments/"+zeroUUID+"/deactivate"), 404, "NOT_FOUND", "deactivate unknown")
	expectErr(t, tc.do(t, "GET", "/departments/not-a-uuid"), 400, "VALIDATION_ERROR", "bad id")

	// deactivation rules: blocked only by ACTIVE teams / mappings
	team := tc.do(t, "POST", "/teams", body(map[string]string{"name": "T", "department_id": pid}))
	expect(t, team, 201, "team")
	tid := team.str("data.id")
	expectErr(t, tc.do(t, "POST", "/departments/"+pid+"/deactivate"), 409, "CONFLICT", "active team blocks")
	expect(t, tc.do(t, "POST", "/teams/"+tid+"/deactivate"), 200, "deactivate team")
	expect(t, tc.do(t, "POST", "/departments/"+pid+"/deactivate", body(map[string]string{"reason": "done"})), 200, "inactive team no longer blocks")
	if tc.do(t, "GET", "/departments/"+pid).str("data.status") != "inactive" {
		t.Fatal("not inactive")
	}
	expectErr(t, tc.do(t, "POST", "/departments/"+pid+"/deactivate"), 409, "CONFLICT", "twice")
	expectErr(t, tc.do(t, "POST", "/departments/"+root2+"/deactivate", rawBody("{oops")), 400, "VALIDATION_ERROR", "bad body")

	// force cascades to children
	fp := tc.mkDept(t, "FP")
	fc := tc.mkDept(t, "FC", fp)
	ft := tc.do(t, "POST", "/teams", body(map[string]string{"name": "FT", "department_id": fp}))
	expect(t, ft, 201, "force team")
	expect(t, tc.do(t, "POST", "/departments/"+fp+"/deactivate", query("force", "true")), 200, "force")
	if tc.do(t, "GET", "/departments/"+fc).str("data.status") != "inactive" {
		t.Fatal("force must deactivate children")
	}
}

func TestDepartmentDepthLimit(t *testing.T) {
	tc := newTenant(t)
	parent := tc.mkDept(t, "L0")
	var last resp
	for i := 1; i <= 12; i++ {
		last = tc.do(t, "POST", "/departments", body(map[string]string{"name": fmt.Sprintf("L%d-%s", i, uniq()), "parent_department_id": parent}))
		if last.Status != 201 {
			expectErr(t, last, 422, "HIERARCHY_TOO_DEEP", "depth cap")
			return
		}
		parent = last.str("data.id")
	}
	t.Fatal("hierarchy depth was never capped")
}

func TestTeamLifecycle(t *testing.T) {
	tc := newTenant(t)
	did := tc.mkDept(t, "TD")
	uid, _ := tc.newUser(t, "lead")
	code := "T" + uniq()
	c := tc.do(t, "POST", "/teams", body(map[string]string{"name": "Team", "department_id": did, "code": code}))
	expect(t, c, 201, "create")
	tid := c.str("data.id")
	if c.str("data.department_id") != did || c.get("data.member_count") != float64(0) || c.str("data.status") != "active" {
		t.Fatalf("create body wrong: %s", c.Raw)
	}
	expect(t, tc.do(t, "PATCH", "/teams/"+tid, body(map[string]string{"name": "Renamed", "lead_user_id": uid})), 200, "update")
	g := tc.do(t, "GET", "/teams/"+tid)
	if g.str("data.name") != "Renamed" || g.str("data.lead_user_id") != uid {
		t.Fatalf("update not persisted: %s", g.Raw)
	}
	if !tc.do(t, "GET", "/teams", query("department_id", did)).has("data", "id", tid) {
		t.Fatal("filter by department failed")
	}
	other := tc.mkDept(t, "TD2")
	if tc.do(t, "GET", "/teams", query("department_id", other)).has("data", "id", tid) {
		t.Fatal("filter leaked")
	}
	// move to another department
	expect(t, tc.do(t, "PATCH", "/teams/"+tid, body(map[string]string{"department_id": other})), 200, "move")
	if tc.do(t, "GET", "/teams/"+tid).str("data.department_id") != other {
		t.Fatal("move not persisted")
	}

	expectErr(t, tc.do(t, "POST", "/teams", body(map[string]string{"name": "dup", "department_id": did, "code": code})), 409, "CONFLICT", "dup code")
	expectErr(t, tc.do(t, "POST", "/teams", body(map[string]string{"name": "x", "department_id": zeroUUID})), 404, "NOT_FOUND", "unknown dept")
	expectErr(t, tc.do(t, "POST", "/teams", body(map[string]string{"name": "x", "department_id": did, "lead_user_id": zeroUUID})), 404, "NOT_FOUND", "unknown lead")
	expectErr(t, tc.do(t, "POST", "/teams", body(map[string]string{"name": "x"})), 400, "VALIDATION_ERROR", "missing dept")
	expectErr(t, tc.do(t, "GET", "/teams", query("department_id", "bad")), 400, "VALIDATION_ERROR", "bad filter")
	expectErr(t, tc.do(t, "GET", "/teams/"+zeroUUID), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "PATCH", "/teams/"+zeroUUID, body(map[string]string{"name": "x"})), 404, "NOT_FOUND", "patch unknown")
	expectErr(t, tc.do(t, "POST", "/teams/"+zeroUUID+"/deactivate"), 404, "NOT_FOUND", "deactivate unknown")
	expectErr(t, tc.do(t, "GET", "/teams/bad"), 400, "VALIDATION_ERROR", "bad id")

	expect(t, tc.do(t, "POST", "/teams/"+tid+"/deactivate"), 200, "deactivate")
	if tc.do(t, "GET", "/teams/"+tid).str("data.status") != "inactive" {
		t.Fatal("not inactive")
	}
	expectErr(t, tc.do(t, "POST", "/teams/"+tid+"/deactivate"), 409, "CONFLICT", "twice")
}

func TestDesignationLifecycle(t *testing.T) {
	tc := newTenant(t)
	code := "D" + uniq()
	c := tc.do(t, "POST", "/designations", body(map[string]any{"title": "Engineer", "code": code, "level": 3, "description": "d"}))
	expect(t, c, 201, "create")
	id := c.str("data.id")
	if c.get("data.level") != float64(3) || c.str("data.status") != "active" {
		t.Fatalf("create body wrong: %s", c.Raw)
	}
	expect(t, tc.do(t, "PATCH", "/designations/"+id, body(map[string]any{"title": "Senior", "level": 5})), 200, "update")
	g := tc.do(t, "GET", "/designations/"+id)
	if g.str("data.title") != "Senior" || g.get("data.level") != float64(5) || g.str("data.code") != code {
		t.Fatalf("update not persisted: %s", g.Raw)
	}
	if !tc.do(t, "GET", "/designations", query("per_page", "100")).has("data", "id", id) {
		t.Fatal("list lacks designation")
	}
	expectErr(t, tc.do(t, "POST", "/designations", body(map[string]string{"title": "dup", "code": code})), 409, "CONFLICT", "dup code")
	expectErr(t, tc.do(t, "POST", "/designations", body(map[string]string{"title": "Senior"})), 409, "CONFLICT", "dup title")
	expectErr(t, tc.do(t, "POST", "/designations", body(map[string]any{"title": "bad", "level": 0})), 400, "VALIDATION_ERROR", "level 0")
	expectErr(t, tc.do(t, "POST", "/designations", body(map[string]string{})), 400, "VALIDATION_ERROR", "missing title")
	expectErr(t, tc.do(t, "GET", "/designations/"+zeroUUID), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "PATCH", "/designations/"+zeroUUID, body(map[string]string{"title": "x"})), 404, "NOT_FOUND", "patch unknown")
	expectErr(t, tc.do(t, "POST", "/designations/"+zeroUUID+"/deactivate"), 404, "NOT_FOUND", "deactivate unknown")
	expectErr(t, tc.do(t, "GET", "/designations/bad"), 400, "VALIDATION_ERROR", "bad id")

	// in use by an active mapping -> cannot deactivate
	uid, _ := tc.newUser(t, "dmap")
	m := tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": uid, "designation_id": id}))
	expect(t, m, 201, "mapping")
	expectErr(t, tc.do(t, "POST", "/designations/"+id+"/deactivate"), 409, "CONFLICT", "in use")
	expect(t, tc.do(t, "POST", "/mappings/"+m.str("data.id")+"/deactivate"), 200, "release")
	expect(t, tc.do(t, "POST", "/designations/"+id+"/deactivate"), 200, "deactivate")
	if tc.do(t, "GET", "/designations/"+id).str("data.status") != "inactive" {
		t.Fatal("not inactive")
	}
	expectErr(t, tc.do(t, "POST", "/designations/"+id+"/deactivate"), 409, "CONFLICT", "twice")
}

func TestMappingRulesAndUserDeactivationConvergence(t *testing.T) {
	tc := newTenant(t)
	d1, d2 := tc.mkDept(t, "M1"), tc.mkDept(t, "M2")
	team := tc.do(t, "POST", "/teams", body(map[string]string{"name": "MT", "department_id": d1}))
	tid := team.str("data.id")
	des := tc.do(t, "POST", "/designations", body(map[string]string{"title": "MD " + uniq()}))
	gid := des.str("data.id")
	u1, _ := tc.newUser(t, "m1")
	u2, _ := tc.newUser(t, "m2")

	m := tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u1, "department_id": d1, "team_id": tid, "designation_id": gid, "is_primary": true}))
	expect(t, m, 201, "create")
	mid := m.str("data.id")
	if m.get("data.is_primary") != true || m.str("data.status") != "active" || m.str("data.user_id") != u1 {
		t.Fatalf("create body wrong: %s", m.Raw)
	}
	g := tc.do(t, "GET", "/mappings/"+mid)
	if g.str("data.team_id") != tid || g.str("data.designation_id") != gid {
		t.Fatalf("read-back wrong: %s", g.Raw)
	}
	if !tc.do(t, "GET", "/mappings", query("user_id", u1)).has("data", "id", mid) {
		t.Fatal("list by user lacks mapping")
	}
	expect(t, tc.do(t, "PATCH", "/mappings/"+mid, body(map[string]string{"manager_user_id": u2})), 200, "set manager")
	if tc.do(t, "GET", "/mappings/"+mid).str("data.manager_user_id") != u2 {
		t.Fatal("manager not persisted")
	}

	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u1, "department_id": d1, "is_primary": true})), 409, "CONFLICT", "second primary")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2})), 400, "VALIDATION_ERROR", "no target")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "department_id": d2, "team_id": tid})), 400, "VALIDATION_ERROR", "team not in dept")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "department_id": d1, "manager_user_id": u2})), 400, "VALIDATION_ERROR", "self manager")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "department_id": d1, "manager_user_id": u1})), 409, "CONFLICT", "manager cycle")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": zeroUUID, "department_id": d1})), 404, "NOT_FOUND", "unknown user")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "department_id": zeroUUID})), 404, "NOT_FOUND", "unknown dept")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "team_id": zeroUUID})), 404, "NOT_FOUND", "unknown team")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "designation_id": zeroUUID})), 404, "NOT_FOUND", "unknown designation")
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "department_id": d1, "manager_user_id": zeroUUID})), 404, "NOT_FOUND", "unknown manager")
	expectErr(t, tc.do(t, "GET", "/mappings"), 400, "VALIDATION_ERROR", "user_id required")
	expectErr(t, tc.do(t, "GET", "/mappings", query("user_id", "bad")), 400, "VALIDATION_ERROR", "bad user_id")
	expectErr(t, tc.do(t, "GET", "/mappings/"+zeroUUID), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "PATCH", "/mappings/"+zeroUUID, body(map[string]string{"team_id": tid})), 404, "NOT_FOUND", "patch unknown")
	expectErr(t, tc.do(t, "POST", "/mappings/"+zeroUUID+"/deactivate"), 404, "NOT_FOUND", "deactivate unknown")
	expectErr(t, tc.do(t, "GET", "/mappings/bad"), 400, "VALIDATION_ERROR", "bad id")

	// FR-M005: deactivating the Module 0 user converges the mapping before the response returns
	expect(t, tc.do(t, "POST", "/users/"+u1+"/deactivate"), 200, "deactivate user")
	if tc.do(t, "GET", "/mappings/"+mid).str("data.status") != "inactive" {
		t.Fatal("mapping must converge when the user is deactivated")
	}
	// an inactive user cannot receive new mappings
	expectErr(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u1, "department_id": d2})), 404, "NOT_FOUND", "inactive user")

	m2 := tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": u2, "department_id": d1}))
	m2id := m2.str("data.id")
	expect(t, tc.do(t, "POST", "/mappings/"+m2id+"/deactivate"), 200, "deactivate without body")
	expectErr(t, tc.do(t, "POST", "/mappings/"+m2id+"/deactivate", body(map[string]string{"reason": "again"})), 409, "CONFLICT", "twice")
	if tc.do(t, "GET", "/mappings/"+m2id).str("data.status") != "inactive" {
		t.Fatal("not inactive")
	}
}

func TestOrgChartIsReadYourWritesConsistent(t *testing.T) {
	tc := newTenant(t)
	// warm every cache variant before writing
	warm := func() {
		for _, q := range [][]string{nil, {"max_depth", "1"}, {"max_depth", "3", "include_inactive", "true"}} {
			expect(t, tc.do(t, "GET", "/org-chart", query(q...)), 200, "warm")
		}
	}
	warm()
	p := tc.mkDept(t, "ChartP")
	if !treeHas(tc.do(t, "GET", "/org-chart"), p) || !treeHas(tc.do(t, "GET", "/org-chart", query("max_depth", "1")), p) {
		t.Fatal("new department missing from a previously cached chart (stale cache)")
	}
	c := tc.mkDept(t, "ChartC", p)
	if !treeHas(tc.do(t, "GET", "/org-chart"), c) {
		t.Fatal("child missing")
	}
	if treeHas(tc.do(t, "GET", "/org-chart", query("max_depth", "1")), c) {
		t.Fatal("max_depth=1 must omit nested child")
	}
	// team + mapping writes invalidate too
	team := tc.do(t, "POST", "/teams", body(map[string]string{"name": "CT", "department_id": p}))
	chart := tc.do(t, "GET", "/org-chart")
	found := false
	for _, n := range chart.list("data") {
		m := n.(map[string]any)
		if m["id"] == p {
			for _, tm := range m["teams"].([]any) {
				if tm.(map[string]any)["id"] == team.str("data.id") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("team missing from chart after create")
	}
	uid, _ := tc.newUser(t, "chart")
	tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": uid, "department_id": p, "is_primary": true}))
	for _, n := range tc.do(t, "GET", "/org-chart").list("data") {
		if m := n.(map[string]any); m["id"] == p && m["headcount"] != float64(1) {
			t.Fatalf("headcount not refreshed: %v", m["headcount"])
		}
	}
	// user deactivation (cross-module) invalidates as well
	tc.do(t, "POST", "/users/"+uid+"/deactivate")
	for _, n := range tc.do(t, "GET", "/org-chart").list("data") {
		if m := n.(map[string]any); m["id"] == p && m["headcount"] != float64(0) {
			t.Fatalf("headcount stale after user deactivation: %v", m["headcount"])
		}
	}
	// inactive departments are hidden unless asked for
	tc.do(t, "POST", "/departments/"+c+"/deactivate")
	if treeHas(tc.do(t, "GET", "/org-chart"), c) {
		t.Fatal("inactive department must be hidden by default")
	}
	if !treeHas(tc.do(t, "GET", "/org-chart", query("include_inactive", "true")), c) {
		t.Fatal("include_inactive=true must show it")
	}
}

func TestOrgChartUserChain(t *testing.T) {
	tc := newTenant(t)
	p := tc.mkDept(t, "CP")
	c := tc.mkDept(t, "CC", p)
	boss, _ := tc.newUser(t, "boss")
	emp, _ := tc.newUser(t, "emp")
	expect(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": boss, "department_id": p, "is_primary": true})), 201, "boss")
	expect(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": emp, "department_id": c, "is_primary": true, "manager_user_id": boss})), 201, "emp")
	r := tc.do(t, "GET", "/org-chart/chain", query("user_id", emp))
	expect(t, r, 200, "chain")
	if r.str("data.department.id") != c || !r.has("data.ancestors", "id", p) || r.str("data.mapping.manager_user_id") != boss {
		t.Fatalf("chain wrong: %s", r.Raw)
	}
	b := tc.do(t, "GET", "/org-chart/chain", query("user_id", boss))
	found := false
	for _, x := range b.list("data.direct_reports") {
		if x == emp {
			found = true
		}
	}
	if !found {
		t.Fatalf("direct_reports lacks employee: %s", b.Raw)
	}
	expectErr(t, tc.do(t, "GET", "/org-chart/chain"), 400, "VALIDATION_ERROR", "user_id required")
	nobody, _ := tc.newUser(t, "nomap")
	expect(t, tc.do(t, "GET", "/org-chart/chain", query("user_id", nobody)), 200, "unmapped user")
}

func TestOrgRBAC(t *testing.T) {
	tc := newTenant(t)
	lim, rid, _ := tc.limitedUser(t, [2]string{"organization", "read"})
	did := tc.mkDept(t, "RB")
	for _, path := range []string{"/departments", "/teams", "/designations", "/org-chart", "/departments/" + did} {
		expect(t, call(t, "GET", path, host(tc.slug), token(lim)), 200, "read "+path)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/departments"}, {"PATCH", "/departments/" + did}, {"POST", "/departments/" + did + "/deactivate"},
		{"POST", "/teams"}, {"POST", "/designations"}, {"POST", "/mappings"}, {"PATCH", "/mappings/" + zeroUUID}, {"POST", "/mappings/" + zeroUUID + "/deactivate"},
	} {
		expectErr(t, call(t, c.method, c.path, host(tc.slug), token(lim), body(map[string]string{"name": "x"})), 403, "FORBIDDEN", "write "+c.method+" "+c.path)
	}
	tc.grant(t, rid, "organization", "update")
	expect(t, call(t, "POST", "/departments", host(tc.slug), token(lim), body(map[string]string{"name": "allowed " + uniq()})), 201, "write after grant")
	expectErr(t, call(t, "GET", "/employees", host(tc.slug), token(lim)), 403, "FORBIDDEN", "org perms do not leak into employee")
}
