package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func (tc *timeCtx) openJob(t *testing.T, headcount int, extra map[string]any) string {
	t.Helper()
	b := map[string]any{"title": "Engineer " + uniq(), "headcount": headcount, "employment_type": "full_time", "description": "Build things"}
	for k, v := range extra {
		b[k] = v
	}
	r := tc.do(t, "POST", "/recruitment/jobs", body(b))
	expect(t, r, 201, "create job")
	id := r.str("data.id")
	expect(t, tc.do(t, "POST", "/recruitment/jobs/"+id+"/status", body(map[string]string{"status": "open"})), 200, "open job")
	return id
}

func (tc *timeCtx) addCandidate(t *testing.T, job, email string) resp {
	t.Helper()
	return tc.do(t, "POST", "/recruitment/candidates", body(map[string]any{"job_id": job, "first_name": "Priya", "last_name": "Nair",
		"email": email, "source": "referral", "expected_ctc": 1500000}))
}

func (tc *timeCtx) stage(t *testing.T, cand string, stages ...string) {
	t.Helper()
	for _, s := range stages {
		expect(t, tc.do(t, "POST", "/recruitment/candidates/"+cand+"/stage", body(map[string]string{"stage": s})), 200, "stage "+s)
	}
}

func (tc *timeCtx) makeOffer(t *testing.T, cand string) resp {
	t.Helper()
	return tc.do(t, "POST", "/recruitment/offers", body(map[string]any{"candidate_id": cand, "offered_ctc": 1800000,
		"joining_date": ymd(time.Now().UTC().AddDate(0, 0, 30))}))
}

// TestRecruitmentLifecycle — G5, G6, G8, G9, G10, G12 end to end against Modules 0, 1 and 2.
func TestRecruitmentLifecycle(t *testing.T) {
	tc := newTimeTenant(t)
	dept := tc.mkDept(t, "Engineering "+uniq())
	expectErr(t, tc.do(t, "POST", "/recruitment/jobs", body(map[string]any{"title": "x", "headcount": 1, "employment_type": "intern",
		"description": "x", "department_id": "00000000-0000-0000-0000-000000000001"})), 400, "VALIDATION_ERROR", "unknown department")
	job := tc.openJob(t, 1, map[string]any{"department_id": dept})
	other := tc.openJob(t, 1, nil)

	email := "priya." + uniq() + "@example.com"
	c := tc.addCandidate(t, job, email)
	expect(t, c, 201, "add candidate")
	cand := c.str("data.id")
	expectErr(t, tc.addCandidate(t, job, strings.ToUpper(email)), 409, "CONFLICT", "G5 duplicate email any case")
	expect(t, tc.addCandidate(t, other, email), 201, "G5 same email on another job")
	expectErr(t, tc.do(t, "POST", "/recruitment/candidates/"+cand+"/stage", body(map[string]string{"stage": "interview"})), 409, "INVALID_STAGE", "skip screening")
	tc.stage(t, cand, "screening", "interview")
	hist := tc.do(t, "GET", "/recruitment/candidates/"+cand).list("data.history")
	if len(hist) != 3 || hist[2].(map[string]any)["to_stage"] != "interview" || hist[2].(map[string]any)["actor_user_id"] != tc.adminID {
		t.Fatalf("G6 history: %v", hist)
	}

	iv := tc.do(t, "POST", "/recruitment/interviews", body(map[string]any{"candidate_id": cand, "round_name": "System design",
		"interviewer_employee_id": tc.memberEmp, "scheduled_at": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339), "duration_mins": 60}))
	expect(t, iv, 201, "schedule interview")
	ivID := iv.str("data.id")
	if mine := tc.as(t, tc.memberTok, "GET", "/recruitment/interviews/me"); len(mine.list("data")) != 1 {
		t.Fatalf("interviewer sees own interview: %s", mine.Raw)
	}
	fb := body(map[string]any{"rating": 5, "recommendation": "strong_hire", "notes": "excellent"})
	expectErr(t, tc.do(t, "POST", "/recruitment/interviews/"+ivID+"/feedback", fb), 404, "NOT_FOUND", "G8 admin is not the interviewer")
	expect(t, tc.as(t, tc.memberTok, "POST", "/recruitment/interviews/"+ivID+"/feedback", fb), 200, "G8 interviewer feedback")
	expectErr(t, tc.as(t, tc.memberTok, "POST", "/recruitment/interviews/"+ivID+"/feedback", fb), 409, "INTERVIEW_STATE", "G8 twice")

	first := tc.makeOffer(t, cand)
	expect(t, first, 201, "offer")
	expectErr(t, tc.makeOffer(t, cand), 409, "OFFER_EXISTS", "G9 second open offer")
	expectErr(t, tc.do(t, "POST", "/recruitment/candidates/"+cand+"/hire", body(map[string]string{"employee_code": "H-" + uniq()})), 409, "NOT_HIREABLE", "hire before acceptance")
	expect(t, tc.do(t, "POST", "/recruitment/offers/"+first.str("data.id")+"/decision", body(map[string]string{"decision": "declined"})), 200, "decline")
	second := tc.makeOffer(t, cand)
	expect(t, second, 201, "G9 offer after decline")
	expect(t, tc.do(t, "POST", "/recruitment/offers/"+second.str("data.id")+"/decision", body(map[string]string{"decision": "accepted"})), 200, "accept")

	code := "H-" + strings.ToUpper(uniq())
	hire := tc.do(t, "POST", "/recruitment/candidates/"+cand+"/hire", body(map[string]string{"employee_code": code}))
	expect(t, hire, 200, "G10 hire")
	if hire.str("data.job_status") != "filled" {
		t.Fatalf("G10 headcount 1 fills the job: %s", hire.Raw)
	}
	emp := tc.do(t, "GET", "/employees/"+hire.str("data.employee_id"))
	if expect(t, emp, 200, "hired employee profile"); emp.str("data.employee_code") != code || emp.str("data.user_id") != hire.str("data.user_id") {
		t.Fatalf("G10 employee: %s", emp.Raw)
	}
	if s := sqlStr(t, "SELECT status FROM users WHERE id = ?", hire.str("data.user_id")); s != "invited" {
		t.Fatalf("G10 user status %s", s)
	}
	if s := sqlStr(t, "SELECT hired_count||'/'||status FROM recruitment_jobs WHERE id = ?", job); s != "1/filled" {
		t.Fatalf("G10 job %s", s)
	}
	expectErr(t, tc.addCandidate(t, job, "late."+uniq()+"@example.com"), 409, "JOB_NOT_OPEN", "G4 filled job")

	if n := sqlStr(t, "SELECT count(*) FROM recruitment_events_outbox WHERE tenant_id = ? AND (payload::text ILIKE ? OR payload::text LIKE '%Priya%' OR payload::text LIKE '%1800000%' OR payload::text LIKE '%excellent%')", tc.id, "%"+email+"%"); n != "0" {
		t.Fatalf("G12 events leak PII (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'recruitment.%' AND (metadata::text ILIKE ? OR metadata::text LIKE '%Priya%' OR metadata::text LIKE '%1800000%' OR metadata::text LIKE '%excellent%')", tc.id, "%"+email+"%"); n != "0" {
		t.Fatalf("G12 audit leaks PII (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(DISTINCT routing_key) FROM recruitment_events_outbox WHERE tenant_id = ?", tc.id); n != "4" {
		t.Fatalf("4 event types expected, got %s", n)
	}
}

// TestRecruitmentRBACAndIsolation — G1, G13 exact grants, G14 paging.
func TestRecruitmentRBACAndIsolation(t *testing.T) {
	a, b := newTimeTenant(t), newTimeTenant(t)
	job := a.openJob(t, 2, nil)
	cand := a.addCandidate(t, job, "rbac."+uniq()+"@example.com").str("data.id")
	for _, p := range [][2]string{{"GET", "/recruitment/jobs"}, {"POST", "/recruitment/jobs"}, {"GET", "/recruitment/candidates"},
		{"POST", "/recruitment/candidates/" + cand + "/hire"}, {"POST", "/recruitment/offers"}} {
		expectErr(t, a.as(t, a.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "G13 member "+p[0]+" "+p[1])
	}
	expect(t, a.as(t, a.memberTok, "GET", "/recruitment/interviews/me"), 200, "G13 member self endpoint")
	reader, _, _ := a.limitedUser(t, [2]string{"recruitment", "read"})
	expect(t, a.as(t, reader, "GET", "/recruitment/candidates/"+cand), 200, "read reads")
	expectErr(t, a.as(t, reader, "POST", "/recruitment/candidates/"+cand+"/stage", body(map[string]string{"stage": "screening"})), 403, "FORBIDDEN", "read cannot move")
	manager, _, _ := a.limitedUser(t, [2]string{"recruitment", "manage"})
	expect(t, a.as(t, manager, "POST", "/recruitment/candidates/"+cand+"/stage", body(map[string]string{"stage": "screening"})), 200, "manage moves")
	expectErr(t, a.as(t, manager, "POST", "/recruitment/candidates/"+cand+"/hire", body(map[string]string{"employee_code": "X-1"})), 403, "FORBIDDEN", "G13 manage cannot hire")

	for _, p := range []string{"/recruitment/jobs/" + job, "/recruitment/candidates/" + cand} {
		expectErr(t, b.do(t, "GET", p), 404, "NOT_FOUND", "G1 tenant B "+p)
	}
	expectErr(t, b.addCandidate(t, job, "x."+uniq()+"@example.com"), 404, "NOT_FOUND", "G1 foreign job")
	expectErr(t, call(t, "GET", "/recruitment/jobs", host(b.slug), token(a.token)), 403, "", "G1 A token on B host")
	for _, q := range [][]string{{"per_page", "0"}, {"page", "-1", "per_page", "abc"}, {"per_page", "100000"}} {
		r := a.do(t, "GET", "/recruitment/candidates", query(q...))
		if expect(t, r, 200, fmt.Sprint("G14 ", q)); num(r, "meta.per_page") < 1 || num(r, "meta.per_page") > 100 {
			t.Fatalf("G14 %v: %s", q, r.Raw)
		}
	}
}
