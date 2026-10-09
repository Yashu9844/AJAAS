package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
)

// TestUnpaidLeave_UnpaidDays — Module 5 contract C3 (D5-03): approved unpaid days clipped to the period.
func TestUnpaidLeave_UnpaidDays(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	lop := h.leaveType("LOP", 0, func(t *models.LeaveType) { t.IsPaid = false })
	paid := h.leaveType("EL", 2000, nil)
	svc, ctx, boss := NewRequestService(h.deps), context.Background(), h.actor(uuid.New())
	approve := func(typ *models.LeaveType, from, to, half string) {
		t.Helper()
		r, err := svc.Apply(ctx, h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: typ.ID.String(), StartDate: from, EndDate: to, HalfDay: half, Reason: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Approve(ctx, boss, r.ID, dto.ReviewRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	approve(lop, "2026-10-29", "2026-11-03", "")           // Thu 29 – Tue 3 Nov: Oct part = Thu, Fri = 2 working days
	approve(lop, "2026-10-13", "2026-10-13", "first_half") // 0.5
	approve(paid, "2026-10-14", "2026-10-15", "")          // paid → ignored
	if _, err := svc.Apply(ctx, h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: lop.ID.String(), StartDate: "2026-10-20", EndDate: "2026-10-20", Reason: "pending"}); err != nil {
		t.Fatal(err) // pending → ignored
	}
	unpaid := NewUnpaidLeave(h.deps)
	got, err := unpaid.UnpaidDays(ctx, h.tenant, emp.ID, day("2026-10-01"), day("2026-10-31"))
	if err != nil || got != calc.Days(250) {
		t.Fatalf("October unpaid = %s, %v (want 2.50)", got, err)
	}
	got, _ = unpaid.UnpaidDays(ctx, h.tenant, emp.ID, day("2026-11-01"), day("2026-11-30"))
	if got != calc.Days(200) {
		t.Fatalf("November unpaid = %s (Mon 2, Tue 3)", got)
	}
	if got, _ = unpaid.UnpaidDays(ctx, uuid.New(), emp.ID, day("2026-10-01"), day("2026-10-31")); got != 0 {
		t.Fatal("other tenant sees nothing")
	}
	h.st.fail["request.overlap"] = errors.New("db down")
	if _, err := unpaid.UnpaidDays(ctx, h.tenant, emp.ID, day("2026-10-01"), day("2026-10-31")); err == nil {
		t.Fatal("repository failure must surface")
	}
	h.st.fail = map[string]error{"type.find": errors.New("db down")}
	if _, err := unpaid.UnpaidDays(ctx, h.tenant, emp.ID, day("2026-10-01"), day("2026-10-31")); err == nil {
		t.Fatal("type lookup failure must surface")
	}
	h.st.fail = map[string]error{"holiday.list": errors.New("db down")}
	if _, err := unpaid.UnpaidDays(ctx, h.tenant, emp.ID, day("2026-10-01"), day("2026-10-31")); err == nil {
		t.Fatal("holiday failure must surface")
	}
}
