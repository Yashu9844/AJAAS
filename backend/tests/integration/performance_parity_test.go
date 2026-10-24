package integration

import (
	"testing"
)

// TestPerformanceCalibrationAndFeedback — G16–G20 end to end: calibration in review drives the 9-box and the subject's
// final rating; continuous feedback and requests respect visibility; nothing private reaches events or audit.
func TestPerformanceCalibrationAndFeedback(t *testing.T) {
	tc := newTimeTenant(t)
	alice := perfPerson{user: tc.memberUser, emp: tc.memberEmp, tok: tc.memberTok}
	mgr, bob := tc.perfPerson(t, "cmgr"), tc.perfPerson(t, "cbob")
	expect(t, tc.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": alice.user, "department_id": tc.mkDept(t, "Cal"),
		"is_primary": true, "manager_user_id": mgr.user})), 201, "alice reports to mgr")
	cy := tc.do(t, "POST", "/hr/performance/cycles", body(map[string]any{"title": "Cal " + uniq(), "start_date": "2026-01-01", "end_date": "2026-06-30"}))
	expect(t, cy, 201, "cycle")
	cid := cy.str("data.id")
	expect(t, tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/activate"), 200, "activate")
	expect(t, tc.as(t, alice.tok, "POST", "/hr/performance/goals", body(map[string]any{"cycle_id": cid, "title": "Goal", "weight": 50})), 201, "goal")
	calib := func(tok string, b map[string]any) resp {
		return tc.as(t, tok, "POST", "/hr/performance/cycles/"+cid+"/calibrations", body(b))
	}
	cb := map[string]any{"employee_id": alice.emp, "overall_score": 4.25, "reason": "SECRET-REASON calibration session"}
	expectErr(t, calib(tc.token, cb), 409, "CYCLE_STATE", "G16 not in review")
	expect(t, tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/launch-reviews"), 200, "launch")
	expectErr(t, calib(tc.token, cb), 409, "NOT_RATED", "G16 no manager review yet")
	mrev := tc.as(t, mgr.tok, "GET", "/hr/performance/reviews/assigned").list("data")[0].(map[string]any)["id"].(string)
	expect(t, tc.as(t, mgr.tok, "POST", "/hr/performance/reviews/"+mrev+"/submit", body(map[string]any{"ratings": ratings(4, 4, 3, 3), "potential": "medium"})), 200, "manager review")

	expectErr(t, calib(alice.tok, cb), 403, "FORBIDDEN", "G16 needs performance:calibrate")
	c := calib(tc.token, cb)
	if expect(t, c, 201, "G16 calibrate"); num(c, "data.before_score") != 3.5 || num(c, "data.after_score") != 4.25 || c.str("data.after_potential") != "medium" {
		t.Fatalf("G16 before/after: %s", c.Raw)
	}
	expect(t, calib(tc.token, map[string]any{"employee_id": alice.emp, "potential": "high", "reason": "SECRET-REASON leadership"}), 201, "second calibration")
	if h := tc.do(t, "GET", "/hr/performance/cycles/"+cid+"/calibrations?employee_id="+alice.emp); len(h.list("data")) != 2 {
		t.Fatalf("G16 history: %s", h.Raw)
	}
	box := tc.do(t, "GET", "/hr/performance/cycles/"+cid+"/nine-box")
	if e := box.list("data.entries")[0].(map[string]any); e["box"] != "high/high" || e["calibrated"] != true {
		t.Fatalf("G16 9-box: %v", e)
	}
	expect(t, tc.do(t, "POST", "/hr/performance/cycles/"+cid+"/complete"), 200, "complete")
	mine := tc.as(t, alice.tok, "GET", "/hr/performance/reviews/me?cycle_id="+cid)
	final, _ := mine.get("data.0.final").(map[string]any)
	if final == nil || final["overall_score"] != 4.25 || final["potential"] != "high" || final["calibrated"] != true {
		t.Fatalf("G17 final rating: %s", mine.Raw)
	}

	// G18/G19 continuous feedback.
	give := func(tok string, b map[string]any) resp {
		return tc.as(t, tok, "POST", "/hr/performance/feedback", body(b))
	}
	expect(t, give(bob.tok, map[string]any{"to_employee_id": alice.emp, "kind": "praise", "visibility": "public", "message": "SECRET-MESSAGE great launch"}), 201, "public praise")
	expectErr(t, give(bob.tok, map[string]any{"to_employee_id": alice.emp, "kind": "constructive", "visibility": "public", "message": "x"}), 400, "VALIDATION_ERROR", "G18 public constructive")
	reqs := tc.as(t, alice.tok, "POST", "/hr/performance/feedback/requests", body(map[string]any{"responder_employee_ids": []string{bob.emp}, "question": "SECRET-QUESTION how was it?"}))
	expect(t, reqs, 201, "G19 request")
	rid := reqs.list("data")[0].(map[string]any)["id"].(string)
	expectErr(t, tc.as(t, alice.tok, "POST", "/hr/performance/feedback/requests", body(map[string]any{"responder_employee_ids": []string{bob.emp}, "question": "again"})), 409, "CONFLICT", "G19 duplicate pending")
	expectErr(t, give(mgr.tok, map[string]any{"to_employee_id": alice.emp, "kind": "praise", "visibility": "private", "message": "m", "request_id": rid}), 404, "NOT_FOUND", "G19 not the responder")
	expect(t, give(bob.tok, map[string]any{"to_employee_id": alice.emp, "kind": "constructive", "visibility": "private", "message": "SECRET-MESSAGE add a rollback plan", "request_id": rid}), 201, "G19 answer")
	if r := tc.as(t, alice.tok, "GET", "/hr/performance/feedback/requests/mine?status=answered"); len(r.list("data")) != 1 {
		t.Fatalf("G19 answered: %s", r.Raw)
	}
	if r := tc.as(t, mgr.tok, "GET", "/hr/performance/feedback?employee_id="+alice.emp); len(r.list("data")) != 2 {
		t.Fatalf("G18 manager sees the report's feedback: %s", r.Raw)
	}
	expectErr(t, tc.as(t, bob.tok, "GET", "/hr/performance/feedback?employee_id="+alice.emp), 404, "NOT_FOUND", "G18 colleague")
	if r := tc.as(t, mgr.tok, "GET", "/hr/performance/feedback/public"); len(r.list("data")) != 1 {
		t.Fatalf("G18 praise wall: %s", r.Raw)
	}
	if r := tc.as(t, alice.tok, "GET", "/hr/performance/feedback/received"); len(r.list("data")) != 2 {
		t.Fatalf("G18 received: %s", r.Raw)
	}

	// G20 privacy: the event data (not the envelope, whose timestamps can contain digits like 4.25).
	for _, q := range []string{
		"SELECT count(*) FROM performance_events_outbox WHERE tenant_id = ? AND ((payload->'payload')::text LIKE '%SECRET%' OR (payload->'payload')::text LIKE '%4.25%' OR (payload->'payload')::text LIKE '%potential%')",
		"SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'performance.%' AND (metadata::text LIKE '%SECRET%' OR metadata::text LIKE '%4.25%')",
	} {
		if n := sqlStr(t, q, tc.id); n != "0" {
			t.Fatalf("G20 leak (%s rows): %s", n, q)
		}
	}
	if n := sqlStr(t, "SELECT count(*) FROM performance_events_outbox WHERE tenant_id = ? AND routing_key IN ('performance.rating_calibrated','performance.feedback_given','performance.feedback_requested')", tc.id); n != "5" {
		t.Fatalf("parity events: %s", n)
	}
}
