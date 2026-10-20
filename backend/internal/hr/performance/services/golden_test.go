package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
)

// G4 — weight cap: 60 + 40 fit, +0.01 does not; cancelling frees the weight; re-weighting is checked too.
func TestGolden_G4_WeightCap(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	me := e.as(e.alice)
	g60 := e.goal(me, cy, e.alice, "60")
	e.goal(me, cy, e.alice, "40")
	_, err := e.goals.Create(ctx, me, dto.CreateGoalRequest{CycleID: cy.String(), Title: "One more", Weight: fx("0.01")})
	wantCode(t, err, "WEIGHT_EXCEEDED")
	_, err = e.goals.SetStatus(ctx, me, g60.ID, "cancelled")
	must(t, err)
	e.goal(me, cy, e.alice, "0.01")
	g := e.goal(me, cy, e.alice, "50")
	_, err = e.goals.Update(ctx, me, g.ID, dto.UpdateGoalRequest{Weight: fxp("60")})
	wantCode(t, err, "WEIGHT_EXCEEDED")
	_, err = e.goals.Update(ctx, me, g.ID, dto.UpdateGoalRequest{Weight: fxp("59.99"), Title: sp(" Renamed "), Description: sp("why")})
	must(t, err)
	_, err = e.goals.SetStatus(ctx, me, g60.ID, "active")
	wantCode(t, err, "GOAL_STATE") // cancelled is final
}

// G5 — key results drive progress (PM-006); direct progress on such a goal is refused; every change is a check-in.
func TestGolden_G5_KeyResultProgress(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	me := e.as(e.alice)
	g := e.goal(me, cy, e.alice, "50")
	got, err := e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{Progress: fxp("30"), Note: sp("halfway-ish")})
	must(t, err)
	if got.Progress != fx("30") || len(got.CheckIns) != 1 || got.CheckIns[0].FromProgress != 0 {
		t.Fatalf("direct progress: %+v", got)
	}
	got, err = e.goals.AddKeyResult(ctx, me, g.ID, dto.KeyResultRequest{Title: "Latency", Unit: sp("ms"), StartValue: fx("50"), TargetValue: fx("10")})
	must(t, err)
	if got.Progress != 0 || len(got.KeyResults) != 1 {
		t.Fatalf("goal progress follows its key results: %+v", got)
	}
	kr := got.KeyResults[0].ID.String()
	got, err = e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{KeyResultID: &kr, Value: fxp("30")})
	must(t, err)
	if got.Progress != fx("50") || got.KeyResults[0].CurrentValue != fx("30") {
		t.Fatalf("EC-02 falling target: %+v", got)
	}
	got, err = e.goals.AddKeyResult(ctx, me, g.ID, dto.KeyResultRequest{Title: "Coverage", StartValue: fx("0"), TargetValue: fx("80")})
	must(t, err)
	kr2 := got.KeyResults[1].ID.String()
	got, err = e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{KeyResultID: &kr2, Value: fxp("120")})
	must(t, err)
	if got.Progress != fx("75") { // mean(50, clamp 100)
		t.Fatalf("mean with clamp: %v", got.Progress)
	}
	_, err = e.goals.CheckIn(ctx, me, g.ID, dto.CheckInRequest{Progress: fxp("90")})
	wantCode(t, err, "HAS_KEY_RESULTS")
	other := e.goal(me, cy, e.alice, "10")
	_, err = e.goals.CheckIn(ctx, me, other.ID, dto.CheckInRequest{KeyResultID: &kr, Value: fxp("1")})
	wantCode(t, err, "NOT_FOUND")
	for _, bad := range []dto.CheckInRequest{{}, {Progress: fxp("1"), KeyResultID: &kr, Value: fxp("1")}, {KeyResultID: &kr}, {Progress: fxp("101")}} {
		_, err = e.goals.CheckIn(ctx, me, g.ID, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	if n := len(e.st.checkins); n != 5 {
		t.Fatalf("check-ins recorded: %d", n)
	}
	got, err = e.goals.Get(ctx, me, g.ID)
	must(t, err)
	if len(got.CheckIns) != 5 || got.CheckIns[0].ToProgress != fx("75") {
		t.Fatalf("history newest first: %+v", got.CheckIns)
	}
}

// G6 — PM-008 access: owner and manager write, performance:read reads, a colleague sees nothing.
func TestGolden_G6_Access(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	g := e.goal(e.as(e.alice), cy, e.alice, "40")
	if _, err := e.goals.Update(ctx, e.as(e.mgr), g.ID, dto.UpdateGoalRequest{Title: sp("Set by manager")}); err != nil {
		t.Fatal("manager writes a report's goal:", err)
	}
	e.goal(e.as(e.mgr), cy, e.bob, "20") // a manager creates for a report
	_, err := e.goals.Get(ctx, e.as(e.carol), g.ID)
	wantCode(t, err, "NOT_FOUND")
	_, err = e.goals.Update(ctx, e.as(e.carol), g.ID, dto.UpdateGoalRequest{Title: sp("x")})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.goals.Create(ctx, e.as(e.carol), dto.CreateGoalRequest{CycleID: cy.String(), EmployeeID: sp(e.alice.ID.String()), Title: "x", Weight: fx("1")})
	wantCode(t, err, "NOT_FOUND")
	reader := e.as(e.carol)
	reader.CanRead = true
	_, err = e.goals.Get(ctx, reader, g.ID)
	must(t, err)
	_, err = e.goals.CheckIn(ctx, reader, g.ID, dto.CheckInRequest{Progress: fxp("5")})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.goals.CheckIn(ctx, e.admin(), g.ID, dto.CheckInRequest{Progress: fxp("5")})
	must(t, err)

	_, _, err = e.goals.List(ctx, e.as(e.carol), repositories.GoalFilter{EmployeeID: &e.alice.ID}, dto.ParsePage("", ""))
	wantCode(t, err, "NOT_FOUND")
	rows, _, err := e.goals.List(ctx, e.as(e.mgr), repositories.GoalFilter{EmployeeID: &e.alice.ID}, dto.ParsePage("", ""))
	if must(t, err); len(rows) != 1 {
		t.Fatalf("manager lists a report's goals: %d", len(rows))
	}
	rows, _, err = e.goals.List(ctx, e.as(e.carol), repositories.GoalFilter{}, dto.ParsePage("", ""))
	if must(t, err); len(rows) != 0 {
		t.Fatalf("default filter is self: %d", len(rows))
	}
	rows, meta, err := e.goals.List(ctx, reader, repositories.GoalFilter{CycleID: &cy}, dto.ParsePage("1", "1"))
	if must(t, err); len(rows) != 1 || meta.TotalItems != 2 {
		t.Fatalf("reader sees all, paged: %d %+v", len(rows), meta)
	}
	mine, _, err := e.goals.ListMine(ctx, e.as(e.bob), &cy, dto.ParsePage("", ""))
	if must(t, err); len(mine) != 1 {
		t.Fatalf("my goals: %d", len(mine))
	}
	_, _, err = e.goals.List(ctx, reader, repositories.GoalFilter{Status: "bogus"}, dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
	stranger := Actor{TenantID: e.tenant, UserID: uuid.New()}
	_, _, err = e.goals.ListMine(ctx, stranger, nil, dto.ParsePage("", ""))
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
}

// G7 — cycle status locks goals and check-ins (PM-003, PM-007).
func TestGolden_G7_CycleLocks(t *testing.T) {
	e := newEnv(t)
	draft := e.cycle("draft")
	g := e.goal(e.as(e.alice), draft, e.alice, "10")
	_, err := e.goals.CheckIn(ctx, e.as(e.alice), g.ID, dto.CheckInRequest{Progress: fxp("5")})
	wantCode(t, err, "CYCLE_STATE")

	cy := e.cycle("active")
	g = e.goal(e.as(e.alice), cy, e.alice, "10")
	_, err = e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	_, err = e.goals.Create(ctx, e.as(e.alice), dto.CreateGoalRequest{CycleID: cy.String(), Title: "late", Weight: fx("1")})
	wantCode(t, err, "CYCLE_STATE")
	_, err = e.goals.Update(ctx, e.as(e.alice), g.ID, dto.UpdateGoalRequest{Title: sp("late")})
	wantCode(t, err, "CYCLE_STATE")
	_, err = e.goals.AddKeyResult(ctx, e.as(e.alice), g.ID, dto.KeyResultRequest{Title: "kr", StartValue: 0, TargetValue: fx("1")})
	wantCode(t, err, "CYCLE_STATE")
	_, err = e.goals.CheckIn(ctx, e.as(e.alice), g.ID, dto.CheckInRequest{Progress: fxp("80")})
	must(t, err) // check-ins stay open in review
	_, err = e.cycles.Complete(ctx, e.admin(), cy)
	must(t, err)
	_, err = e.goals.CheckIn(ctx, e.as(e.alice), g.ID, dto.CheckInRequest{Progress: fxp("90")})
	wantCode(t, err, "CYCLE_STATE")

	exited := e.ppl.add(e.tenant, "exited")
	_, err = e.goals.Create(ctx, e.admin(), dto.CreateGoalRequest{CycleID: draft.String(), EmployeeID: sp(exited.ID.String()), Title: "x", Weight: fx("1")})
	wantCode(t, err, "EMPLOYEE_STATE")
}

// G8 — launch reviews: self for every goal owner, manager where one exists, idempotent (PM-010, EC-06).
func TestGolden_G8_LaunchReviews(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	e.goal(e.as(e.alice), cy, e.alice, "10")
	e.goal(e.as(e.bob), cy, e.bob, "10")
	e.goal(e.as(e.carol), cy, e.carol, "10") // carol has no manager
	e.goal(e.as(e.alice), cy, e.alice, "20") // a second goal does not add reviews
	out, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	if out.Employees != 3 || out.SelfReviews != 3 || out.ManagerReviews != 2 || out.WithoutManager != 1 || out.Cycle.Status != "review" {
		t.Fatalf("launch: %+v", out)
	}
	again, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	if again.SelfReviews+again.ManagerReviews != 0 || len(e.st.reviews) != 5 {
		t.Fatalf("EC-06 idempotent: %+v (%d reviews)", again, len(e.st.reviews))
	}
	assigned, _, err := e.reviews.ListAssigned(ctx, e.as(e.mgr), "pending", dto.ParsePage("", ""))
	if must(t, err); len(assigned) != 2 {
		t.Fatalf("manager has two reviews to write: %d", len(assigned))
	}
	_, err = e.cycles.LaunchReviews(ctx, e.admin(), e.cycle("draft"))
	wantCode(t, err, "CYCLE_STATE")
}

// G9 — submit: only the reviewer, exact criteria, potential rules, immutable (PM-012).
func TestGolden_G9_Submit(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	e.goal(e.as(e.alice), cy, e.alice, "10")
	_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	self, mgr := e.reviewOf(e.alice, models.ReviewSelf), e.reviewOf(e.alice, models.ReviewManager)
	full := models.Ratings{{Criterion: "quality", Score: 4}, {Criterion: "Delivery", Score: 3}, {Criterion: "Collaboration", Score: 5}, {Criterion: "Ownership", Score: 4}}

	_, err = e.reviews.Submit(ctx, e.as(e.carol), self, dto.SubmitReviewRequest{Ratings: full})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.reviews.Submit(ctx, e.as(e.alice), self, dto.SubmitReviewRequest{Ratings: full[:3]})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.reviews.Submit(ctx, e.as(e.alice), self, dto.SubmitReviewRequest{Ratings: append(full[:3:3], models.Rating{Criterion: "Speed", Score: 3})})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.reviews.Submit(ctx, e.as(e.alice), self, dto.SubmitReviewRequest{Ratings: full, Potential: sp("high")})
	wantCode(t, err, "VALIDATION_ERROR")
	got, err := e.reviews.Submit(ctx, e.as(e.alice), self, dto.SubmitReviewRequest{Ratings: full, Strengths: sp("ownership")})
	must(t, err)
	if got.Status != "submitted" || *got.OverallScore != fx("4") || got.Ratings[0].Criterion != "Quality" {
		t.Fatalf("self review: %+v", got)
	}
	_, err = e.reviews.Submit(ctx, e.as(e.alice), self, dto.SubmitReviewRequest{Ratings: full})
	wantCode(t, err, "REVIEW_STATE")
	_, err = e.reviews.Submit(ctx, e.as(e.mgr), mgr, dto.SubmitReviewRequest{Ratings: full})
	wantCode(t, err, "VALIDATION_ERROR")
	got, err = e.reviews.Submit(ctx, e.as(e.mgr), mgr, dto.SubmitReviewRequest{Ratings: full, Potential: sp("medium")})
	must(t, err)
	if *got.Potential != "medium" {
		t.Fatalf("manager review: %+v", got)
	}
	_, err = e.reviews.Submit(ctx, Actor{TenantID: e.tenant, UserID: uuid.New()}, mgr, dto.SubmitReviewRequest{Ratings: full})
	wantCode(t, err, "NOT_FOUND")
}

// G10 — subjects see their submitted reviews only after completion; peers stay anonymous (PM-013, PM-014, EC-08/09).
func TestGolden_G10_SubjectVisibility(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	e.goal(e.as(e.alice), cy, e.alice, "10")
	_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	peer, err := e.reviews.Assign(ctx, e.admin(), dto.AssignReviewerRequest{CycleID: cy.String(), EmployeeID: e.alice.ID.String(),
		ReviewerEmployeeID: e.carol.ID.String(), Type: "peer"})
	must(t, err)
	ratings := models.Ratings{{Criterion: "Quality", Score: 2}, {Criterion: "Delivery", Score: 2}, {Criterion: "Collaboration", Score: 2}, {Criterion: "Ownership", Score: 3}}
	_, err = e.reviews.Submit(ctx, e.as(e.carol), peer.ID, dto.SubmitReviewRequest{Ratings: ratings, Improvements: sp("documentation")})
	must(t, err)
	mine, err := e.reviews.ListMine(ctx, e.as(e.alice), nil)
	if must(t, err); len(mine) != 0 {
		t.Fatalf("EC-09 nothing before completion: %d", len(mine))
	}
	_, err = e.cycles.Complete(ctx, e.admin(), cy)
	must(t, err)
	mine, err = e.reviews.ListMine(ctx, e.as(e.alice), &cy)
	must(t, err)
	if len(mine) != 1 || mine[0].ReviewerEmployeeID != nil || *mine[0].OverallScore != fx("2.25") {
		t.Fatalf("EC-09 one anonymous peer review: %+v", mine)
	}
	_, err = e.reviews.Submit(ctx, e.as(e.alice), e.reviewOf(e.alice, models.ReviewSelf), dto.SubmitReviewRequest{Ratings: ratings})
	wantCode(t, err, "REVIEW_STATE") // EC-08: expired on completion
	if r := e.st.reviews[e.reviewOf(e.alice, models.ReviewManager)]; r.Status != models.ReviewExpired {
		t.Fatalf("PM-013 pending → expired: %s", r.Status)
	}
	_, err = e.reviews.Get(ctx, e.as(e.alice), peer.ID)
	wantCode(t, err, "NOT_FOUND") // the subject reads through /reviews/me only
	got, err := e.reviews.Get(ctx, e.as(e.carol), peer.ID)
	if must(t, err); got.ReviewerEmployeeID == nil {
		t.Fatal("the reviewer sees their own review in full")
	}
}

// G11 — 9-box: band × potential from the latest manager review; unrated listed; goal score attached (PM-015).
func TestGolden_G11_NineBox(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	ga := e.goal(e.as(e.alice), cy, e.alice, "60")
	e.goal(e.as(e.alice), cy, e.alice, "40")
	e.goal(e.as(e.bob), cy, e.bob, "10")
	_, err := e.goals.CheckIn(ctx, e.as(e.alice), ga.ID, dto.CheckInRequest{Progress: fxp("50")})
	must(t, err)
	_, err = e.cycles.NineBox(ctx, e.tenant, cy)
	wantCode(t, err, "CYCLE_STATE")
	_, err = e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	ratings := models.Ratings{{Criterion: "Quality", Score: 4}, {Criterion: "Delivery", Score: 4}, {Criterion: "Collaboration", Score: 3}, {Criterion: "Ownership", Score: 3}}
	_, err = e.reviews.Submit(ctx, e.as(e.mgr), e.reviewOf(e.alice, models.ReviewManager), dto.SubmitReviewRequest{Ratings: ratings, Potential: sp("high")})
	must(t, err)
	box, err := e.cycles.NineBox(ctx, e.tenant, cy)
	must(t, err)
	byEmp := map[uuid.UUID]dto.NineBoxEntry{}
	for _, en := range box.Entries {
		byEmp[en.EmployeeID] = en
	}
	if a := byEmp[e.alice.ID]; a.Box != "high/high" || a.GoalScore != fx("30") || *a.OverallScore != fx("3.5") {
		t.Fatalf("alice: %+v", a)
	}
	if b := byEmp[e.bob.ID]; b.Box != "unrated" || b.OverallScore != nil {
		t.Fatalf("bob: %+v", b)
	}
	if box.Counts["unrated"] != 1 || box.Counts["high/high"] != 1 {
		t.Fatalf("counts: %v", box.Counts)
	}
}

// G12 (unit) — events and audit carry ids and statuses only (PM-017).
func TestGolden_G12_Privacy(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	e.goal(e.as(e.alice), cy, e.alice, "10")
	_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(t, err)
	ratings := models.Ratings{{Criterion: "Quality", Score: 5}, {Criterion: "Delivery", Score: 5}, {Criterion: "Collaboration", Score: 5}, {Criterion: "Ownership", Score: 5}}
	_, err = e.reviews.Submit(ctx, e.as(e.mgr), e.reviewOf(e.alice, models.ReviewManager), dto.SubmitReviewRequest{Ratings: ratings,
		Potential: sp("high"), Strengths: sp("SECRET-STRENGTH"), Improvements: sp("SECRET-GAP")})
	must(t, err)
	for _, row := range e.st.outbox {
		for _, leak := range []string{"SECRET", "ratings", "overall", "potential", "\"score\""} {
			if strings.Contains(row.Payload, leak) {
				t.Fatalf("event %s leaks %q: %s", row.RoutingKey, leak, row.Payload)
			}
		}
	}
	for _, r := range e.aud.rows {
		if s := strings.ToLower(fmt.Sprintf("%v", r)); strings.Contains(s, "secret") || strings.Contains(s, "potential") || strings.Contains(s, "overall") {
			t.Fatalf("audit leaks: %s", s)
		}
	}
	if len(e.pub.sent) == 0 {
		t.Fatal("events are published after commit")
	}
}

// G15 — cascading: same cycle, not cancelled, depth ≤ 3 (PM-005, EC-10).
func TestGolden_G15_Cascading(t *testing.T) {
	e := newEnv(t)
	cy := e.cycle("active")
	admin := e.admin()
	mk := func(parent *dto.GoalResponse, owner string) (*dto.GoalResponse, error) {
		req := dto.CreateGoalRequest{CycleID: cy.String(), EmployeeID: sp(owner), Title: "level", Weight: fx("1")}
		if parent != nil {
			req.ParentGoalID = sp(parent.ID.String())
		}
		return e.goals.Create(ctx, admin, req)
	}
	l1, err := mk(nil, e.mgr.ID.String())
	must(t, err)
	l2, err := mk(l1, e.alice.ID.String())
	must(t, err)
	l3, err := mk(l2, e.bob.ID.String())
	must(t, err)
	_, err = mk(l3, e.carol.ID.String())
	wantCode(t, err, "GOAL_DEPTH")
	other := e.goal(admin, e.cycle("active"), e.alice, "1")
	_, err = mk(other, e.alice.ID.String())
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.goals.SetStatus(ctx, admin, l1.ID, "cancelled")
	must(t, err)
	_, err = mk(l1, e.alice.ID.String())
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.goals.Create(ctx, admin, dto.CreateGoalRequest{CycleID: cy.String(), EmployeeID: sp(e.alice.ID.String()), Title: "x", Weight: fx("1"),
		ParentGoalID: sp(uuid.NewString())})
	wantCode(t, err, "NOT_FOUND")
}
