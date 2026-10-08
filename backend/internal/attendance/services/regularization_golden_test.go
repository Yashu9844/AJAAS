package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
)

func regReq(date string, in, out time.Time) dto.CreateRegularizationRequest {
	return dto.CreateRegularizationRequest{AttendanceDate: date, RequestedPunchIn: in, RequestedPunchOut: out, Reason: "forgot to punch out"}
}

// TestGolden_G6_RegularizationLifecycle — protected (G6, AT-017).
func TestGolden_G6_RegularizationLifecycle(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user, approver := uuid.New(), uuid.New()
	h.emps.add(h.tenant, user, "active")
	h.emps.add(h.tenant, approver, "active")

	// Employee punched IN only (forgot OUT) on 2026-10-08.
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	h.advance(24 * time.Hour) // now 2026-10-09 09:00 UTC
	in, out := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 8, 18, 30, 0, 0, time.UTC)
	req, err := svc.Create(ctx, h.actor(user), regReq("2026-10-08", in, out))
	if err != nil || req.Status != models.RegPending {
		t.Fatalf("create: %+v %v", req, err)
	}
	approved, err := svc.Approve(ctx, h.actor(approver), uuid.MustParse(req.ID), dto.ReviewRequest{Comment: strp("ok")})
	if err != nil || approved.Status != models.RegApproved || approved.ReviewerUserID == nil || *approved.ReviewerUserID != approver.String() {
		t.Fatalf("approve: %+v %v", approved, err)
	}
	if len(h.st.records) != 1 {
		t.Fatalf("approval must reuse the existing record, have %d", len(h.st.records))
	}
	for _, rec := range h.st.records {
		if !rec.IsRegularized || rec.TotalWorkMinutes != 570 || rec.Status != models.StatusPresent ||
			!rec.FirstPunchIn.Equal(in) || !rec.LastPunchOut.Equal(out) {
			t.Fatalf("record not rebuilt from requested window: %+v", rec)
		}
	}
	superseded, fresh := 0, 0
	for _, p := range h.st.punches {
		if p.IsSuperseded {
			superseded++
		} else if p.Source == models.SourceRegularization {
			fresh++
		}
	}
	if superseded != 1 || fresh != 2 {
		t.Fatalf("AT-017: superseded=%d regularization punches=%d", superseded, fresh)
	}
	want := []string{"attendance.punch.in", "attendance.regularization.requested", "attendance.regularization.approved"}
	if len(h.pub.keys) != 3 || h.pub.keys[1] != want[1] || h.pub.keys[2] != want[2] {
		t.Fatalf("events = %v", h.pub.keys)
	}
	acts := h.audit.actions()
	if acts[len(acts)-1] != "attendance.regularization_approved" {
		t.Fatalf("audit = %v", acts)
	}
	if _, err := svc.Approve(ctx, h.actor(approver), uuid.MustParse(req.ID), dto.ReviewRequest{}); codeOf(err) != "REGULARIZATION_NOT_PENDING" {
		t.Fatalf("re-approve: %v", err)
	}
}

// TestGolden_G6_RegularizationCreatesMissingRecord covers EC-10.
func TestGolden_G6_RegularizationCreatesMissingRecord(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user, approver := uuid.New(), uuid.New()
	h.emps.add(h.tenant, user, "active")
	in, out := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 13, 0, 0, 0, time.UTC)
	req, err := svc.Create(ctx, h.actor(user), regReq("2026-10-07", in, out))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, h.actor(approver), uuid.MustParse(req.ID), dto.ReviewRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rec := range h.st.records {
		if rec.Source != models.SourceRegularization || rec.Status != models.StatusHalfDay || rec.TotalWorkMinutes != 240 {
			t.Fatalf("EC-10 record: %+v", rec)
		}
	}
}

// TestGolden_G7_SelfApprovalForbidden — protected (G7, AT-016).
func TestGolden_G7_SelfApprovalForbidden(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	req, err := svc.Create(ctx, h.actor(user), regReq("2026-10-07", time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.MustParse(req.ID)
	if _, err := svc.Approve(ctx, h.actor(user), id, dto.ReviewRequest{}); codeOf(err) != "SELF_APPROVAL_FORBIDDEN" {
		t.Fatalf("approve own: %v", err)
	}
	if _, err := svc.Reject(ctx, h.actor(user), id, dto.ReviewRequest{Comment: strp("no")}); codeOf(err) != "SELF_APPROVAL_FORBIDDEN" {
		t.Fatalf("reject own: %v", err)
	}
	if h.st.regs[id].Status != models.RegPending || len(h.st.records) != 0 {
		t.Fatal("G7: request must stay pending and nothing applied")
	}
}

// TestGolden_G8_OnePendingPerDate — protected (G8, AT-015).
func TestGolden_G8_OnePendingPerDate(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	r := regReq("2026-10-07", time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC))
	first, err := svc.Create(ctx, h.actor(user), r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, h.actor(user), r); codeOf(err) != "REGULARIZATION_PENDING" {
		t.Fatalf("EC-11: %v", err)
	}
	if _, err := svc.Cancel(ctx, h.actor(user), uuid.MustParse(first.ID)); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := svc.Create(ctx, h.actor(user), r); err != nil {
		t.Fatalf("after cancel a new request is allowed: %v", err)
	}
}

func TestRegularization_CreateValidation(t *testing.T) {
	h := newHarness() // now 2026-10-08 09:00 UTC
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	d7 := func(hh int) time.Time { return time.Date(2026, 10, 7, hh, 0, 0, 0, time.UTC) }
	cases := map[string]struct {
		req  dto.CreateRegularizationRequest
		code string
	}{
		"bad date":        {regReq("07-10-2026", d7(9), d7(18)), "VALIDATION_ERROR"},
		"future":          {regReq("2026-10-09", time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)), "REGULARIZATION_WINDOW"},
		"older than 30d":  {regReq("2026-09-07", time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 7, 18, 0, 0, 0, time.UTC)), "REGULARIZATION_WINDOW"},
		"out before in":   {regReq("2026-10-07", d7(18), d7(9)), "VALIDATION_ERROR"},
		"span over 20h":   {regReq("2026-10-07", d7(1), d7(1).Add(21*time.Hour)), "VALIDATION_ERROR"},
		"in on other day": {regReq("2026-10-07", time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC), d7(9)), "VALIDATION_ERROR"},
	}
	for name, tc := range cases {
		if _, err := svc.Create(ctx, h.actor(user), tc.req); codeOf(err) != tc.code {
			t.Errorf("%s: got %v want %s", name, err, tc.code)
		}
	}
	if _, err := svc.Create(ctx, h.actor(uuid.New()), regReq("2026-10-07", d7(9), d7(18))); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("no profile: %v", err)
	}
	h.st.fail["reg.create"] = errors.New("db down")
	if _, err := svc.Create(ctx, h.actor(user), regReq("2026-10-07", d7(9), d7(18))); err == nil || len(h.st.regs) != 0 {
		t.Fatalf("create failure must roll back: %v regs=%d", err, len(h.st.regs))
	}
}

func TestRegularization_RejectCancelList(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user, other, approver := uuid.New(), uuid.New(), uuid.New()
	emp := h.emps.add(h.tenant, user, "active")
	h.emps.add(h.tenant, other, "active")
	mk := func(day int) uuid.UUID {
		r, err := svc.Create(ctx, h.actor(user), regReq("2026-10-0"+string(rune('0'+day)),
			time.Date(2026, 10, day, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, day, 18, 0, 0, 0, time.UTC)))
		if err != nil {
			t.Fatal(err)
		}
		return uuid.MustParse(r.ID)
	}
	a, b := mk(5), mk(6)
	if _, err := svc.Reject(ctx, h.actor(approver), a, dto.ReviewRequest{}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("AT-016 reject needs comment: %v", err)
	}
	rej, err := svc.Reject(ctx, h.actor(approver), a, dto.ReviewRequest{Comment: strp("no evidence")})
	if err != nil || rej.Status != models.RegRejected || rej.ReviewComment == nil {
		t.Fatalf("reject: %+v %v", rej, err)
	}
	if _, err := svc.Cancel(ctx, h.actor(user), a); codeOf(err) != "REGULARIZATION_NOT_PENDING" {
		t.Fatalf("cancel rejected: %v", err)
	}
	if _, err := svc.Cancel(ctx, h.actor(other), b); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("AT-018 only owner cancels (404, no leak): %v", err)
	}
	if _, err := svc.Approve(ctx, h.actor(approver), uuid.New(), dto.ReviewRequest{}); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("approve unknown: %v", err)
	}
	p := dto.Page{Page: 1, PerPage: 20}
	mine, meta, err := svc.ListMine(ctx, h.actor(user), "pending", p)
	if err != nil || len(mine) != 1 || meta.TotalItems != 1 {
		t.Fatalf("list mine: %v %+v %v", mine, meta, err)
	}
	all, _, err := svc.List(ctx, h.tenant, "", emp.ID.String(), p)
	if err != nil || len(all) != 2 {
		t.Fatalf("admin list: %v %v", all, err)
	}
	if _, _, err := svc.List(ctx, h.tenant, "weird", "", p); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad status: %v", err)
	}
	if _, _, err := svc.List(ctx, h.tenant, "", "bad", p); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad employee: %v", err)
	}
	if _, _, err := svc.ListMine(ctx, h.actor(uuid.New()), "", p); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("no profile: %v", err)
	}
	if _, _, err := svc.ListMine(ctx, h.actor(user), "nope", p); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad status mine: %v", err)
	}
	h.st.fail["reg.list"] = errors.New("db down")
	if _, _, err := svc.List(ctx, h.tenant, "", "", p); err == nil {
		t.Fatal("repo error must propagate")
	}
}

func TestRegularization_ApproveRollsBackOnFailure(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	r, err := svc.Create(ctx, h.actor(user), regReq("2026-10-07", time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	h.st.fail["punch.create"] = errors.New("db down")
	if _, err := svc.Approve(ctx, h.actor(uuid.New()), uuid.MustParse(r.ID), dto.ReviewRequest{}); err == nil {
		t.Fatal("expected failure")
	}
	if h.st.regs[uuid.MustParse(r.ID)].Status != models.RegPending || len(h.st.records) != 0 {
		t.Fatal("failed approval must leave request pending and create nothing")
	}
}
