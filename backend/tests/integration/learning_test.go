package integration

import (
	"testing"
	"time"
)

// TestLearning ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â LG1..LG6: a joiner is enrolled by a department rule exactly once, an unsafe URL is refused, a
// manager-verified completion needs a different person, a retired course keeps its completions, overdue follows
// the tenant's time zone and events and audit hold ids only.
func TestLearning(t *testing.T) {
	cc := newCoreTenant(t)
	const base = "/hr/learning"

	// Settings: the default zone, and an invalid one refused.
	if s := cc.do(t, "GET", base+"/settings"); s.str("data.time_zone") != "Asia/Kolkata" {
		t.Fatalf("default zone: %s", s.Raw)
	}
	expectErr(t, cc.do(t, "PUT", base+"/settings", body(map[string]any{"time_zone": "Mars/Base"})), 400, "VALIDATION_ERROR", "invalid zone")

	// LG4 unsafe links are refused.
	for _, u := range []string{"javascript:alert(1)", "http://learn.example.com/x", "data:text/html,<b>x</b>", "https://user:pw@learn.example.com/x"} {
		expectErr(t, cc.do(t, "POST", base+"/courses", body(map[string]any{"title": "Bad", "content_kind": "link", "content_url": u})), 400, "VALIDATION_ERROR", "LG4 "+u)
	}
	mk := func(title string, extra map[string]any) string {
		b := map[string]any{"title": title + " " + uniq(), "content_kind": "link", "content_url": "https://learn.example.com/" + uniq(), "verification": "manager", "self_enrol": true}
		for k, v := range extra {
			b[k] = v
		}
		c := cc.do(t, "POST", base+"/courses", body(b))
		expect(t, c, 201, "course")
		id := c.str("data.id")
		expect(t, cc.do(t, "POST", base+"/courses/"+id+"/publish"), 200, "publish")
		return id
	}
	course := mk("Fire safety", nil)
	if v := cc.do(t, "PATCH", base+"/courses/"+course, body(map[string]any{"duration_mins": 30})); num(v, "data.version") != 2 {
		t.Fatalf("a content change bumps the version: %s", v.Raw)
	}
	if l := cc.do(t, "GET", base+"/courses/"+course+"/versions"); len(l.list("data")) != 2 {
		t.Fatalf("versions: %s", l.Raw)
	}
	if cat := cc.as(t, cc.memberTok, "GET", base+"/catalog"); len(cat.list("data")) != 1 {
		t.Fatalf("a learner browses published courses: %s", cat.Raw)
	}
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/courses", body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "a learner cannot manage the catalogue")

	// LG1 a department rule enrols a joiner exactly once.
	rule := cc.do(t, "POST", base+"/assignments", body(map[string]any{"target_kind": "department", "target_id": cc.dept1, "course_id": course, "due_in_days": 14}))
	expect(t, rule, 201, "department rule")
	if num(rule, "data.matched") != 0 {
		t.Fatalf("nobody is placed in the department yet: %s", rule.Raw)
	}
	expectErr(t, cc.do(t, "POST", base+"/assignments", body(map[string]any{"target_kind": "all", "course_id": course, "recurrence_days": 29, "due_in_days": 5})), 400, "VALIDATION_ERROR", "recurrence below 30 days")
	cc.complete(t, "hire", cc.memberEmp, cc.day(-10), map[string]any{"position_id": cc.p1, "manager_employee_id": cc.adminEmp, "worker_type": "employee", "employment_type": "full_time"})
	mine := cc.as(t, cc.memberTok, "GET", base+"/enrolments/me")
	if len(mine.list("data")) != 1 {
		t.Fatalf("LG1 the hire enrolled the joiner: %s", mine.Raw)
	}
	cc.complete(t, "promote", cc.memberEmp, cc.day(-5), map[string]any{"grade_id": cc.g2})
	if again := cc.as(t, cc.memberTok, "GET", base+"/enrolments/me"); len(again.list("data")) != 1 {
		t.Fatalf("LG1 a later action does not enrol twice: %s", again.Raw)
	}
	eid := mine.list("data")[0].(map[string]any)["id"].(string)
	if sqlStr(t, "SELECT count(*) FROM lrn_enrolments WHERE tenant_id = ? AND employee_profile_id = ?", cc.id, cc.memberEmp) != "1" {
		t.Fatal("LG1 exactly one row")
	}

	// LG5 a manager-verified completion cannot finish the enrolment until someone else verifies it.
	expect(t, cc.as(t, cc.memberTok, "POST", base+"/enrolments/"+eid+"/start"), 200, "start")
	done := cc.as(t, cc.memberTok, "POST", base+"/enrolments/"+eid+"/complete", body(map[string]any{"score": 91, "evidence_ref": "certs/secret-evidence.pdf"}))
	expect(t, done, 200, "complete")
	if done.str("data.status") != "in_progress" || done.str("data.completion.status") != "pending" {
		t.Fatalf("LG5 pending verification: %s", done.Raw)
	}
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/enrolments/"+eid+"/complete"), 409, "COMPLETION_PENDING", "second submission")
	comp := done.str("data.completion.id")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/completions/"+comp+"/verify"), 403, "LEARNING_SELF_VERIFY", "own completion")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/completions/"+comp+"/reject", body(map[string]any{"comment": "no"})), 403, "LEARNING_SELF_VERIFY", "own completion rejected")
	if p := cc.do(t, "GET", base+"/completions/pending"); len(p.list("data")) != 1 {
		t.Fatalf("HR sees the pending completion: %s", p.Raw)
	}
	rej := cc.do(t, "POST", base+"/completions/"+comp+"/reject", body(map[string]any{"comment": "needs the signed sheet"}))
	expect(t, rej, 200, "reject")
	again := cc.as(t, cc.memberTok, "POST", base+"/enrolments/"+eid+"/complete", body(map[string]any{"evidence_ref": "certs/signed.pdf"}))
	expect(t, again, 200, "resubmit")
	ver := cc.do(t, "POST", base+"/completions/"+again.str("data.completion.id")+"/verify")
	if expect(t, ver, 200, "verify"); ver.str("data.status") != "completed" || ver.str("data.completion.status") != "verified" {
		t.Fatalf("verified: %s", ver.Raw)
	}
	if tr := cc.as(t, cc.memberTok, "GET", base+"/transcript/me"); len(tr.list("data")) != 1 {
		t.Fatalf("transcript: %s", tr.Raw)
	}
	if c := cc.do(t, "POST", base+"/enrolments/"+eid+"/correct", body(map[string]any{"reason": "score typo", "score": 95})); c.str("data.completion.supersedes_id") == "" {
		t.Fatalf("a correction supersedes the live completion: %s", c.Raw)
	} else if n := sqlStr(t, "SELECT count(*) FROM lrn_completions WHERE enrolment_id = ?", eid); n != "3" {
		t.Fatalf("nothing is deleted (%s rows)", n)
	}

	// LG2 retiring keeps the completions; a retired course cannot be self-enrolled.
	expect(t, cc.do(t, "POST", base+"/courses/"+course+"/retire"), 200, "retire")
	if tr := cc.as(t, cc.memberTok, "GET", base+"/transcript/me"); len(tr.list("data")) != 1 {
		t.Fatalf("LG2 transcript after retirement: %s", tr.Raw)
	}
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/enrolments/self", body(map[string]any{"course_id": course})), 409, "COURSE_STATE", "self-enrol in a retired course")

	// Self-enrolment, plans and progress.
	open := mk("Data privacy", map[string]any{"verification": "self"})
	se := cc.as(t, cc.memberTok, "POST", base+"/enrolments/self", body(map[string]any{"course_id": open}))
	expect(t, se, 201, "self-enrol")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/enrolments/self", body(map[string]any{"course_id": open})), 409, "ALREADY_ENROLLED", "twice")
	plan := cc.do(t, "POST", base+"/plans", body(map[string]any{"name": "Basics " + uniq()}))
	expect(t, plan, 201, "plan")
	pid := plan.str("data.id")
	expect(t, cc.do(t, "PUT", base+"/plans/"+pid+"/items", body(map[string]any{"items": []map[string]any{{"course_id": open}}})), 200, "plan items")
	if pr := cc.as(t, cc.memberTok, "GET", base+"/plans/"+pid+"/progress"); pr.get("data.complete") != false {
		t.Fatalf("not complete yet: %s", pr.Raw)
	}
	selfEnrolment := se.list("data")[0].(map[string]any)["id"].(string)
	expect(t, cc.as(t, cc.memberTok, "POST", base+"/enrolments/"+selfEnrolment+"/complete"), 200, "self-attested")
	if pr := cc.as(t, cc.memberTok, "GET", base+"/plans/"+pid+"/progress"); pr.get("data.complete") != true {
		t.Fatalf("complete after the mandatory course: %s", pr.Raw)
	}

	// LG3 overdue follows the tenant's time zone: a due date that is "today" in Pago Pago (UTC-11) is already past in
	// Kiritimati (UTC+14).
	late := mk("Late training", map[string]any{"verification": "self"})
	asn := cc.do(t, "POST", base+"/assignments", body(map[string]any{"target_kind": "employee", "target_id": cc.memberEmp, "course_id": late, "due_in_days": 5}))
	expect(t, asn, 201, "late assignment")
	pago, _ := time.LoadLocation("Pacific/Pago_Pago")
	due := ymd(time.Now().In(pago))
	if err := testDB.Exec("UPDATE lrn_enrolments SET due_date = ? WHERE assignment_id = ?", due, asn.str("data.assignment.id")).Error; err != nil {
		t.Fatal(err)
	}
	expect(t, cc.do(t, "PUT", base+"/settings", body(map[string]any{"time_zone": "Pacific/Pago_Pago"})), 200, "zone west")
	if o := cc.as(t, cc.memberTok, "GET", base+"/enrolments/me?status=overdue"); len(o.list("data")) != 0 {
		t.Fatalf("LG3 due today is not overdue in Pago Pago: %s", o.Raw)
	}
	expect(t, cc.do(t, "PUT", base+"/settings", body(map[string]any{"time_zone": "Pacific/Kiritimati"})), 200, "zone east")
	if o := cc.as(t, cc.memberTok, "GET", base+"/enrolments/me?status=overdue"); len(o.list("data")) != 1 {
		t.Fatalf("LG3 the same date is overdue in Kiritimati: %s (all: %s, due %s)", o.Raw, cc.as(t, cc.memberTok, "GET", base+"/enrolments/me").Raw, due)
	}

	// Reports, waiver, recurrence run.
	rep := cc.do(t, "GET", base+"/reports/compliance")
	if expect(t, rep, 200, "compliance"); len(rep.list("data")) < 2 {
		t.Fatalf("compliance rows: %s", rep.Raw)
	}
	lateEnrol := cc.as(t, cc.memberTok, "GET", base+"/enrolments/me?status=overdue").list("data")[0].(map[string]any)["id"].(string)
	expect(t, cc.do(t, "POST", base+"/enrolments/"+lateEnrol+"/waive", body(map[string]any{"reason": "on leave"})), 200, "waive")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/enrolments/"+lateEnrol+"/waive", body(map[string]any{"reason": "x"})), 403, "FORBIDDEN", "a learner cannot waive")
	expect(t, cc.do(t, "POST", base+"/recurrence/run"), 200, "recurrence")
	expect(t, cc.do(t, "POST", base+"/assignments/"+rule.str("data.assignment.id")+"/cancel"), 200, "cancel")

	// LG6 events and audit hold ids only.
	if n := sqlStr(t, "SELECT count(*) FROM lrn_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%secret-evidence%' OR payload::text LIKE '%signed sheet%' OR payload::text LIKE '%\"score\"%' OR payload::text LIKE '%on leave%')", cc.id); n != "0" {
		t.Fatalf("LG6 events leak content (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'learning.%' AND (metadata::text LIKE '%secret-evidence%' OR metadata::text LIKE '%signed sheet%' OR metadata::text LIKE '%on leave%' OR metadata::text LIKE '%score typo%')", cc.id); n != "0" {
		t.Fatalf("LG6 audit leaks content (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM lrn_events_outbox WHERE tenant_id = ?", cc.id); n == "0" {
		t.Fatal("events were written")
	}
}
