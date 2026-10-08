package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
)

// TestGolden_G10_AssignmentOverlap — protected (golden-tests.md G10, AT-011 / FR-SA002).
func TestGolden_G10_AssignmentOverlap(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewAssignmentService(h.deps)
	shift := createShift(t, h, dto.CreateShiftRequest{Name: "Day", StartTime: "09:00", EndTime: "18:00"})
	sid := uuid.MustParse(shift.ID)
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	admin := h.actor(uuid.New())

	a, err := svc.Assign(ctx, admin, sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-01-01"})
	if err != nil {
		t.Fatalf("A: %v", err)
	}
	b, err := svc.Assign(ctx, admin, sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-03-01", EffectiveTo: strp("2026-06-30")})
	if err != nil {
		t.Fatalf("B must auto-close open-ended A: %v", err)
	}
	closed := h.st.assigns[uuid.MustParse(a.ID)]
	if closed.EffectiveTo == nil || !closed.EffectiveTo.Equal(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("A must end 2026-02-28, got %v", closed.EffectiveTo)
	}
	if _, err := svc.Assign(ctx, admin, sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-06-01", EffectiveTo: strp("2026-07-15")}); codeOf(err) != "ASSIGNMENT_OVERLAP" {
		t.Fatalf("C overlapping B must 409 ASSIGNMENT_OVERLAP, got %v", err)
	}
	if b.EffectiveTo == nil || *b.EffectiveTo != "2026-06-30" {
		t.Fatalf("B range wrong: %+v", b)
	}
	if len(h.st.assigns) != 2 {
		t.Fatalf("rejected C must not persist (tx rollback), have %d", len(h.st.assigns))
	}
}

func TestAssignmentService_Validation(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewAssignmentService(h.deps)
	shift := createShift(t, h, dto.CreateShiftRequest{Name: "Day", StartTime: "09:00", EndTime: "18:00"})
	sid := uuid.MustParse(shift.ID)
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	admin := h.actor(uuid.New())
	cases := map[string]struct {
		shift uuid.UUID
		req   dto.AssignShiftRequest
		code  string
	}{
		"bad employee uuid": {sid, dto.AssignShiftRequest{EmployeeID: "nope", EffectiveFrom: "2026-01-01"}, "VALIDATION_ERROR"},
		"bad from":          {sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "01-01-2026"}, "VALIDATION_ERROR"},
		"bad to":            {sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-01-01", EffectiveTo: strp("x")}, "VALIDATION_ERROR"},
		"to before from":    {sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-02-01", EffectiveTo: strp("2026-01-01")}, "VALIDATION_ERROR"},
		"unknown shift":     {uuid.New(), dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-01-01"}, "NOT_FOUND"},
		"unknown employee":  {sid, dto.AssignShiftRequest{EmployeeID: uuid.New().String(), EffectiveFrom: "2026-01-01"}, "NOT_FOUND"},
	}
	for name, tc := range cases {
		if _, err := svc.Assign(ctx, admin, tc.shift, tc.req); codeOf(err) != tc.code {
			t.Errorf("%s: got %v want %s", name, err, tc.code)
		}
	}
	if _, err := NewShiftService(h.deps).Deactivate(ctx, admin, sid); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.Assign(ctx, admin, sid, dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-01-01"}); codeOf(err) != "SHIFT_INACTIVE" {
		t.Fatalf("EC-18: inactive shift must 409, got %v", err)
	}
}

func TestAssignmentService_List(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	svc := NewAssignmentService(h.deps)
	shift := createShift(t, h, dto.CreateShiftRequest{Name: "Day", StartTime: "09:00", EndTime: "18:00"})
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	if _, err := svc.Assign(ctx, h.actor(uuid.New()), uuid.MustParse(shift.ID), dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	list, meta, err := svc.List(ctx, h.tenant, emp.ID.String(), dto.Page{Page: 1, PerPage: 20})
	if err != nil || len(list) != 1 || meta.TotalItems != 1 || list[0].EffectiveTo != nil {
		t.Fatalf("list: %+v %+v %v", list, meta, err)
	}
	if _, _, err := svc.List(ctx, h.tenant, "bad", dto.Page{Page: 1, PerPage: 20}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad employee id: %v", err)
	}
}
