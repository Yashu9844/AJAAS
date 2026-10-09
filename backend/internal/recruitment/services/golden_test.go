package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/events"
)

var bg = context.Background()

// G4 — RC-001: candidates only on open jobs.
func TestGolden_G4_ClosedJob(t *testing.T) {
	h := newHarness()
	draft := must(h.jobs.Create(bg, h.actor, dto.CreateJobRequest{Title: "QA", Headcount: 1, EmploymentType: "contract", Description: "x"}))
	_, err := h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: draft.ID.String(), FirstName: "A", LastName: "B", Email: "a@x.io", Source: "other"})
	wantErr(t, err, ErrJobNotOpen)
	must(h.jobs.SetStatus(bg, h.actor, draft.ID, "closed"))
	_, err = h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: draft.ID.String(), FirstName: "A", LastName: "B", Email: "a@x.io", Source: "other"})
	wantErr(t, err, ErrJobNotOpen)
}

// G5 — FR-CD001: duplicate email per job is case-insensitive; another job is fine.
func TestGolden_G5_DuplicateEmail(t *testing.T) {
	h := newHarness()
	j1, j2 := h.openJob(t, 2), h.openJob(t, 2)
	c := h.candidate(t, j1.ID, "Asha@Example.com")
	if c.Email != "asha@example.com" {
		t.Fatalf("email not normalized: %s", c.Email)
	}
	_, err := h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: j1.ID.String(), FirstName: "A", LastName: "R", Email: "ASHA@example.COM", Source: "linkedin"})
	wantErr(t, err, ErrDuplicateEmail)
	h.candidate(t, j2.ID, "asha@example.com")
	other := h.candidate(t, j1.ID, "ravi@example.com")
	email := "ASHA@example.com"
	_, err = h.cands.Update(bg, h.actor, other.ID, dto.UpdateCandidateRequest{Email: &email})
	wantErr(t, err, ErrDuplicateEmail)
	same := "asha@EXAMPLE.com"
	must(h.cands.Update(bg, h.actor, c.ID, dto.UpdateCandidateRequest{Email: &same}))
}

// G6 — RC-003: every stage change appends an event with actor; RC-002 rejects shortcuts.
func TestGolden_G6_StageHistory(t *testing.T) {
	h := newHarness()
	c := h.candidate(t, h.openJob(t, 1).ID, "a@x.io")
	_, err := h.cands.MoveStage(bg, h.actor, c.ID, dto.StageRequest{Stage: "interview"})
	wantErr(t, err, ErrInvalidStage)
	note := "strong CV"
	must(h.cands.MoveStage(bg, h.actor, c.ID, dto.StageRequest{Stage: "screening", Note: &note}))
	h.toStage(t, c.ID, "interview", "rejected")
	got := must(h.cands.Get(bg, h.actor.TenantID, c.ID))
	want := []string{"applied", "screening", "interview", "rejected"}
	if got.Stage != "rejected" || len(got.History) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, e := range got.History {
		if e.ToStage != want[i] || e.ActorUserID != h.actor.UserID || (i > 0 && *e.FromStage != want[i-1]) {
			t.Fatalf("event %d: %+v", i, e)
		}
	}
	if *got.History[1].Note != note || got.History[0].FromStage != nil {
		t.Fatal("note/from")
	}
	_, err = h.cands.MoveStage(bg, h.actor, c.ID, dto.StageRequest{Stage: "screening"})
	wantErr(t, err, ErrInvalidStage)
}

// G7 — RC-004: interviews only in stage interview, only in the future, only with working interviewers.
func TestGolden_G7_InterviewGate(t *testing.T) {
	h := newHarness()
	c := h.candidate(t, h.openJob(t, 1).ID, "a@x.io")
	iv := h.emps.add(h.actor.TenantID, "active")
	req := dto.ScheduleInterviewRequest{CandidateID: c.ID.String(), RoundName: "Tech", InterviewerEmployeeID: iv.ID.String(),
		ScheduledAt: h.now.Add(24 * time.Hour), DurationMins: 60}
	h.toStage(t, c.ID, "screening")
	_, err := h.ivs.Schedule(bg, h.actor, req)
	wantErr(t, err, ErrInvalidStage)
	h.toStage(t, c.ID, "interview")
	past := req
	past.ScheduledAt = h.now
	_, err = h.ivs.Schedule(bg, h.actor, past)
	wantCode(t, err, "VALIDATION_ERROR")
	gone := req
	gone.InterviewerEmployeeID = h.emps.add(h.actor.TenantID, "terminated").ID.String()
	_, err = h.ivs.Schedule(bg, h.actor, gone)
	wantErr(t, err, ErrInterviewerState)
	must(h.ivs.Schedule(bg, h.actor, req))
	if len(h.pub.keys) == 0 || h.pub.keys[len(h.pub.keys)-1] != events.InterviewScheduled {
		t.Fatalf("events %v", h.pub.keys)
	}
}

// G8 — RC-005: only the assigned interviewer gives feedback, once.
func TestGolden_G8_FeedbackOwnership(t *testing.T) {
	h := newHarness()
	c := h.candidate(t, h.openJob(t, 1).ID, "a@x.io")
	h.toStage(t, c.ID, "screening", "interview")
	mine, other := h.emps.add(h.actor.TenantID, "active"), h.emps.add(h.actor.TenantID, "active")
	iv := must(h.ivs.Schedule(bg, h.actor, dto.ScheduleInterviewRequest{CandidateID: c.ID.String(), RoundName: "Tech",
		InterviewerEmployeeID: mine.ID.String(), ScheduledAt: h.now.Add(time.Hour), DurationMins: 45}))
	fb := dto.FeedbackRequest{Rating: 4, Recommendation: "hire"}
	_, err := h.ivs.Feedback(bg, Actor{TenantID: h.actor.TenantID, UserID: other.UserID}, iv.ID, fb)
	wantErr(t, err, ErrNotFound)
	me := Actor{TenantID: h.actor.TenantID, UserID: mine.UserID}
	done := must(h.ivs.Feedback(bg, me, iv.ID, fb))
	if done.Status != "completed" || *done.Rating != 4 || done.FeedbackAt == nil {
		t.Fatalf("%+v", done)
	}
	_, err = h.ivs.Feedback(bg, me, iv.ID, fb)
	wantErr(t, err, ErrInterviewState)
	list, meta := must2(h.ivs.ListMine(bg, me, "completed", dto.Page{Page: 1, PerPage: 20}))
	if len(list) != 1 || meta.TotalItems != 1 {
		t.Fatal("ListMine")
	}
	_, err = h.ivs.Cancel(bg, h.actor, iv.ID)
	wantErr(t, err, ErrInterviewState)
}

// G9 — RC-006: one open offer; a declined offer frees the slot.
func TestGolden_G9_OneOpenOffer(t *testing.T) {
	h := newHarness()
	c := h.candidate(t, h.openJob(t, 1).ID, "a@x.io")
	h.toStage(t, c.ID, "screening")
	_, err := h.offs.Create(bg, h.actor, dto.CreateOfferRequest{CandidateID: c.ID.String(), OfferedCTC: 1, JoiningDate: "2026-11-01"})
	wantErr(t, err, ErrInvalidStage)
	h.toStage(t, c.ID, "interview")
	first := h.offer(t, c.ID)
	if got := must(h.cands.Get(bg, h.actor.TenantID, c.ID)); got.Stage != "offer" {
		t.Fatalf("stage %s", got.Stage)
	}
	_, err = h.offs.Create(bg, h.actor, dto.CreateOfferRequest{CandidateID: c.ID.String(), OfferedCTC: 1, JoiningDate: "2026-11-01"})
	wantErr(t, err, ErrOfferExists)
	must(h.offs.Decide(bg, h.actor, first.ID, "declined"))
	_, err = h.offs.Decide(bg, h.actor, first.ID, "accepted")
	wantErr(t, err, ErrOfferState)
	second := h.offer(t, c.ID)
	must(h.offs.Decide(bg, h.actor, second.ID, "accepted"))
	_, err = h.offs.Create(bg, h.actor, dto.CreateOfferRequest{CandidateID: c.ID.String(), OfferedCTC: 1, JoiningDate: "2026-11-01"})
	wantErr(t, err, ErrOfferExists)
}

// G10 — FR-HR001/RC-008: hire invites, creates the employee, marks hired, counts, fills at headcount.
func TestGolden_G10_Hire(t *testing.T) {
	h := newHarness()
	job := h.openJob(t, 2)
	c1, c2, c3 := h.accepted(t, job.ID, "a@x.io"), h.accepted(t, job.ID, "b@x.io"), h.accepted(t, job.ID, "c@x.io")
	declined := h.candidate(t, job.ID, "d@x.io")
	h.toStage(t, declined.ID, "screening", "interview")
	must(h.offs.Decide(bg, h.actor, h.offer(t, declined.ID).ID, "declined"))
	_, err := h.hire.Hire(bg, h.actor, declined.ID, dto.HireRequest{EmployeeCode: "EMP-9"})
	wantErr(t, err, ErrNotHireable)

	r1 := must(h.hire.Hire(bg, h.actor, c1, dto.HireRequest{EmployeeCode: "EMP-1"}))
	if r1.JobStatus != "open" || len(h.users.invites) != 1 || h.users.invites[0].Email != "a@x.io" {
		t.Fatalf("%+v %+v", r1, h.users.invites)
	}
	req := h.emps.created[0]
	if req.UserID != r1.UserID || req.EmploymentType != "full_time" || req.JoiningDate.Format("2006-01-02") != "2026-11-02" {
		t.Fatalf("employee request %+v", req)
	}
	r2 := must(h.hire.Hire(bg, h.actor, c2, dto.HireRequest{EmployeeCode: "EMP-2", EmploymentType: "contract"}))
	if r2.JobStatus != "filled" || h.emps.created[1].EmploymentType != "contract" {
		t.Fatalf("%+v", r2)
	}
	j := must(h.jobs.Get(bg, h.actor.TenantID, job.ID))
	if j.HiredCount != 2 || j.Status != "filled" || j.ClosedAt == nil {
		t.Fatalf("%+v", j)
	}
	got := must(h.cands.Get(bg, h.actor.TenantID, c1))
	if got.Stage != "hired" || *got.HiredEmployeeID != r1.EmployeeID {
		t.Fatalf("%+v", got)
	}
	_, err = h.hire.Hire(bg, h.actor, c3, dto.HireRequest{EmployeeCode: "EMP-3"})
	wantErr(t, err, ErrJobNotOpen)
	_, err = h.hire.Hire(bg, h.actor, c1, dto.HireRequest{EmployeeCode: "EMP-1"})
	wantErr(t, err, ErrNotHireable)
	_, err = h.jobs.SetStatus(bg, h.actor, job.ID, "open")
	wantErr(t, err, ErrJobState)
}

// G11 — RC-010: a failure after the invite is retried with the same user and one profile.
func TestGolden_G11_HireIdempotency(t *testing.T) {
	h := newHarness()
	c := h.accepted(t, h.openJob(t, 1).ID, "a@x.io")
	h.emps.failNew = errors.New("employee code taken")
	_, err := h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "EMP-1"})
	if err == nil || len(h.users.invites) != 1 {
		t.Fatal("first attempt must fail after the invite")
	}
	h.emps.failNew = nil
	h.st.fail["outbox.create"] = errors.New("db blip")
	_, err = h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "EMP-1"})
	if err == nil || len(h.emps.created) != 1 {
		t.Fatal("second attempt must fail after the employee exists")
	}
	delete(h.st.fail, "outbox.create")
	r := must(h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "EMP-1"}))
	if len(h.users.invites) != 1 || len(h.emps.created) != 1 || r.JobStatus != "filled" {
		t.Fatalf("invites %d employees %d %+v", len(h.users.invites), len(h.emps.created), r)
	}
}

// G12 (unit half) — RC-011: events and audit carry no PII, CTC or feedback.
func TestGolden_G12_NoPIIInEventsOrAudit(t *testing.T) {
	h := newHarness()
	c := h.accepted(t, h.openJob(t, 1).ID, "secret.person@x.io")
	must(h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "EMP-1"}))
	var blob strings.Builder
	for _, e := range h.st.outbox {
		blob.WriteString(e.Payload)
	}
	raw, _ := json.Marshal(h.audit.entries)
	blob.Write(raw)
	for _, banned := range []string{"secret.person", "Asha", "Rao", "1200000", "offered_ctc"} {
		if strings.Contains(blob.String(), banned) {
			t.Fatalf("leaked %q", banned)
		}
	}
	if len(h.st.outbox) != 3 {
		t.Fatalf("want job_published + candidate_applied + candidate_hired, got %d", len(h.st.outbox))
	}
}

func must2[A any, B any](a A, b B, err error) (A, B) {
	if err != nil {
		panic(err)
	}
	return a, b
}
