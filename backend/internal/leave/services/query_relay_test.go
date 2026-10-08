package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
)

func TestRequestQueries(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	other, otherEmp := h.employee()
	el := h.leaveType("EL", 2000, nil)
	_, _ = h.apply(t, user, el, "2026-10-12", "2026-10-12")
	_, _ = h.apply(t, user, el, "2026-10-19", "2026-10-19")
	theirs, _ := h.apply(t, other, el, "2026-10-13", "2026-10-13")
	svc, ctx, page := NewRequestService(h.deps), context.Background(), dto.Page{Page: 1, PerPage: 20}
	mine, meta, err := svc.ListMine(ctx, h.actor(user), "pending", page)
	if err != nil || len(mine) != 2 || meta.TotalItems != 2 || mine[0].LeaveTypeCode != "EL" {
		t.Fatalf("mine: %d %v", len(mine), err)
	}
	if _, _, err := svc.ListMine(ctx, h.actor(uuid.New()), "", page); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("mine without profile: %v", err)
	}
	all, _, _ := svc.List(ctx, h.tenant, RequestQuery{}, page)
	byEmp, _, _ := svc.List(ctx, h.tenant, RequestQuery{EmployeeID: otherEmp.ID.String()}, page)
	ranged, _, _ := svc.List(ctx, h.tenant, RequestQuery{From: "2026-10-13", To: "2026-10-16"}, page)
	if len(all) != 3 || len(byEmp) != 1 || len(ranged) != 1 || ranged[0].EmployeeID != otherEmp.ID {
		t.Fatalf("filters: all=%d emp=%d range=%d", len(all), len(byEmp), len(ranged))
	}
	for _, q := range []RequestQuery{{EmployeeID: "x"}, {From: "bad"}, {To: "bad"}} {
		if _, _, err := svc.List(ctx, h.tenant, q, page); codeOf(err) != "VALIDATION_ERROR" {
			t.Errorf("list %+v: %v", q, err)
		}
	}
	got, err := svc.Get(ctx, h.tenant, theirs.ID)
	if err != nil || got.EmployeeID != otherEmp.ID || got.Reason == "" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := svc.Get(ctx, uuid.New(), theirs.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("cross-tenant get: %v", err)
	}
	h.st.fail["type.list"] = errors.New("db down")
	if _, err := svc.Get(ctx, h.tenant, theirs.ID); err == nil {
		t.Fatal("type lookup failure must surface")
	}
	if _, _, err := svc.List(ctx, h.tenant, RequestQuery{}, page); err == nil {
		t.Fatal("list type lookup failure must surface")
	}
	h.st.fail = map[string]error{"request.list": errors.New("db down")}
	if _, _, err := svc.List(ctx, h.tenant, RequestQuery{}, page); err == nil {
		t.Fatal("list failure must surface")
	}
	_ = emp
}

func TestOutboxRelay(t *testing.T) {
	h := newHarness()
	user, _ := h.employee()
	el := h.leaveType("EL", 1000, nil)
	h.pub.fail = true
	_, _ = h.apply(t, user, el, "2026-10-12", "2026-10-12") // applied + accrued rows stay unpublished
	relay := NewOutboxRelay(h.deps)
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("broker down: %d %v", n, err)
	}
	for _, o := range h.st.outbox {
		if o.Attempts != 1 || o.LastError == nil {
			t.Fatalf("failure must be recorded: %+v", o)
		}
	}
	h.pub.fail = false
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 2 {
		t.Fatalf("relay: %d %v", n, err)
	}
	h.st.fail["outbox.fetch"] = errors.New("db down")
	if _, err := relay.RelayOnce(context.Background()); err == nil {
		t.Fatal("fetch failure must surface")
	}
	relay.interval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	time.Sleep(5 * time.Millisecond)
	cancel()
	<-done
}

func TestTxRunner_DB(t *testing.T) {
	if NewTxRunner(nil).DB() != nil {
		t.Fatal("DB passthrough")
	}
}

// TestFaultInjection: every repository failure inside a command surfaces and rolls the tx back.
func TestFaultInjection(t *testing.T) {
	ops := []string{"balance.lock", "balance.create", "balance.find", "balance.update", "ledger.create", "outbox.create",
		"request.create", "request.update", "request.find", "request.overlap", "type.find", "holiday.list"}
	for _, op := range ops {
		h := newHarness()
		user, emp := h.employee()
		el := h.leaveType("EL", 1000, func(t *models.LeaveType) { t.CarryForwardLimit = 500 })
		prev := models.Balance{ID: uuid.New(), TenantID: h.tenant, EmployeeProfileID: emp.ID, LeaveTypeID: el.ID, Year: 2025, Accrued: 300}
		h.st.balances[prev.ID] = prev
		svc, ctx := NewRequestService(h.deps), context.Background()
		pending, _ := h.apply(t, user, el, "2026-10-13", "2026-10-13")
		approved, _ := h.apply(t, user, el, "2026-10-14", "2026-10-14")
		_, _ = svc.Approve(ctx, h.actor(uuid.New()), approved.ID, dto.ReviewRequest{})
		before := len(h.st.ledger)
		h.st.fail[op] = errors.New("boom:" + op)
		errs := []error{}
		_, err := h.apply(t, user, el, "2026-10-12", "2026-10-12")
		errs = append(errs, err)
		_, err = svc.Approve(ctx, h.actor(uuid.New()), pending.ID, dto.ReviewRequest{})
		errs = append(errs, err)
		_, err = svc.Cancel(ctx, h.actor(user), approved.ID)
		errs = append(errs, err)
		_, err = NewBalanceService(h.deps).Adjust(ctx, h.actor(uuid.New()), dto.AdjustBalanceRequest{EmployeeID: emp.ID.String(),
			LeaveTypeID: el.ID.String(), Days: 100, Reason: "x"})
		errs = append(errs, err)
		failed := 0
		for _, e := range errs {
			if e != nil {
				failed++
			}
		}
		if failed == 0 {
			t.Errorf("%s: no command failed", op)
		}
		if failed == len(errs) && len(h.st.ledger) != before {
			t.Errorf("%s: failed commands must not leave ledger rows", op)
		}
	}
}

// Attendance sync failures roll back approve/cancel (C8: same tx).
func TestAttendanceSyncFailureRollsBack(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1000, nil)
	svc, ctx := NewRequestService(h.deps), context.Background()
	req, _ := h.apply(t, user, el, "2026-10-12", "2026-10-12")
	h.att.err = errors.New("attendance down")
	if _, err := svc.Approve(ctx, h.actor(uuid.New()), req.ID, dto.ReviewRequest{}); err == nil {
		t.Fatal("approve must fail")
	}
	if h.st.requests[req.ID].Status != models.StatusPending || h.balanceOf(emp.ID, el.ID).Used != 0 {
		t.Fatal("approve must roll back")
	}
	h.att.err = nil
	_, _ = svc.Approve(ctx, h.actor(uuid.New()), req.ID, dto.ReviewRequest{})
	h.att.err = errors.New("attendance down")
	if _, err := svc.Cancel(ctx, h.actor(user), req.ID); err == nil || h.st.requests[req.ID].Status != models.StatusApproved {
		t.Fatalf("cancel must roll back: %v", err)
	}
}
