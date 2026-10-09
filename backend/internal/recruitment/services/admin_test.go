package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

var page1 = dto.Page{Page: 1, PerPage: 20}

func TestJobs_UpdateListAndOrgRefs(t *testing.T) {
	h := newHarness()
	dept, desig := uuid.New(), uuid.New()
	h.org.ids[dept], h.org.ids[desig] = true, true
	ds, gs, bad, junk := dept.String(), desig.String(), uuid.NewString(), "nope"
	j := must(h.jobs.Create(bg, h.actor, dto.CreateJobRequest{Title: "SRE", DepartmentID: &ds, DesignationID: &gs, Headcount: 2,
		EmploymentType: "full_time", MinExperienceYears: ptr(3), Description: "x"}))
	if j.Status != "draft" || *j.DepartmentID != dept || j.MinExperienceYears != 3 {
		t.Fatalf("%+v", j)
	}
	_, err := h.jobs.Create(bg, h.actor, dto.CreateJobRequest{Title: "x", DepartmentID: &bad, Headcount: 1, EmploymentType: "intern", Description: "x"})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = h.jobs.Create(bg, h.actor, dto.CreateJobRequest{Title: "x", DesignationID: &junk, Headcount: 1, EmploymentType: "intern", Description: "x"})
	wantCode(t, err, "VALIDATION_ERROR")
	h.org.err = errors.New("org down")
	_, err = h.jobs.Update(bg, h.actor, j.ID, dto.UpdateJobRequest{DepartmentID: &ds})
	wantErr(t, err, h.org.err)
	h.org.err = nil

	title, loc, typ, desc, hc := "Senior SRE", "Pune", "contract", "y", 3
	u := must(h.jobs.Update(bg, h.actor, j.ID, dto.UpdateJobRequest{Title: &title, Location: &loc, EmploymentType: &typ, Description: &desc,
		Headcount: &hc, DepartmentID: &ds, DesignationID: &gs, MinExperienceYears: ptr(5)}))
	if u.Title != title || *u.Location != loc || u.Headcount != 3 || u.EmploymentType != typ || u.MinExperienceYears != 5 {
		t.Fatalf("%+v", u)
	}
	_, err = h.jobs.Update(bg, h.actor, uuid.New(), dto.UpdateJobRequest{Title: &title})
	wantErr(t, err, ErrNotFound)
	must(h.jobs.SetStatus(bg, h.actor, j.ID, "open"))
	must(h.jobs.SetStatus(bg, h.actor, j.ID, "on_hold"))
	reopened := must(h.jobs.SetStatus(bg, h.actor, j.ID, "open"))
	if reopened.OpenedAt == nil || countKey(h, "recruitment.job_published") != 1 {
		t.Fatal("job_published must fire on first open only")
	}
	c := h.accepted(t, j.ID, "a@x.io")
	must(h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "E1"}))
	one := 0
	_, err = h.jobs.Update(bg, h.actor, j.ID, dto.UpdateJobRequest{Headcount: &one})
	wantCode(t, err, "VALIDATION_ERROR")
	must(h.jobs.SetStatus(bg, h.actor, j.ID, "closed"))
	_, err = h.jobs.Update(bg, h.actor, j.ID, dto.UpdateJobRequest{Title: &title})
	wantErr(t, err, ErrJobState)

	must(h.jobs.Create(bg, h.actor, dto.CreateJobRequest{Title: "Other", Headcount: 1, EmploymentType: "intern", Description: "x"}))
	list, meta := must2(h.jobs.List(bg, h.actor.TenantID, repositories.JobFilter{Status: "closed", DepartmentID: &dept}, page1))
	if len(list) != 1 || meta.TotalItems != 1 || list[0].ID != j.ID {
		t.Fatalf("%+v", meta)
	}
	if _, err := h.jobs.Get(bg, uuid.New(), j.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant job must 404 (RC-009)")
	}
}

func TestCandidates_ListUpdateAndOfferExit(t *testing.T) {
	h := newHarness()
	job := h.openJob(t, 1)
	c := h.candidate(t, job.ID, "a@x.io")
	neg := calc.Money(-1)
	_, err := h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: job.ID.String(), FirstName: "x", LastName: "y", Email: "n@x.io", Source: "other", ExpectedCTC: &neg})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = h.cands.Update(bg, h.actor, c.ID, dto.UpdateCandidateRequest{ExpectedCTC: &neg})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: "bad", FirstName: "x", LastName: "y", Email: "n@x.io", Source: "other"})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = h.cands.Create(bg, h.actor, dto.CreateCandidateRequest{JobID: uuid.NewString(), FirstName: "x", LastName: "y", Email: "n@x.io", Source: "other"})
	wantErr(t, err, ErrNotFound)

	fn, ln, ph, url, ctc, np := "Ana", "Roy", "+91", "https://cv.example/a", calc.Rupees(900000), 30
	u := must(h.cands.Update(bg, h.actor, c.ID, dto.UpdateCandidateRequest{FirstName: &fn, LastName: &ln, Phone: &ph, ResumeURL: &url, ExpectedCTC: &ctc, NoticePeriodDays: &np}))
	if u.FirstName != fn || *u.Phone != ph || *u.ExpectedCTC != ctc || *u.NoticePeriodDays != 30 {
		t.Fatalf("%+v", u)
	}
	h.toStage(t, c.ID, "screening", "interview")
	o := h.offer(t, c.ID)
	h.toStage(t, c.ID, "withdrawn")
	offers, _ := must2(h.offs.List(bg, h.actor.TenantID, c.ID, page1))
	if offers[0].ID != o.ID || offers[0].Status != "withdrawn" {
		t.Fatalf("leaving offer stage must withdraw the open offer: %+v", offers)
	}
	list, meta := must2(h.cands.List(bg, h.actor.TenantID, repositories.CandidateFilter{JobID: &job.ID, Stage: "withdrawn"}, page1))
	if len(list) != 1 || meta.TotalItems != 1 {
		t.Fatal("candidate list filter")
	}
	_, err = h.cands.MoveStage(bg, h.actor, uuid.New(), dto.StageRequest{Stage: "screening"})
	wantErr(t, err, ErrNotFound)
	_, err = h.cands.Get(bg, uuid.New(), c.ID)
	wantErr(t, err, ErrNotFound)

	hired := h.accepted(t, h.openJob(t, 1).ID, "h@x.io")
	must(h.hire.Hire(bg, h.actor, hired, dto.HireRequest{EmployeeCode: "E1"}))
	_, err = h.cands.Update(bg, h.actor, hired, dto.UpdateCandidateRequest{FirstName: &fn})
	wantErr(t, err, ErrInvalidStage)
}

func TestInterviews_ListCancelAndLookups(t *testing.T) {
	h := newHarness()
	c := h.candidate(t, h.openJob(t, 1).ID, "a@x.io")
	h.toStage(t, c.ID, "screening", "interview")
	emp := h.emps.add(h.actor.TenantID, "probation")
	req := dto.ScheduleInterviewRequest{CandidateID: c.ID.String(), RoundName: "HR", InterviewerEmployeeID: emp.ID.String(), ScheduledAt: h.now.Add(time.Hour), DurationMins: 30}
	iv := must(h.ivs.Schedule(bg, h.actor, req))
	list, _ := must2(h.ivs.ListByCandidate(bg, h.actor.TenantID, c.ID, page1))
	if len(list) != 1 || list[0].ID != iv.ID {
		t.Fatal("ListByCandidate")
	}
	if got := must(h.ivs.Cancel(bg, h.actor, iv.ID)); got.Status != "cancelled" {
		t.Fatal("cancel")
	}
	_, err := h.ivs.Cancel(bg, h.actor, uuid.New())
	wantErr(t, err, ErrNotFound)
	for _, bad := range []dto.ScheduleInterviewRequest{{CandidateID: "x", InterviewerEmployeeID: emp.ID.String()}, {CandidateID: c.ID.String(), InterviewerEmployeeID: "x"}} {
		_, err = h.ivs.Schedule(bg, h.actor, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	unknown := req
	unknown.InterviewerEmployeeID = uuid.NewString()
	_, err = h.ivs.Schedule(bg, h.actor, unknown)
	wantErr(t, err, ErrEmployeeNotFound)
	_, _, err = h.ivs.ListMine(bg, Actor{TenantID: h.actor.TenantID, UserID: uuid.New()}, "", page1)
	wantErr(t, err, ErrEmployeeNotFound)
	h.emps.err = errors.New("m2 down")
	_, err = h.ivs.Schedule(bg, h.actor, req)
	wantErr(t, err, h.emps.err)
	_, err = h.ivs.Feedback(bg, h.actor, iv.ID, dto.FeedbackRequest{Rating: 3, Recommendation: "hold"})
	wantErr(t, err, h.emps.err)
}

func TestOffers_ValidationWithdrawAndExpiry(t *testing.T) {
	h := newHarness()
	c := h.candidate(t, h.openJob(t, 1).ID, "a@x.io")
	h.toStage(t, c.ID, "screening", "interview")
	base := dto.CreateOfferRequest{CandidateID: c.ID.String(), OfferedCTC: calc.Rupees(100000), JoiningDate: "2026-11-01"}
	str := func(s string) *string { return &s }
	for name, mut := range map[string]func(*dto.CreateOfferRequest){
		"candidate":   func(r *dto.CreateOfferRequest) { r.CandidateID = "x" },
		"ctc":         func(r *dto.CreateOfferRequest) { r.OfferedCTC = 0 },
		"joining":     func(r *dto.CreateOfferRequest) { r.JoiningDate = "2026-10-08" },
		"expires bad": func(r *dto.CreateOfferRequest) { r.ExpiresOn = str("soon") },
		"expires late": func(r *dto.CreateOfferRequest) {
			r.ExpiresOn = str("2026-11-02")
		},
	} {
		r := base
		mut(&r)
		if _, err := h.offs.Create(bg, h.actor, r); err == nil {
			t.Errorf("%s: want validation error", name)
		}
	}
	r := base
	r.ExpiresOn = str("2026-10-20")
	o := must(h.offs.Create(bg, h.actor, r))
	if *o.ExpiresOn != "2026-10-20" || o.JoiningDate != "2026-11-01" {
		t.Fatalf("%+v", o)
	}
	h.now = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	_, err := h.offs.Decide(bg, h.actor, o.ID, "accepted")
	wantErr(t, err, ErrOfferState)
	if w := must(h.offs.Withdraw(bg, h.actor, o.ID)); w.Status != "withdrawn" || w.DecidedAt == nil {
		t.Fatalf("%+v", w)
	}
	_, err = h.offs.Withdraw(bg, h.actor, uuid.New())
	wantErr(t, err, ErrNotFound)
	_, err = h.offs.Create(bg, h.actor, dto.CreateOfferRequest{CandidateID: uuid.NewString(), OfferedCTC: 1, JoiningDate: "2026-12-01"})
	wantErr(t, err, ErrNotFound)
}

func TestHire_PortFailures(t *testing.T) {
	h := newHarness()
	c := h.accepted(t, h.openJob(t, 1).ID, "a@x.io")
	h.users.err = errors.New("email already registered")
	_, err := h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "E1"})
	wantErr(t, err, h.users.err)
	h.users.err, h.users.badID = nil, true
	if _, err = h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "E1"}); err == nil {
		t.Fatal("bad user id must fail")
	}
	h.users.badID = false
	h.emps.err = errors.New("m2 down")
	_, err = h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "E1"})
	wantErr(t, err, h.emps.err)
	h.emps.err = nil
	_, err = h.hire.Hire(bg, h.actor, uuid.New(), dto.HireRequest{EmployeeCode: "E1"})
	wantErr(t, err, ErrNotFound)
	if got := must(h.hire.Hire(bg, h.actor, c, dto.HireRequest{EmployeeCode: "E1"})); got.JobStatus != "filled" {
		t.Fatal("hire after transient failures")
	}
}

func TestOutboxRelay(t *testing.T) {
	h := newHarness()
	h.pub.fail = true
	h.openJob(t, 1)
	relay := NewOutboxRelay(h.deps)
	if n, _ := relay.RelayOnce(bg); n != 0 {
		t.Fatal("broker down publishes nothing")
	}
	for _, e := range h.st.outbox {
		if e.Attempts != 1 || e.LastError == nil {
			t.Fatalf("%+v", e)
		}
	}
	h.pub.fail = false
	if n, _ := relay.RelayOnce(bg); n != 1 {
		t.Fatal("relay must publish the backlog")
	}
	if n, _ := relay.RelayOnce(bg); n != 0 {
		t.Fatal("published rows are not resent")
	}
	h.st.fail["outbox.fetch"] = errors.New("db down")
	if _, err := relay.RelayOnce(bg); err == nil {
		t.Fatal("fetch error")
	}
	ctx, cancel := context.WithCancel(bg)
	relay.interval = time.Millisecond
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	relay.Run(ctx)
}

func TestHelpers(t *testing.T) {
	if err := orNotFound(nil); !errors.Is(err, ErrNotFound) {
		t.Fatal("orNotFound(nil)")
	}
	if id, err := parseOptionalUUID("x", nil); id != nil || err != nil {
		t.Fatal("nil optional")
	}
	if runner := NewTxRunner(nil); runner.DB() != nil {
		t.Fatal("DB")
	}
	var ae *sharedErrors.AppError
	if !errors.As(invalid("f", "m"), &ae) || ae.StatusCode != 400 {
		t.Fatal("invalid")
	}
}

func countKey(h *harness, key string) int {
	n := 0
	for _, k := range h.pub.keys {
		if k == key {
			n++
		}
	}
	return n
}

func ptr[T any](v T) *T { return &v }
