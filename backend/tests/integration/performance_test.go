package integration

import (
	"strings"
	"testing"
)

// perfPerson is an employee with a login for the performance goldens.
type perfPerson struct{ user, emp, tok string }

func (tc *timeCtx) perfPerson(t *testing.T, tag string) perfPerson {
	t.Helper()
	uid, email := tc.newUser(t, tag)
	r := tc.do(t, "POST", "/hr/employees", body(empBody(uid, strings.ToUpper(tag)+"-"+strings.ToUpper(uniq()), nil)))
	expect(t, r, 201, tag+" employee profile")
	return perfPerson{user: uid, emp: r.str("data.id"), tok: login(t, tc.slug, email, password).str("data.access_token")}
}

func ratings(scores ...int) []map[string]any {
	names := []string{"Quality", "Delivery", "Collaboration", "Ownership"}
	out := make([]map[string]any, len(names))
	for i, n := range names {
		out[i] = map[string]any{"criterion": n, "score": scores[i]}
	}
	return out
}

// TestPerformanceLifecycle — G1, G4–G6, G8–G14 end to end: the member (alice) reports to mgr through HR/Organization;
// bob is a colleague and peer reviewer; the tenant admin runs the cycle.
func TestPerformanceLifecycle(t *testing.T) {
	tc := newTimeTenant(t)
	alice := perfPerson{user: tc.memberUser, emp: tc.memberEmp, tok: tc.memberTok}
	mgr, bob := tc.perfPerson(t, "mgr"), tc.perfPerson(t, "bob")
	expect(t, tc.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": alice.user, "department_id": tc.mkDept(t, "Perf"),
		"is_primary": true, "manager_user_id": mgr.user})), 201, "alice reports to mgr")

	// G13 RBAC + D7-06: anyone reads cycles; only performance:manage runs them.
	cb := body(map[string]any{"title": "FY26 H1", "start_date": "2026-01-01", "end_date": "2026-06-30"})
	expectErr(t, tc.as(t, alice.tok, "POST", "/hr/performance/cycles", cb), 403, "FORBIDDEN", "G13 member creates cycle")
	cy := tc.do(t, "POST", "/hr/performance/cycles", cb)
	expect(t, cy, 201, "create cycle")
	cid := cy.str("data.id")
	if n := len(cy.list("data.criteria")); n != 4 {
		t.Fatalf("default criteria: %s", cy.Raw)
	}
	expect(t, tc.as(t, alice.tok, "GET", "/hr/performance/cycles/"+cid), 200, "D7-06 member reads cycle")
	expect(t, tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/activate"), 200, "activate")

	// G4 weight cap.
	goal := func(tok string, weight any) resp {
		return tc.as(t, tok, "POST", "/hr/performance/goals", body(map[string]any{"cycle_id": cid, "title": "Goal " + uniq(), "weight": weight}))
	}
	g1 := goal(alice.tok, 60)
	expect(t, g1, 201, "goal 60")
	g1id := g1.str("data.id")
	expect(t, goal(alice.tok, 40), 201, "goal 40")
	expectErr(t, goal(alice.tok, "0.01"), 409, "WEIGHT_EXCEEDED", "G4 over 100")

	// G5 key results drive progress; history recorded.
	kr := tc.as(t, alice.tok, "POST", "/hr/performance/goals/"+g1id+"/key-results", body(map[string]any{"title": "Ship features", "start_value": 0, "target_value": 10}))
	expect(t, kr, 201, "key result")
	krID := kr.list("data.key_results")[0].(map[string]any)["id"].(string)
	ci := tc.as(t, alice.tok, "POST", "/hr/performance/goals/"+g1id+"/check-ins", body(map[string]any{"key_result_id": krID, "value": 5, "note": "half way"}))
	if expect(t, ci, 201, "G5 KR check-in"); num(ci, "data.progress") != 50 {
		t.Fatalf("G5 progress: %s", ci.Raw)
	}
	expectErr(t, tc.as(t, alice.tok, "POST", "/hr/performance/goals/"+g1id+"/check-ins", body(map[string]any{"progress": 90})), 409, "HAS_KEY_RESULTS", "G5 direct")
	if n := sqlStr(t, "SELECT count(*) FROM performance_checkins WHERE goal_id = ?", g1id); n != "2" {
		t.Fatalf("G5 check-ins: %s", n)
	}

	// G6 access: manager writes, colleague sees nothing.
	expect(t, tc.as(t, mgr.tok, "PATCH", "/hr/performance/goals/"+g1id, body(map[string]any{"title": "Ship v2"})), 200, "G6 manager edits")
	expectErr(t, tc.as(t, bob.tok, "GET", "/hr/performance/goals/"+g1id), 404, "NOT_FOUND", "G6 colleague")
	expectErr(t, tc.as(t, bob.tok, "GET", "/hr/performance/goals?employee_id="+alice.emp), 404, "NOT_FOUND", "G6 colleague lists")
	if mine := tc.as(t, mgr.tok, "GET", "/hr/performance/goals?employee_id="+alice.emp); len(mine.list("data")) != 2 {
		t.Fatalf("G6 manager lists report: %s", mine.Raw)
	}
	expectErr(t, tc.as(t, alice.tok, "GET", "/hr/performance/cycles/"+cid+"/nine-box"), 403, "FORBIDDEN", "G13 member 9-box")
	expectErr(t, tc.as(t, alice.tok, "GET", "/hr/performance/reviews"), 403, "FORBIDDEN", "G13 member lists reviews")

	// G8 launch reviews, idempotent.
	l := tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/launch-reviews")
	if expect(t, l, 200, "G8 launch"); num(l, "data.self_reviews_created") != 1 || num(l, "data.manager_reviews_created") != 1 {
		t.Fatalf("G8: %s", l.Raw)
	}
	if again := tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/launch-reviews"); num(again, "data.self_reviews_created")+num(again, "data.manager_reviews_created") != 0 {
		t.Fatalf("G8 idempotent: %s", again.Raw)
	}
	peer := tc.do(t, "POST", "/hr/performance/reviews", body(map[string]any{"cycle_id": cid, "employee_id": alice.emp, "reviewer_employee_id": bob.emp, "type": "peer"}))
	expect(t, peer, 201, "assign peer")
	expectErr(t, tc.do(t, "POST", "/hr/performance/reviews", body(map[string]any{"cycle_id": cid, "employee_id": alice.emp, "reviewer_employee_id": bob.emp, "type": "peer"})), 409, "CONFLICT", "duplicate peer")

	// G9 submit.
	assigned := func(p perfPerson) string {
		r := tc.as(t, p.tok, "GET", "/hr/performance/reviews/assigned?status=pending")
		if len(r.list("data")) != 1 {
			t.Fatalf("one review to write: %s", r.Raw)
		}
		return r.list("data")[0].(map[string]any)["id"].(string)
	}
	mrev, srev := assigned(mgr), assigned(alice)
	expectErr(t, tc.as(t, bob.tok, "POST", "/hr/performance/reviews/"+mrev+"/submit", body(map[string]any{"ratings": ratings(4, 4, 4, 4), "potential": "high"})), 404, "NOT_FOUND", "G9 not the reviewer")
	expectErr(t, tc.as(t, mgr.tok, "POST", "/hr/performance/reviews/"+mrev+"/submit", body(map[string]any{"ratings": ratings(4, 4, 4, 4)})), 400, "VALIDATION_ERROR", "G9 potential required")
	sub := tc.as(t, mgr.tok, "POST", "/hr/performance/reviews/"+mrev+"/submit", body(map[string]any{"ratings": ratings(4, 4, 3, 3), "potential": "high", "strengths": "PRIVATE-STRENGTH"}))
	if expect(t, sub, 200, "G9 manager submits"); num(sub, "data.overall_score") != 3.5 {
		t.Fatalf("overall: %s", sub.Raw)
	}
	expectErr(t, tc.as(t, mgr.tok, "POST", "/hr/performance/reviews/"+mrev+"/submit", body(map[string]any{"ratings": ratings(4, 4, 4, 4), "potential": "high"})), 409, "REVIEW_STATE", "G9 immutable")
	expect(t, tc.as(t, alice.tok, "POST", "/hr/performance/reviews/"+srev+"/submit", body(map[string]any{"ratings": ratings(5, 5, 5, 5)})), 200, "self review")
	expect(t, tc.as(t, bob.tok, "POST", "/hr/performance/reviews/"+peer.str("data.id")+"/submit", body(map[string]any{"ratings": ratings(3, 3, 3, 3), "improvements": "PRIVATE-GAP"})), 200, "peer review")

	// G10 nothing before completion; G11 9-box.
	if r := tc.as(t, alice.tok, "GET", "/hr/performance/reviews/me"); len(r.list("data")) != 0 {
		t.Fatalf("G10 before completion: %s", r.Raw)
	}
	box := tc.do(t, "GET", "/hr/performance/cycles/"+cid+"/nine-box")
	expect(t, box, 200, "G11 9-box")
	entry := box.list("data.entries")[0].(map[string]any)
	if entry["box"] != "high/high" || entry["goal_score"].(float64) != 30 {
		t.Fatalf("G11 entry: %v", entry)
	}
	expect(t, tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/complete"), 200, "complete")
	mine := tc.as(t, alice.tok, "GET", "/hr/performance/reviews/me")
	if len(mine.list("data")) != 3 {
		t.Fatalf("G10 after completion: %s", mine.Raw)
	}
	for _, it := range mine.list("data") {
		if r := it.(map[string]any); r["type"] == "peer" && r["reviewer_employee_id"] != nil {
			t.Fatalf("G10 peer anonymous: %v", r)
		}
	}
	expectErr(t, tc.as(t, alice.tok, "POST", "/hr/performance/goals/"+g1id+"/check-ins", body(map[string]any{"key_result_id": krID, "value": 6})), 409, "CYCLE_STATE", "G7 completed")

	// G12 privacy and the four event types.
	for _, q := range []string{
		"SELECT count(*) FROM performance_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%PRIVATE%' OR payload::text LIKE '%ratings%' OR payload::text LIKE '%potential%' OR payload::text LIKE '%overall%')",
		"SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'performance.%' AND (metadata::text LIKE '%PRIVATE%' OR metadata::text LIKE '%potential%' OR metadata::text LIKE '%overall%')",
	} {
		if n := sqlStr(t, q, tc.id); n != "0" {
			t.Fatalf("G12 leak (%s rows): %s", n, q)
		}
	}
	if n := sqlStr(t, "SELECT count(DISTINCT routing_key) FROM performance_events_outbox WHERE tenant_id = ?", tc.id); n != "4" {
		t.Fatalf("4 event types expected, got %s", n)
	}

	// G1 isolation and G14 pagination.
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", "/hr/performance/cycles/"+cid), 404, "NOT_FOUND", "G1 foreign id")
	expectErr(t, call(t, "GET", "/hr/performance/cycles", host(tc.slug), token(other.token)), 403, "", "G1 foreign token")
	if p := tc.do(t, "GET", "/hr/performance/cycles?page=0&per_page=500"); num(p, "meta.page") != 1 || num(p, "meta.per_page") != 100 {
		t.Fatalf("G14 clamp: %s", p.Raw)
	}
}
