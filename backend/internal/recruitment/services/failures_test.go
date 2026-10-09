package services

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/repositories"
)

// TestRepositoryFailures — every repository error propagates and rolls the transaction back.
func TestRepositoryFailures(t *testing.T) {
	boom := errors.New("db down")
	for _, op := range []string{"job.create", "job.update", "job.find", "job.lock", "job.list", "candidate.create", "candidate.update",
		"candidate.find", "candidate.lock", "candidate.email", "candidate.list", "candidate.event", "candidate.events", "interview.create",
		"interview.update", "interview.find", "interview.list", "offer.create", "offer.update", "offer.find", "offer.latest", "offer.list", "outbox.create"} {
		t.Run(op, func(t *testing.T) {
			h := newHarness()
			emp := h.emps.add(h.actor.TenantID, "active")
			job := h.openJob(t, 2)
			c := h.candidate(t, job.ID, "a@x.io")
			h.toStage(t, c.ID, "screening", "interview")
			iv := must(h.ivs.Schedule(bg, h.actor, dto.ScheduleInterviewRequest{CandidateID: c.ID.String(), RoundName: "R",
				InterviewerEmployeeID: emp.ID.String(), ScheduledAt: h.now.Add(time.Hour), DurationMins: 30}))
			o := h.offer(t, c.ID)
			fresh := h.candidate(t, job.ID, "b@x.io")
			h.toStage(t, fresh.ID, "screening", "interview")
			before := len(h.st.events)
			h.st.fail[op] = boom
			errs := runAll(h, fixture{job: job.ID, cand: c.ID, fresh: fresh.ID, iv: ivID(iv), offer: o.ID, interviewer: emp})
			if !anyIs(errs, boom) {
				t.Fatalf("%s: no operation surfaced the failure", op)
			}
			delete(h.st.fail, op)
			if op == "candidate.event" && len(h.st.events) != before {
				t.Fatal("failed stage event must roll back")
			}
		})
	}
}

// fixture names the rows runAll exercises; fresh is an interview-stage candidate with no offer.
type fixture struct {
	job, cand, fresh, iv, offer uuid.UUID
	interviewer                 *employeeDTO.EmployeeResponse
}

func ivID(iv *dto.InterviewResponse) uuid.UUID { return iv.ID }

func runAll(h *harness, f fixture) []error {
	jobID, candID, ivID, offerID, interviewerUser := f.job, f.cand, f.iv, f.offer, f.interviewer.UserID
	title := "t"
	email := "z@x.io"
	var errs []error
	add := func(_ interface{}, err error) { errs = append(errs, err) }
	add2 := func(_, _ interface{}, err error) { errs = append(errs, err) }
	add(h.jobs.Create(bg, h.actor, dto.CreateJobRequest{Title: "x", Headcount: 1, EmploymentType: "intern", Description: "x"}))
	add(h.jobs.Get(bg, h.actor.TenantID, jobID))
	add2(h.jobs.List(bg, h.actor.TenantID, repositories.JobFilter{}, page1))
	add(h.jobs.Update(bg, h.actor, jobID, dto.UpdateJobRequest{Title: &title}))
	add(h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: jobID.String(), FirstName: "x", LastName: "y", Email: "q@x.io", Source: "other"}))
	add(h.cands.Get(bg, h.actor.TenantID, candID))
	add2(h.cands.List(bg, h.actor.TenantID, repositories.CandidateFilter{}, page1))
	add(h.cands.Update(bg, h.actor, candID, dto.UpdateCandidateRequest{Email: &email}))
	add2(h.ivs.ListByCandidate(bg, h.actor.TenantID, candID, page1))
	add2(h.ivs.ListMine(bg, Actor{TenantID: h.actor.TenantID, UserID: interviewerUser}, "", page1))
	add(h.ivs.Cancel(bg, h.actor, ivID))
	add2(h.offs.List(bg, h.actor.TenantID, candID, page1))
	add(h.offs.Decide(bg, h.actor, offerID, "accepted"))
	add(h.ivs.Schedule(bg, h.actor, dto.ScheduleInterviewRequest{CandidateID: f.fresh.String(), RoundName: "R2",
		InterviewerEmployeeID: f.interviewer.ID.String(), ScheduledAt: h.now.Add(time.Hour), DurationMins: 30}))
	add(h.offs.Create(bg, h.actor, dto.CreateOfferRequest{CandidateID: f.fresh.String(), OfferedCTC: 1, JoiningDate: "2026-12-01"}))
	add(h.hire.Hire(bg, h.actor, candID, dto.HireRequest{EmployeeCode: "E1"}))
	add(h.jobs.SetStatus(bg, h.actor, jobID, "on_hold"))
	add(h.cands.MoveStage(bg, h.actor, candID, dto.StageRequest{Stage: "rejected"}))
	return errs
}

func anyIs(errs []error, target error) bool {
	for _, e := range errs {
		if errors.Is(e, target) {
			return true
		}
	}
	return false
}
