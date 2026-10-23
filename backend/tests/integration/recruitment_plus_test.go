package integration

import (
	"sync"
	"testing"
	"time"
)

// TestRecruitmentParity ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â RG1..RG5: the last-seat reservation, position-linked requisitions, scorecards and offer
// approval, end to end against HR/Core, employees and onboarding.
func TestRecruitmentParity(t *testing.T) {
	cc := newCoreTenant(t)
	const base = "/hr/recruitment"
	approver2, _, _ := cc.limitedUser(t, [2]string{"recruitment", "approve"}, [2]string{"recruitment", "read"})

	// RG2 requisition for the core position: 1 seat open, so two seats are refused; submit ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ approve opens a draft job.
	expectErr(t, cc.do(t, "POST", base+"/requisitions", body(map[string]any{"position_id": cc.p1, "headcount": 2, "justification": "scale"})), 409, "NO_VACANCY", "more than open")
	rq := cc.do(t, "POST", base+"/requisitions", body(map[string]any{"position_id": cc.p1, "headcount": 1, "justification": "grow the team"}))
	expect(t, rq, 201, "requisition")
	rid := rq.str("data.id")
	expect(t, cc.do(t, "POST", base+"/requisitions/"+rid+"/submit"), 200, "submit")
	expectErr(t, cc.do(t, "POST", base+"/requisitions", body(map[string]any{"position_id": cc.p1, "headcount": 1, "justification": "again"})), 409, "NO_VACANCY", "seat held by the submitted requisition")
	expectErr(t, cc.do(t, "POST", base+"/requisitions/"+rid+"/approve"), 403, "RECRUITMENT_SELF_APPROVAL", "own requisition")
	ap := cc.as(t, approver2, "POST", base+"/requisitions/"+rid+"/approve", body(map[string]any{"comment": "ok"}))
	expect(t, ap, 200, "approve requisition")
	jobID := ap.str("data.job_id")
	if jobID == "" || sqlStr(t, "SELECT status||'/'||headcount FROM recruitment_jobs WHERE id = ?", jobID) != "draft/1" {
		t.Fatalf("RP-03 draft job: %s", ap.Raw)
	}
	expect(t, cc.do(t, "POST", base+"/jobs/"+jobID+"/status", body(map[string]string{"status": "open"})), 200, "open the job")

	// RG3 scorecard: weights must total 100; two interviews average exactly.
	expectErr(t, cc.do(t, "POST", base+"/scorecards", body(map[string]any{"name": "Bad " + uniq(), "competencies": []map[string]any{{"name": "A", "weight_pct": 50}}})), 400, "SCORECARD_INVALID", "weights")
	sc := cc.do(t, "POST", base+"/scorecards", body(map[string]any{"name": "Backend " + uniq(), "competencies": []map[string]any{
		{"name": "Coding", "weight_pct": 60}, {"name": "Communication", "weight_pct": 40}}}))
	expect(t, sc, 201, "scorecard")
	expect(t, cc.do(t, "PUT", base+"/jobs/"+jobID+"/scorecard", body(map[string]any{"scorecard_id": sc.str("data.id")})), 200, "assign scorecard")

	c := cc.addCandidate(t, jobID, "rp."+uniq()+"@example.com")
	expect(t, c, 201, "candidate")
	cand := c.str("data.id")
	cc.stage(t, cand, "screening", "interview")
	iv := cc.do(t, "POST", base+"/interviews", body(map[string]any{"candidate_id": cand, "round_name": "Tech",
		"interviewer_employee_id": cc.memberEmp, "scheduled_at": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339), "duration_mins": 60}))
	expect(t, iv, 201, "interview")
	ivID := iv.str("data.id")
	scores := body(map[string]any{"scores": []map[string]any{{"competency": "Coding", "score": 4}, {"competency": "communication", "score": 3}}})
	expectErr(t, cc.do(t, "PUT", base+"/interviews/"+ivID+"/scores", scores), 404, "NOT_FOUND", "admin is not the interviewer")
	expect(t, cc.as(t, cc.memberTok, "PUT", base+"/interviews/"+ivID+"/scores", scores), 200, "scores")
	card := cc.do(t, "GET", base+"/candidates/"+cand+"/scorecard")
	if expect(t, card, 200, "candidate scorecard"); num(card, "data.overall") != 360 {
		t.Fatalf("RG3 overall 4ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â60%% + 3ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â40%% = 3.80: %s", card.Raw)
	}

	// RG4 offer approval: two approvals required above the threshold; the maker cannot approve.
	expect(t, cc.do(t, "PUT", base+"/offer-policy", body(map[string]any{"required_approvals": 2, "threshold_ctc": 1000000})), 200, "policy")
	off := cc.makeOffer(t, cand)
	expect(t, off, 201, "offer")
	if off.str("data.status") != "pending_approval" {
		t.Fatalf("RG4 offer waits for approval: %s", off.Raw)
	}
	oid := off.str("data.id")
	expectErr(t, cc.makeOffer(t, cand), 409, "OFFER_EXISTS", "pending counts as live")
	expectErr(t, cc.do(t, "POST", base+"/offers/"+oid+"/decision", body(map[string]string{"decision": "accepted"})), 409, "OFFER_STATE", "cannot accept before approval")
	expectErr(t, cc.do(t, "POST", base+"/offers/"+oid+"/approve"), 403, "RECRUITMENT_SELF_APPROVAL", "maker approves own offer")
	one := cc.as(t, approver2, "POST", base+"/offers/"+oid+"/approve")
	expect(t, one, 200, "first approval")
	if one.str("data.status") != "pending_approval" {
		t.Fatalf("RG4 one of two: %s", one.Raw)
	}
	expectErr(t, cc.as(t, approver2, "POST", base+"/offers/"+oid+"/approve"), 409, "ALREADY_DECIDED", "same approver twice")
	second, _, _ := cc.limitedUser(t, [2]string{"recruitment", "approve"}, [2]string{"recruitment", "read"})
	done := cc.as(t, second, "POST", base+"/offers/"+oid+"/approve")
	expect(t, done, 200, "second approval")
	if done.str("data.status") != "offered" {
		t.Fatalf("RG4 two of two releases the offer: %s", done.Raw)
	}
	expect(t, cc.do(t, "POST", base+"/offers/"+oid+"/decision", body(map[string]string{"decision": "accepted"})), 200, "accept")

	// RG1 two hires racing for one seat: exactly one wins, the loser leaves no user behind and the seat is accounted for.
	race := cc.openJob(t, 1, nil)
	var racers [2]string
	for i := range racers {
		rc := cc.addCandidate(t, race, "race"+uniq()+"@example.com")
		expect(t, rc, 201, "racer")
		racers[i] = rc.str("data.id")
		cc.stage(t, racers[i], "screening", "interview")
		o := cc.makeOffer(t, racers[i])
		expect(t, o, 201, "racer offer")
		if o.str("data.status") == "pending_approval" {
			for _, tok := range []string{approver2, second} {
				expect(t, cc.as(t, tok, "POST", base+"/offers/"+o.str("data.id")+"/approve"), 200, "racer approval")
			}
		}
		expect(t, cc.do(t, "POST", base+"/offers/"+o.str("data.id")+"/decision", body(map[string]string{"decision": "accepted"})), 200, "racer accept")
	}
	var wg sync.WaitGroup
	codes := make([]int, len(racers))
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := cc.do(t, "POST", base+"/candidates/"+racers[i]+"/hire", body(map[string]string{"employee_code": "RG-" + uniq()}))
			codes[i] = r.Status
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, s := range codes {
		if s == 200 {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("RG1 exactly one hire wins the last seat: %v", codes)
	}
	if s := sqlStr(t, "SELECT hired_count||'/'||reserved_count||'/'||status FROM recruitment_jobs WHERE id = ?", race); s != "1/0/filled" {
		t.Fatalf("RG1 seat accounting: %s", s)
	}
	// The loser has no reservation, and releasing again is a no-op.
	for i, id := range racers {
		if codes[i] != 200 {
			expect(t, cc.do(t, "POST", base+"/candidates/"+id+"/hire/release"), 204, "release is idempotent")
		}
	}

	// RG5 events and audit carry ids only: no scores, notes, justification or amounts.
	if n := sqlStr(t, "SELECT count(*) FROM recruitment_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%grow the team%' OR payload::text LIKE '%1800000%' OR payload::text LIKE '%Coding%' OR payload::text LIKE '%solid%')", cc.id); n != "0" {
		t.Fatalf("RG5 events leak content (%s rows)", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'recruitment.%' AND (metadata::text LIKE '%grow the team%' OR metadata::text LIKE '%1800000%' OR metadata::text LIKE '%Coding%')", cc.id); n != "0" {
		t.Fatalf("RG5 audit leaks content (%s rows)", n)
	}
}
