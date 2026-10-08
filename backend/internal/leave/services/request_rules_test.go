package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
)

// TestApply_Rules covers LV-001..LV-007 error codes (spec §6). Today is Thu 2026-10-08.
func TestApply_Rules(t *testing.T) {
	h := newHarness()
	user, _ := h.employee()
	el := h.leaveType("EL", 3000, nil)
	inactive := h.leaveType("OLD", 1000, func(t *models.LeaveType) { t.Status = models.TypeInactive })
	maleOnly := h.leaveType("PAT", 1000, func(t *models.LeaveType) { t.ApplicableGender = models.GenderMale })
	noHalf := h.leaveType("SL", 1000, func(t *models.LeaveType) { t.AllowHalfDay = false })
	notice := h.leaveType("NL", 1000, func(t *models.LeaveType) { t.MinNoticeDays = 7 })
	short := h.leaveType("ML", 1000, func(t *models.LeaveType) { t.MaxConsecutiveDays = intp(1) })
	svc := NewRequestService(h.deps)
	cases := []struct {
		name string
		req  dto.ApplyLeaveRequest
		code string
	}{
		{"bad type uuid", dto.ApplyLeaveRequest{LeaveTypeID: "x", StartDate: "2026-10-12", EndDate: "2026-10-12"}, "VALIDATION_ERROR"},
		{"unknown type", dto.ApplyLeaveRequest{LeaveTypeID: uuid.NewString(), StartDate: "2026-10-12", EndDate: "2026-10-12"}, "NOT_FOUND"},
		{"inactive type", dto.ApplyLeaveRequest{LeaveTypeID: inactive.ID.String(), StartDate: "2026-10-12", EndDate: "2026-10-12"}, "LEAVE_TYPE_INACTIVE"},
		{"gender", dto.ApplyLeaveRequest{LeaveTypeID: maleOnly.ID.String(), StartDate: "2026-10-12", EndDate: "2026-10-12"}, "LEAVE_TYPE_NOT_APPLICABLE"},
		{"bad start", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "12/10", EndDate: "2026-10-12"}, "VALIDATION_ERROR"},
		{"bad end", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-10-12", EndDate: "x"}, "VALIDATION_ERROR"},
		{"start after end", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-10-13", EndDate: "2026-10-12"}, "INVALID_LEAVE_RANGE"},
		{"next year", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-12-31", EndDate: "2027-01-04"}, "INVALID_LEAVE_RANGE"},
		{"half multi-day", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-10-12", EndDate: "2026-10-13", HalfDay: "first_half"}, "VALIDATION_ERROR"},
		{"half not allowed", dto.ApplyLeaveRequest{LeaveTypeID: noHalf.ID.String(), StartDate: "2026-10-12", EndDate: "2026-10-12", HalfDay: "first_half"}, "VALIDATION_ERROR"},
		{"notice", dto.ApplyLeaveRequest{LeaveTypeID: notice.ID.String(), StartDate: "2026-10-12", EndDate: "2026-10-12"}, "LEAVE_NOTICE"},
		{"backdate window", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-09-01", EndDate: "2026-09-01"}, "LEAVE_NOTICE"},
		{"weekend only", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-10-10", EndDate: "2026-10-11"}, "NO_WORKING_DAYS"},
		{"too long", dto.ApplyLeaveRequest{LeaveTypeID: short.ID.String(), StartDate: "2026-10-12", EndDate: "2026-10-13"}, "LEAVE_TOO_LONG"},
		{"backdated within window ok", dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-09-21", EndDate: "2026-09-21"}, ""},
	}
	for _, tc := range cases {
		tc.req.Reason = "r"
		if _, err := svc.Apply(context.Background(), h.actor(user), tc.req); codeOf(err) != tc.code {
			t.Errorf("%s: got %q want %q", tc.name, codeOf(err), tc.code)
		}
	}
}

// LV-001: caller must have a working employee profile.
func TestApply_EmployeeChecks(t *testing.T) {
	h := newHarness()
	el := h.leaveType("EL", 1000, nil)
	if _, err := h.apply(t, uuid.New(), el, "2026-10-12", "2026-10-12"); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("no profile: %v", err)
	}
	gone := uuid.New()
	h.emps.add(h.tenant, gone, "terminated", "", time.Time{})
	if _, err := h.apply(t, gone, el, "2026-10-12", "2026-10-12"); codeOf(err) != "EMPLOYEE_NOT_ACTIVE" {
		t.Fatalf("terminated: %v", err)
	}
	h.emps.err = errString("directory down")
	if _, err := h.apply(t, gone, el, "2026-10-12", "2026-10-12"); codeOf(err) != "ERR:directory down" {
		t.Fatalf("directory error: %v", err)
	}
}

// LV-006 with the tenant calendar: blocking holidays excluded, optional ones counted; sandwich (EC-03, EC-05, EC-06).
func TestPreview_CalendarAndSandwich(t *testing.T) {
	h := newHarness()
	user, _ := h.employee()
	el := h.leaveType("EL", 2000, nil)
	sw := h.leaveType("SW", 2000, func(t *models.LeaveType) { t.SandwichRule = true })
	h.st.holidays[uuid.New()] = models.Holiday{TenantID: h.tenant, HolidayDate: day("2026-10-14"), Name: "Fest"}
	h.st.holidays[uuid.New()] = models.Holiday{TenantID: h.tenant, HolidayDate: day("2026-10-15"), Name: "Opt", IsOptional: true}
	svc := NewRequestService(h.deps)
	p, err := svc.Preview(context.Background(), h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(),
		StartDate: "2026-10-12", EndDate: "2026-10-16", Reason: "x"})
	if err != nil || p.TotalDays != 400 || len(p.WorkingDates) != 4 || p.Available != 2000 || p.AvailableAfter != 1600 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	p, _ = svc.Preview(context.Background(), h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: sw.ID.String(),
		StartDate: "2026-10-09", EndDate: "2026-10-12", Reason: "x"})
	if p.TotalDays != 400 {
		t.Fatalf("sandwich Fri–Mon = %s", p.TotalDays)
	}
	if len(h.st.requests) != 0 {
		t.Fatal("preview must not create requests")
	}
	if _, err := svc.Preview(context.Background(), h.actor(uuid.New()), dto.ApplyLeaveRequest{}); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("preview unknown employee: %v", err)
	}
	if _, err := svc.Preview(context.Background(), h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(),
		StartDate: "2026-10-10", EndDate: "2026-10-11"}); codeOf(err) != "NO_WORKING_DAYS" {
		t.Fatalf("preview weekend: %v", err)
	}
}

func TestReview_States(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1000, nil)
	svc, ctx, boss := NewRequestService(h.deps), context.Background(), h.actor(uuid.New())
	req, _ := h.apply(t, user, el, "2026-10-12", "2026-10-12")
	if _, err := svc.Reject(ctx, boss, req.ID, dto.ReviewRequest{}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("reject without comment: %v", err)
	}
	res, err := svc.Reject(ctx, boss, req.ID, dto.ReviewRequest{Comment: strp("no")})
	if err != nil || res.Status != models.StatusRejected || h.balanceOf(emp.ID, el.ID).Reserved != 0 {
		t.Fatalf("reject: %+v %v", res, err)
	}
	if _, err := svc.Approve(ctx, boss, req.ID, dto.ReviewRequest{}); codeOf(err) != "LEAVE_NOT_PENDING" {
		t.Fatalf("approve rejected: %v", err)
	}
	if _, err := svc.Cancel(ctx, h.actor(user), req.ID); codeOf(err) != "LEAVE_NOT_CANCELLABLE" {
		t.Fatalf("cancel rejected: %v", err)
	}
	if _, err := svc.Approve(ctx, boss, uuid.New(), dto.ReviewRequest{}); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("approve unknown: %v", err)
	}
	other, _ := h.employee()
	pending, _ := h.apply(t, user, el, "2026-10-13", "2026-10-13")
	if _, err := svc.Cancel(ctx, h.actor(other), pending.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("cancel someone else's: %v", err)
	}
	if _, err := svc.Cancel(ctx, h.actor(uuid.New()), pending.ID); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("cancel without profile: %v", err)
	}
	res, err = svc.Cancel(ctx, h.actor(user), pending.ID)
	if err != nil || res.Status != models.StatusCancelled || h.balanceOf(emp.ID, el.ID).Reserved != 0 {
		t.Fatalf("cancel pending: %+v %v", res, err)
	}
}

// Half-day approval marks no attendance date (D4-09).
func TestApprove_HalfDayNoAttendance(t *testing.T) {
	h := newHarness()
	user, _ := h.employee()
	el := h.leaveType("EL", 1000, nil)
	svc, ctx := NewRequestService(h.deps), context.Background()
	req, err := svc.Apply(ctx, h.actor(user), dto.ApplyLeaveRequest{LeaveTypeID: el.ID.String(), StartDate: "2026-10-12",
		EndDate: "2026-10-12", HalfDay: models.HalfSecond, Reason: "x"})
	if err != nil || req.TotalDays != 50 {
		t.Fatalf("half apply: %+v %v", req, err)
	}
	if _, err := svc.Approve(ctx, h.actor(uuid.New()), req.ID, dto.ReviewRequest{}); err != nil || len(h.att.marked) != 0 {
		t.Fatalf("half approve: %v marks %v", err, h.att.marked)
	}
	if _, err := svc.Cancel(ctx, h.actor(user), req.ID); err != nil || len(h.att.cleared) != 0 {
		t.Fatalf("half cancel: %v clears %v", err, h.att.cleared)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func day(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}
