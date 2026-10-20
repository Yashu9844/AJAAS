package services

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
)

// ratedCycle runs a cycle to review with alice's manager review submitted (3.50, medium).
func (e *env) ratedCycle() uuid.UUID {
	e.t.Helper()
	cy := e.cycle("active")
	e.goal(e.as(e.alice), cy, e.alice, "10")
	e.goal(e.as(e.bob), cy, e.bob, "10")
	_, err := e.cycles.LaunchReviews(ctx, e.admin(), cy)
	must(e.t, err)
	ratings := models.Ratings{{Criterion: "Quality", Score: 4}, {Criterion: "Delivery", Score: 4}, {Criterion: "Collaboration", Score: 3}, {Criterion: "Ownership", Score: 3}}
	_, err = e.reviews.Submit(ctx, e.as(e.mgr), e.reviewOf(e.alice, models.ReviewManager), dto.SubmitReviewRequest{Ratings: ratings, Potential: sp("medium")})
	must(e.t, err)
	return cy
}

func (e *env) calibrate(cy uuid.UUID, emp uuid.UUID, score, potential *string, reason string) (*dto.CalibrationResponse, error) {
	req := dto.CalibrateRequest{EmployeeID: emp.String(), Potential: potential, Reason: reason}
	if score != nil {
		req.OverallScore = fxp(*score)
	}
	return e.cals.Calibrate(ctx, e.admin(), cy, req)
}

// G16 — calibration: only in review, needs a manager review, never a no-op, append-only, drives the 9-box.
func TestGolden_G16_Calibration(t *testing.T) {
	e := newEnv(t)
	active := e.cycle("active")
	_, err := e.calibrate(active, e.alice.ID, sp("4"), nil, "cross-team alignment")
	wantCode(t, err, "CYCLE_STATE")
	cy := e.ratedCycle()
	_, err = e.calibrate(cy, e.bob.ID, sp("4"), nil, "cross-team alignment")
	wantCode(t, err, "NOT_RATED")
	_, err = e.calibrate(cy, e.alice.ID, sp("3.5"), sp("medium"), "nothing really changes")
	wantCode(t, err, "VALIDATION_ERROR")
	first, err := e.calibrate(cy, e.alice.ID, sp("4.25"), nil, "calibration session Q2: delivery underrated")
	must(t, err)
	if first.BeforeScore != fx("3.5") || first.BeforePotential != "medium" || first.AfterScore != fx("4.25") || first.AfterPotential != "medium" {
		t.Fatalf("before/after: %+v", first)
	}
	second, err := e.calibrate(cy, e.alice.ID, nil, sp("high"), "leadership shown in incident review")
	must(t, err)
	if second.BeforeScore != fx("4.25") || second.AfterScore != fx("4.25") || second.AfterPotential != "high" {
		t.Fatalf("second builds on the first: %+v", second)
	}
	hist, meta, err := e.cals.List(ctx, e.tenant, cy, &e.alice.ID, dto.ParsePage("", ""))
	if must(t, err); meta.TotalItems != 2 || hist[0].ID != second.ID {
		t.Fatalf("append-only, newest first: %+v", hist)
	}
	box, err := e.cycles.NineBox(ctx, e.tenant, cy)
	must(t, err)
	for _, en := range box.Entries {
		if en.EmployeeID == e.alice.ID && (en.Box != "high/high" || !en.Calibrated || *en.OverallScore != fx("4.25")) {
			t.Fatalf("9-box uses the final rating: %+v", en)
		}
		if en.EmployeeID == e.bob.ID && (en.Box != "unrated" || en.Calibrated) {
			t.Fatalf("bob: %+v", en)
		}
	}
	if r := e.st.reviews[e.reviewOf(e.alice, models.ReviewManager)]; *r.OverallScore != fx("3.5") || *r.Potential != "medium" {
		t.Fatal("the manager review itself is never edited (D7-10)")
	}
	_, err = e.cycles.Complete(ctx, e.admin(), cy)
	must(t, err)
	_, err = e.calibrate(cy, e.alice.ID, sp("5"), nil, "too late for this cycle")
	wantCode(t, err, "CYCLE_STATE")
}

// Calibration input rules (FR-CA001).
func TestCalibration_Validation(t *testing.T) {
	e := newEnv(t)
	cy := e.ratedCycle()
	for _, bad := range []dto.CalibrateRequest{
		{EmployeeID: "x", OverallScore: fxp("4"), Reason: "long enough reason"},
		{EmployeeID: e.alice.ID.String(), OverallScore: fxp("4"), Reason: "short"},
		{EmployeeID: e.alice.ID.String(), Reason: "long enough reason"},
		{EmployeeID: e.alice.ID.String(), OverallScore: fxp("0.99"), Reason: "long enough reason"},
		{EmployeeID: e.alice.ID.String(), OverallScore: fxp("5.01"), Reason: "long enough reason"},
		{EmployeeID: e.alice.ID.String(), Potential: sp("stellar"), Reason: "long enough reason"},
		{EmployeeID: e.alice.ID.String(), OverallScore: fxp("4"), Reason: strings.Repeat("x", 1001)},
	} {
		_, err := e.cals.Calibrate(ctx, e.admin(), cy, bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err := e.calibrate(uuid.New(), e.alice.ID, sp("4"), nil, "long enough reason")
	wantCode(t, err, "NOT_FOUND")
	_, _, err = e.cals.List(ctx, e.tenant, uuid.New(), nil, dto.ParsePage("", ""))
	wantCode(t, err, "NOT_FOUND")
}

// G17 — after completion the subject sees the final rating (calibrated) but never the reason or the calibrator.
func TestGolden_G17_FinalRatingForSubject(t *testing.T) {
	e := newEnv(t)
	cy := e.ratedCycle()
	_, err := e.calibrate(cy, e.alice.ID, sp("4.5"), sp("high"), "SECRET-REASON from the session")
	must(t, err)
	_, err = e.cycles.Complete(ctx, e.admin(), cy)
	must(t, err)
	mine, err := e.reviews.ListMine(ctx, e.as(e.alice), &cy)
	must(t, err)
	if len(mine) != 1 || mine[0].Final == nil || mine[0].Final.OverallScore != fx("4.5") || mine[0].Final.Potential != "high" || !mine[0].Final.Calibrated {
		t.Fatalf("final rating: %+v", mine)
	}
	if strings.Contains(fmt.Sprintf("%+v", mine), "SECRET") {
		t.Fatal("the calibration reason never reaches the subject")
	}

	e2 := newEnv(t)
	cy2 := e2.ratedCycle()
	_, err = e2.cycles.Complete(ctx, e2.admin(), cy2)
	must(t, err)
	mine, err = e2.reviews.ListMine(ctx, e2.as(e2.alice), nil)
	must(t, err)
	if mine[0].Final == nil || mine[0].Final.Calibrated || mine[0].Final.OverallScore != fx("3.5") {
		t.Fatalf("uncalibrated final = manager review: %+v", mine[0].Final)
	}
}

func (e *env) give(from, to string, kind, vis, msg string, request *string) (*dto.FeedbackResponse, error) {
	var giver Actor
	for _, p := range []struct {
		id string
		a  Actor
	}{{e.alice.ID.String(), e.as(e.alice)}, {e.bob.ID.String(), e.as(e.bob)}, {e.carol.ID.String(), e.as(e.carol)}, {e.mgr.ID.String(), e.as(e.mgr)}} {
		if p.id == from {
			giver = p.a
		}
	}
	return e.feedback.Give(ctx, giver, dto.GiveFeedbackRequest{ToEmployeeID: to, Kind: kind, Visibility: vis, Message: msg, RequestID: request})
}

// G18 — feedback visibility (FB-001, FB-002).
func TestGolden_G18_FeedbackVisibility(t *testing.T) {
	e := newEnv(t)
	_, err := e.give(e.carol.ID.String(), e.alice.ID.String(), "constructive", "private", "Please share the design doc earlier", nil)
	must(t, err)
	_, err = e.give(e.bob.ID.String(), e.alice.ID.String(), "praise", "public", "Great incident handling!", nil)
	must(t, err)
	_, err = e.give(e.bob.ID.String(), e.alice.ID.String(), "constructive", "public", "x", nil)
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.give(e.alice.ID.String(), e.alice.ID.String(), "praise", "private", "me", nil)
	wantCode(t, err, "VALIDATION_ERROR")
	exited := e.ppl.add(e.tenant, "exited")
	_, err = e.give(e.bob.ID.String(), exited.ID.String(), "praise", "private", "bye", nil)
	wantCode(t, err, "EMPLOYEE_STATE")

	recv, _, err := e.feedback.ListReceived(ctx, e.as(e.alice), dto.ParsePage("", ""))
	if must(t, err); len(recv) != 2 {
		t.Fatalf("recipient sees both: %d", len(recv))
	}
	given, _, err := e.feedback.ListGiven(ctx, e.as(e.carol), dto.ParsePage("", ""))
	if must(t, err); len(given) != 1 {
		t.Fatalf("giver sees theirs: %d", len(given))
	}
	mgr, _, err := e.feedback.ListFor(ctx, e.as(e.mgr), e.alice.ID, dto.ParsePage("", ""))
	if must(t, err); len(mgr) != 2 {
		t.Fatalf("manager sees the report's feedback: %d", len(mgr))
	}
	_, _, err = e.feedback.ListFor(ctx, e.as(e.carol), e.alice.ID, dto.ParsePage("", ""))
	wantCode(t, err, "NOT_FOUND")
	reader := e.as(e.carol)
	reader.CanRead = true
	_, _, err = e.feedback.ListFor(ctx, reader, e.alice.ID, dto.ParsePage("", ""))
	must(t, err)
	wall, _, err := e.feedback.ListPublic(ctx, e.tenant, dto.ParsePage("", ""))
	if must(t, err); len(wall) != 1 || wall[0].Kind != "praise" {
		t.Fatalf("praise wall: %+v", wall)
	}
}

// G19 — feedback requests (FB-004, FB-005).
func TestGolden_G19_FeedbackRequests(t *testing.T) {
	e := newEnv(t)
	ask := func(a Actor, about *string, responders ...string) ([]dto.FeedbackRequestResponse, error) {
		return e.feedback.Request(ctx, a, dto.RequestFeedbackRequest{ResponderEmployeeIDs: responders, AboutEmployeeID: about, Question: "How did the launch go?"})
	}
	reqs, err := ask(e.as(e.alice), nil, e.bob.ID.String(), e.carol.ID.String())
	if must(t, err); len(reqs) != 2 || reqs[0].AboutEmployeeID != e.alice.ID {
		t.Fatalf("two requests about alice: %+v", reqs)
	}
	_, err = ask(e.as(e.alice), nil, e.bob.ID.String())
	wantCode(t, err, "CONFLICT")
	_, err = ask(e.as(e.alice), nil, e.alice.ID.String())
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = ask(e.as(e.alice), nil, e.mgr.ID.String(), e.mgr.ID.String())
	wantCode(t, err, "VALIDATION_ERROR")
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = e.ppl.add(e.tenant, "active").ID.String()
	}
	_, err = ask(e.as(e.alice), nil, eleven...)
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = ask(e.as(e.carol), sp(e.alice.ID.String()), e.bob.ID.String())
	wantCode(t, err, "NOT_FOUND") // carol does not manage alice
	mreq, err := ask(e.as(e.mgr), sp(e.alice.ID.String()), e.carol.ID.String())
	if must(t, err); mreq[0].AboutEmployeeID != e.alice.ID {
		t.Fatalf("manager asks about a report: %+v", mreq)
	}

	bobReq := reqs[0].ID.String()
	if reqs[0].ResponderEmployeeID != e.bob.ID {
		bobReq = reqs[1].ID.String()
	}
	_, err = e.give(e.carol.ID.String(), e.alice.ID.String(), "praise", "private", "answering someone else's", &bobReq)
	wantCode(t, err, "NOT_FOUND")
	_, err = e.give(e.bob.ID.String(), e.carol.ID.String(), "praise", "private", "wrong person", &bobReq)
	wantCode(t, err, "VALIDATION_ERROR")
	fb, err := e.give(e.bob.ID.String(), e.alice.ID.String(), "constructive", "private", "Smooth launch; add a rollback plan", &bobReq)
	must(t, err)
	if r := e.st.requests[uuid.MustParse(bobReq)]; r.Status != models.RequestAnswered || *r.FeedbackID != fb.ID {
		t.Fatalf("answered and linked: %+v", r)
	}
	_, err = e.give(e.bob.ID.String(), e.alice.ID.String(), "praise", "private", "again", &bobReq)
	wantCode(t, err, "REQUEST_STATE")

	assigned, _, err := e.feedback.ListAssigned(ctx, e.as(e.carol), "pending", dto.ParsePage("", ""))
	if must(t, err); len(assigned) != 2 {
		t.Fatalf("carol has two to answer: %d", len(assigned))
	}
	_, err = e.feedback.Decline(ctx, e.as(e.bob), assigned[0].ID)
	wantCode(t, err, "NOT_FOUND")
	d, err := e.feedback.Decline(ctx, e.as(e.carol), assigned[0].ID)
	if must(t, err); d.Status != "declined" {
		t.Fatalf("declined: %+v", d)
	}
	_, err = e.feedback.Decline(ctx, e.as(e.carol), assigned[0].ID)
	wantCode(t, err, "REQUEST_STATE")
	mine, _, err := e.feedback.ListMine(ctx, e.as(e.alice), "", dto.ParsePage("", ""))
	if must(t, err); len(mine) != 2 {
		t.Fatalf("alice's requests: %d", len(mine))
	}
	_, _, err = e.feedback.ListMine(ctx, e.as(e.alice), "closed", dto.ParsePage("", ""))
	wantCode(t, err, "VALIDATION_ERROR")
}

// G20 — no message, question, reason, scores or potential in events or audit (PM-022, FB-006).
func TestGolden_G20_ParityPrivacy(t *testing.T) {
	e := newEnv(t)
	cy := e.ratedCycle()
	_, err := e.calibrate(cy, e.alice.ID, sp("4.5"), sp("high"), "SECRET-REASON from the session")
	must(t, err)
	reqs, err := e.feedback.Request(ctx, e.as(e.alice), dto.RequestFeedbackRequest{ResponderEmployeeIDs: []string{e.bob.ID.String()}, Question: "SECRET-QUESTION?"})
	must(t, err)
	rid := reqs[0].ID.String()
	_, err = e.give(e.bob.ID.String(), e.alice.ID.String(), "praise", "public", "SECRET-MESSAGE", &rid)
	must(t, err)
	for _, row := range e.st.outbox {
		for _, leak := range []string{"SECRET", "4.50", "\"high\"", "potential", "score", "message", "question", "reason"} {
			if strings.Contains(row.Payload, leak) {
				t.Fatalf("event %s leaks %q: %s", row.RoutingKey, leak, row.Payload)
			}
		}
	}
	for _, r := range e.aud.rows {
		if s := fmt.Sprintf("%v", r); strings.Contains(s, "SECRET") || strings.Contains(s, "4.5") {
			t.Fatalf("audit leaks: %s", s)
		}
	}
	keys := map[string]bool{}
	for _, row := range e.st.outbox {
		keys[row.RoutingKey] = true
	}
	for _, k := range []string{"performance.rating_calibrated", "performance.feedback_requested", "performance.feedback_given"} {
		if !keys[k] {
			t.Fatalf("missing event %s", k)
		}
	}
}

// Feedback input rules and identity errors.
func TestFeedback_Validation(t *testing.T) {
	e := newEnv(t)
	stranger := Actor{TenantID: e.tenant, UserID: uuid.New()}
	for _, bad := range []dto.GiveFeedbackRequest{
		{ToEmployeeID: "x", Kind: "praise", Visibility: "private", Message: "m"},
		{ToEmployeeID: e.bob.ID.String(), Kind: "praise", Visibility: "private", Message: " "},
		{ToEmployeeID: e.bob.ID.String(), Kind: "praise", Visibility: "private", Message: strings.Repeat("m", 2001)},
		{ToEmployeeID: e.bob.ID.String(), Kind: "praise", Visibility: "private", Message: "m", RequestID: sp("x")},
	} {
		_, err := e.feedback.Give(ctx, e.as(e.alice), bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err := e.feedback.Give(ctx, e.as(e.alice), dto.GiveFeedbackRequest{ToEmployeeID: uuid.NewString(), Kind: "praise", Visibility: "private", Message: "m"})
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.feedback.Give(ctx, stranger, dto.GiveFeedbackRequest{ToEmployeeID: e.bob.ID.String(), Kind: "praise", Visibility: "private", Message: "m"})
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	exited := e.ppl.add(e.tenant, "exited")
	_, err = e.feedback.Give(ctx, e.as(exited), dto.GiveFeedbackRequest{ToEmployeeID: e.bob.ID.String(), Kind: "praise", Visibility: "private", Message: "m"})
	wantCode(t, err, "EMPLOYEE_STATE")
	_, err = e.feedback.Give(ctx, e.as(e.alice), dto.GiveFeedbackRequest{ToEmployeeID: e.bob.ID.String(), Kind: "praise", Visibility: "private", Message: "m",
		RequestID: sp(uuid.NewString())})
	wantCode(t, err, "NOT_FOUND")
	for _, bad := range []dto.RequestFeedbackRequest{
		{ResponderEmployeeIDs: []string{e.bob.ID.String()}, Question: " "},
		{ResponderEmployeeIDs: []string{}, Question: "q"},
		{ResponderEmployeeIDs: []string{"x"}, Question: "q"},
		{ResponderEmployeeIDs: []string{e.bob.ID.String()}, Question: "q", AboutEmployeeID: sp("x")},
	} {
		_, err = e.feedback.Request(ctx, e.as(e.alice), bad)
		wantCode(t, err, "VALIDATION_ERROR")
	}
	_, err = e.feedback.Request(ctx, e.as(e.alice), dto.RequestFeedbackRequest{ResponderEmployeeIDs: []string{exited.ID.String()}, Question: "q"})
	wantCode(t, err, "EMPLOYEE_STATE")
	_, err = e.feedback.Request(ctx, stranger, dto.RequestFeedbackRequest{ResponderEmployeeIDs: []string{e.bob.ID.String()}, Question: "q"})
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	for _, call := range []func() error{
		func() error { _, _, err := e.feedback.ListReceived(ctx, stranger, dto.ParsePage("", "")); return err },
		func() error { _, _, err := e.feedback.ListGiven(ctx, stranger, dto.ParsePage("", "")); return err },
		func() error {
			_, _, err := e.feedback.ListAssigned(ctx, stranger, "", dto.ParsePage("", ""))
			return err
		},
	} {
		wantCode(t, call(), "EMPLOYEE_NOT_FOUND")
	}
	_, err = e.feedback.Decline(ctx, stranger, uuid.New())
	wantCode(t, err, "NOT_FOUND")
}

// Parity infrastructure failures propagate and roll back.
func TestFailures_Parity(t *testing.T) {
	cases := []struct {
		fail string
		run  func(e *env, cy uuid.UUID) error
	}{
		{"cals.create", func(e *env, cy uuid.UUID) error {
			_, err := e.calibrate(cy, e.alice.ID, sp("4"), nil, "long enough reason")
			return err
		}},
		{"cals.latest", func(e *env, cy uuid.UUID) error {
			_, err := e.calibrate(cy, e.alice.ID, sp("4"), nil, "long enough reason")
			return err
		}},
		{"reviews.list", func(e *env, cy uuid.UUID) error {
			_, err := e.calibrate(cy, e.alice.ID, sp("4"), nil, "long enough reason")
			return err
		}},
		{"cycles.lock", func(e *env, cy uuid.UUID) error {
			_, err := e.calibrate(cy, e.alice.ID, sp("4"), nil, "long enough reason")
			return err
		}},
		{"outbox.create", func(e *env, cy uuid.UUID) error {
			_, err := e.calibrate(cy, e.alice.ID, sp("4"), nil, "long enough reason")
			return err
		}},
		{"cals.list", func(e *env, cy uuid.UUID) error {
			_, _, err := e.cals.List(ctx, e.tenant, cy, nil, dto.ParsePage("", ""))
			return err
		}},
		{"cycles.find", func(e *env, cy uuid.UUID) error {
			_, _, err := e.cals.List(ctx, e.tenant, cy, nil, dto.ParsePage("", ""))
			return err
		}},
		{"cals.perEmployee", func(e *env, cy uuid.UUID) error { _, err := e.cycles.NineBox(ctx, e.tenant, cy); return err }},
		{"cals.latest", func(e *env, cy uuid.UUID) error {
			if _, err := e.cycles.Complete(ctx, e.admin(), cy); err != nil {
				return err
			}
			_, err := e.reviews.ListMine(ctx, e.as(e.alice), nil)
			return err
		}},
		{"feedback.create", func(e *env, _ uuid.UUID) error {
			_, err := e.give(e.bob.ID.String(), e.alice.ID.String(), "praise", "private", "m", nil)
			return err
		}},
		{"feedback.list", func(e *env, _ uuid.UUID) error {
			_, _, err := e.feedback.ListPublic(ctx, e.tenant, dto.ParsePage("", ""))
			return err
		}},
		{"requests.exists", func(e *env, _ uuid.UUID) error { return e.askBob() }},
		{"requests.create", func(e *env, _ uuid.UUID) error { return e.askBob() }},
		{"requests.list", func(e *env, _ uuid.UUID) error {
			_, _, err := e.feedback.ListMine(ctx, e.as(e.alice), "", dto.ParsePage("", ""))
			return err
		}},
		{"requests.lock", func(e *env, _ uuid.UUID) error {
			_, err := e.feedback.Decline(ctx, e.as(e.bob), uuid.New())
			return err
		}},
		{"managers", func(e *env, _ uuid.UUID) error {
			_, _, err := e.feedback.ListFor(ctx, e.as(e.carol), e.alice.ID, dto.ParsePage("", ""))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.fail, func(t *testing.T) {
			e := newEnv(t)
			cy := e.ratedCycle()
			e.st.fail[tc.fail] = errBoom
			e.ppl.fail[tc.fail] = errBoom
			if err := tc.run(e, cy); !errors.Is(err, errBoom) {
				t.Fatalf("want boom, got %v", err)
			}
		})
	}
	for _, op := range []string{"requests.update", "outbox.create"} {
		t.Run("answer/"+op, func(t *testing.T) {
			e := newEnv(t)
			must(t, e.askBob())
			var rid string
			for id := range e.st.requests {
				rid = id.String()
			}
			e.st.fail[op] = errBoom
			if _, err := e.give(e.bob.ID.String(), e.alice.ID.String(), "praise", "private", "m", &rid); !errors.Is(err, errBoom) {
				t.Fatalf("want boom, got %v", err)
			}
			if len(e.st.feedback) != 0 || e.st.requests[uuid.MustParse(rid)].Status != models.RequestPending {
				t.Fatal("rolled back")
			}
			if op == "requests.update" {
				if _, err := e.feedback.Decline(ctx, e.as(e.bob), uuid.MustParse(rid)); !errors.Is(err, errBoom) {
					t.Fatalf("decline: want boom, got %v", err)
				}
			}
		})
	}
}

func (e *env) askBob() error {
	_, err := e.feedback.Request(ctx, e.as(e.alice), dto.RequestFeedbackRequest{ResponderEmployeeIDs: []string{e.bob.ID.String()}, Question: "q"})
	return err
}
