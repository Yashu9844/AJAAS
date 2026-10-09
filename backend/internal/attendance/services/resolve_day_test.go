package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
)

func assignFrom(t *testing.T, h *harness, emp uuid.UUID, req dto.CreateShiftRequest, from string, to *string) {
	t.Helper()
	s := createShift(t, h, req)
	if _, err := NewAssignmentService(h.deps).Assign(context.Background(), h.actor(uuid.New()), uuid.MustParse(s.ID),
		dto.AssignShiftRequest{EmployeeID: emp.String(), EffectiveFrom: from, EffectiveTo: to}); err != nil {
		t.Fatal(err)
	}
}

// AT-005 across UTC midnight: an IST day shift punched at 20:00 UTC belongs to the next IST day.
func TestResolveDay_DateMovesForwardWithShiftZone(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	emp := h.emps.add(h.tenant, user, "active")
	assignFrom(t, h, emp.ID, dto.CreateShiftRequest{Name: "IST Day", StartTime: "01:00", EndTime: "10:00", Timezone: "Asia/Kolkata"}, "2026-10-01", nil)
	h.now = time.Date(2026, 10, 8, 20, 0, 0, 0, time.UTC) // 01:30 IST on 2026-10-09
	res, err := punch(t, h, user, "in")
	if err != nil || res.Record.AttendanceDate != "2026-10-09" || res.Record.ShiftID == nil || res.Record.LateMinutes != 15 {
		t.Fatalf("expected IST date 2026-10-09 with 15 min late: %+v %v", res, err)
	}
}

// If attribution moves the date onto a day with no assignment, fall back to no-shift UTC rules.
func TestResolveDay_MovedOntoUnassignedDayFallsBack(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	emp := h.emps.add(h.tenant, user, "active")
	assignFrom(t, h, emp.ID, dto.CreateShiftRequest{Name: "IST Day", StartTime: "01:00", EndTime: "10:00", Timezone: "Asia/Kolkata"}, "2026-10-01", strp("2026-10-08"))
	h.now = time.Date(2026, 10, 8, 20, 0, 0, 0, time.UTC)
	res, err := punch(t, h, user, "in")
	if err != nil || res.Record.AttendanceDate != "2026-10-08" || res.Record.ShiftID != nil {
		t.Fatalf("expected UTC fallback date without shift: %+v %v", res, err)
	}
}

func TestResolveDay_RepoErrorsPropagate(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	h.st.fail["assign.active"] = errors.New("db down")
	if _, err := punch(t, h, user, "in"); err == nil {
		t.Fatal("assignment lookup error must propagate")
	}
	if _, err := NewQueryService(h.deps).Today(context.Background(), h.actor(user)); err == nil {
		t.Fatal("Today must propagate assignment errors")
	}
}

func TestPunch_MobileSource(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	res, err := NewPunchService(h.deps).Punch(context.Background(), h.actor(user), dto.PunchRequest{Type: "in", Source: "mobile"})
	if err != nil || res.Punch.Source != "mobile" || res.Record.Source != "mobile" {
		t.Fatalf("mobile source: %+v %v", res, err)
	}
}

func TestReview_DirectoryFailureFailsClosed(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewRegularizationService(h.deps)
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	r, err := svc.Create(ctx, h.actor(user), regReq("2026-10-07", time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	h.emps.err = errors.New("module 2 down")
	if _, err := svc.Approve(ctx, h.actor(uuid.New()), uuid.MustParse(r.ID), dto.ReviewRequest{}); err == nil {
		t.Fatal("self-review check must fail closed when Module 2 is down")
	}
}
