package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/events"
	"github.com/jaas/jaas/internal/leave/models"
)

func ledgerKinds(h *harness) map[string]int64 {
	out := map[string]int64{}
	for _, e := range h.st.ledger {
		out[e.Kind] += int64(e.Days)
	}
	return out
}

func outboxKeys(h *harness) map[string]int {
	out := map[string]int{}
	for _, e := range h.st.outbox {
		out[e.RoutingKey]++
	}
	return out
}

// TestGolden_G5_ApplyReserves — golden G5 (FR-LR002, LV-013).
func TestGolden_G5_ApplyReserves(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1000, nil)
	res, err := h.apply(t, user, el, "2026-10-12", "2026-10-13")
	if err != nil || res.Status != models.StatusPending || res.TotalDays != 200 || res.LeaveTypeCode != "EL" {
		t.Fatalf("apply: %+v %v", res, err)
	}
	b := h.balanceOf(emp.ID, el.ID)
	if b.Accrued != 1000 || b.Reserved != 200 || b.Available() != 800 {
		t.Fatalf("balance %+v", b)
	}
	if k := ledgerKinds(h); k[models.KindAccrual] != 1000 || k[models.KindReserve] != 200 || len(h.st.ledger) != 2 {
		t.Fatalf("ledger %+v", k)
	}
	if k := outboxKeys(h); k[events.Applied] != 1 || k[events.Accrued] != 1 {
		t.Fatalf("outbox %+v", k)
	}
	if len(h.pub.keys) != 1 || h.pub.keys[0] != events.Applied {
		t.Fatalf("applied published after commit; accrual left to relay: %v", h.pub.keys)
	}
	if h.st.locks == 0 {
		t.Fatal("LV-014 lock not taken")
	}
}

// TestGolden_G6_InsufficientBalance — golden G6 (LV-009).
func TestGolden_G6_InsufficientBalance(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	cl := h.leaveType("CL", 100, nil)
	if _, err := h.apply(t, user, cl, "2026-10-12", "2026-10-13"); codeOf(err) != "INSUFFICIENT_BALANCE" {
		t.Fatalf("paid overdraw: %v", err)
	}
	if len(h.st.requests) != 0 || len(h.st.ledger) != 0 {
		t.Fatal("rejected apply must roll back everything")
	}
	lop := h.leaveType("LOP", 0, func(t *models.LeaveType) { t.IsPaid = false })
	if _, err := h.apply(t, user, lop, "2026-10-12", "2026-10-13"); err != nil {
		t.Fatalf("unpaid skips the balance check: %v", err)
	}
	if b := h.balanceOf(emp.ID, lop.ID); b.Reserved != 200 || b.Available() != -200 {
		t.Fatalf("unpaid still reserves: %+v", b)
	}
}

// TestGolden_G7_Overlap — golden G7 (LV-008).
func TestGolden_G7_Overlap(t *testing.T) {
	h := newHarness()
	user, _ := h.employee()
	el := h.leaveType("EL", 2000, nil)
	if _, err := h.apply(t, user, el, "2026-10-12", "2026-10-14"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.apply(t, user, el, "2026-10-13", "2026-10-13"); codeOf(err) != "LEAVE_OVERLAP" {
		t.Fatalf("overlap: %v", err)
	}
	svc := NewRequestService(h.deps)
	half := func(which string) error {
		_, err := svc.Apply(context.Background(), h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(),
			StartDate: "2026-10-15", EndDate: "2026-10-15", HalfDay: which, Reason: "x"})
		return err
	}
	if err := half(models.HalfFirst); err != nil {
		t.Fatal(err)
	}
	if err := half(models.HalfSecond); err != nil {
		t.Fatalf("opposite halves coexist: %v", err)
	}
	if err := half(models.HalfFirst); codeOf(err) != "LEAVE_OVERLAP" {
		t.Fatalf("same half twice: %v", err)
	}
}

// TestGolden_G8_ApproveMarksAttendance — golden G8 (FR-LR003, LV-015).
func TestGolden_G8_ApproveMarksAttendance(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1000, nil)
	req, _ := h.apply(t, user, el, "2026-10-12", "2026-10-13")
	res, err := NewRequestService(h.deps).Approve(context.Background(), h.actor(uuid.New()), req.ID, dto.ReviewRequest{Comment: strp("ok")})
	if err != nil || res.Status != models.StatusApproved || res.ReviewerUserID == nil || *res.ReviewComment != "ok" {
		t.Fatalf("approve: %+v %v", res, err)
	}
	b := h.balanceOf(emp.ID, el.ID)
	if b.Used != 200 || b.Reserved != 0 || b.Available() != 800 {
		t.Fatalf("balance %+v", b)
	}
	if k := ledgerKinds(h); k[models.KindRelease] != -200 || k[models.KindConsume] != 200 {
		t.Fatalf("ledger %+v", k)
	}
	if len(h.att.marked) != 2 || h.att.marked[0].Day() != 12 || h.att.marked[1].Day() != 13 {
		t.Fatalf("attendance marks %v", h.att.marked)
	}
}

// TestGolden_G10_SelfApproval — golden G10 (LV-010).
func TestGolden_G10_SelfApproval(t *testing.T) {
	h := newHarness()
	user, _ := h.employee()
	el := h.leaveType("EL", 1000, nil)
	req, _ := h.apply(t, user, el, "2026-10-12", "2026-10-12")
	svc := NewRequestService(h.deps)
	if _, err := svc.Approve(context.Background(), h.actor(user), req.ID, dto.ReviewRequest{}); codeOf(err) != "SELF_APPROVAL_FORBIDDEN" {
		t.Fatalf("self approve: %v", err)
	}
	if _, err := svc.Reject(context.Background(), h.actor(user), req.ID, dto.ReviewRequest{Comment: strp("no")}); codeOf(err) != "SELF_APPROVAL_FORBIDDEN" {
		t.Fatalf("self reject: %v", err)
	}
	if h.st.requests[req.ID].Status != models.StatusPending || len(h.att.marked) != 0 {
		t.Fatal("request must stay pending, attendance untouched")
	}
}

// TestGolden_G11_AuditPrivacy — golden G11 (LS-T6, FR-EV003).
func TestGolden_G11_AuditPrivacy(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1000, nil)
	svc, ctx, approver := NewRequestService(h.deps), context.Background(), h.actor(uuid.New())
	a, _ := h.apply(t, user, el, "2026-10-12", "2026-10-12")
	b, _ := h.apply(t, user, el, "2026-10-13", "2026-10-13")
	c, _ := h.apply(t, user, el, "2026-10-14", "2026-10-14")
	_, _ = svc.Approve(ctx, approver, a.ID, dto.ReviewRequest{})
	_, _ = svc.Reject(ctx, approver, b.ID, dto.ReviewRequest{Comment: strp("busy week")})
	_, _ = svc.Cancel(ctx, h.actor(user), c.ID)
	_, _ = NewBalanceService(h.deps).Adjust(ctx, approver, dto.AdjustBalanceRequest{EmployeeID: emp.ID.String(),
		LeaveTypeID: el.ID.String(), Days: 50, Reason: "medical certificate"})
	want := []string{"leave.applied", "leave.applied", "leave.applied", "leave.approved", "leave.rejected", "leave.cancelled", "leave.balance_adjusted"}
	if len(h.audit.entries) != len(want) {
		t.Fatalf("audit rows %d, want %d", len(h.audit.entries), len(want))
	}
	for i, e := range h.audit.entries {
		meta := strings.ToLower(strings.Join(metaValues(e.metadata), " "))
		if e.action != want[i] || strings.Contains(meta, "family") || strings.Contains(meta, "busy") || strings.Contains(meta, "medical") {
			t.Errorf("audit %d = %s %v", i, e.action, e.metadata)
		}
	}
	for _, o := range h.st.outbox {
		if strings.Contains(o.Payload, "family") || strings.Contains(o.Payload, "busy") || strings.Contains(o.Payload, "reason") {
			t.Fatalf("payload leaks free text: %s", o.Payload)
		}
	}
}

func metaValues(m interface{}) []string {
	mm, _ := m.(map[string]string)
	var out []string
	for _, v := range mm {
		out = append(out, v)
	}
	return out
}

// TestGolden_G13_CancelApprovedFuture — golden G13 (FR-LR005, LV-011).
func TestGolden_G13_CancelApprovedFuture(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1000, nil)
	svc, ctx := NewRequestService(h.deps), context.Background()
	req, _ := h.apply(t, user, el, "2026-10-12", "2026-10-13")
	_, _ = svc.Approve(ctx, h.actor(uuid.New()), req.ID, dto.ReviewRequest{})
	res, err := svc.Cancel(ctx, h.actor(user), req.ID)
	if err != nil || res.Status != models.StatusCancelled || res.CancelledAt == nil {
		t.Fatalf("cancel: %+v %v", res, err)
	}
	if b := h.balanceOf(emp.ID, el.ID); b.Used != 0 || b.Available() != 1000 {
		t.Fatalf("used restored: %+v", b)
	}
	if k := ledgerKinds(h); k[models.KindReversal] != -200 || len(h.att.cleared) != 2 {
		t.Fatalf("reversal %+v cleared %v", k, h.att.cleared)
	}
	started, _ := h.apply(t, user, el, "2026-10-14", "2026-10-14")
	_, _ = svc.Approve(ctx, h.actor(uuid.New()), started.ID, dto.ReviewRequest{})
	h.now = h.now.AddDate(0, 0, 6) // 2026-10-14
	if _, err := svc.Cancel(ctx, h.actor(user), started.ID); codeOf(err) != "LEAVE_ALREADY_STARTED" {
		t.Fatalf("started leave: %v", err)
	}
}
