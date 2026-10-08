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

func TestQuery_TodayStates(t *testing.T) {
	h := newHarness()
	q := NewQueryService(h.deps)
	ctx := context.Background()
	user := uuid.New()
	emp := h.emps.add(h.tenant, user, "active")

	empty, err := q.Today(ctx, h.actor(user))
	if err != nil || empty.Record != nil || empty.OpenSession || empty.Shift != nil || empty.AttendanceDate != "2026-10-08" {
		t.Fatalf("before any punch: %+v %v", empty, err)
	}
	shift := createShift(t, h, dto.CreateShiftRequest{Name: "Day", StartTime: "09:00", EndTime: "18:00"})
	if _, err := NewAssignmentService(h.deps).Assign(ctx, h.actor(uuid.New()), uuid.MustParse(shift.ID),
		dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	h.advance(2 * time.Hour)
	open, err := q.Today(ctx, h.actor(user))
	if err != nil || !open.OpenSession || open.Record == nil || open.Shift == nil || open.Shift.Name != "Day" {
		t.Fatalf("open session: %+v %v", open, err)
	}
	if _, err := punch(t, h, user, "out"); err != nil {
		t.Fatal(err)
	}
	closed, _ := q.Today(ctx, h.actor(user))
	if closed.OpenSession || closed.Record == nil || closed.Record.TotalWorkMinutes != 120 {
		t.Fatalf("closed: %+v", closed)
	}
	if _, err := q.Today(ctx, h.actor(uuid.New())); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("no profile: %v", err)
	}
}

func TestQuery_MineRange(t *testing.T) {
	h := newHarness()
	q := NewQueryService(h.deps)
	ctx := context.Background()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	rows, err := q.Mine(ctx, h.actor(user), "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("default 7-day window: %v %v", rows, err)
	}
	if _, err := q.Mine(ctx, h.actor(user), "2026-10-09", "2026-10-01"); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("EC-17 from>to: %v", err)
	}
	if _, err := q.Mine(ctx, h.actor(user), "2026-01-01", "2026-10-08"); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("EC-17 range > 62 days: %v", err)
	}
	if _, err := q.Mine(ctx, h.actor(uuid.New()), "", ""); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("no profile: %v", err)
	}
	h.st.fail["record.list"] = errors.New("db down")
	if _, err := q.Mine(ctx, h.actor(user), "", ""); err == nil {
		t.Fatal("repo error must propagate")
	}
}

func TestQuery_AdminListAndDetail(t *testing.T) {
	h := newHarness()
	q := NewQueryService(h.deps)
	ctx := context.Background()
	user := uuid.New()
	emp := h.emps.add(h.tenant, user, "active")
	res, err := punch(t, h, user, "in")
	if err != nil {
		t.Fatal(err)
	}
	p := dto.Page{Page: 1, PerPage: 20}
	rows, meta, err := q.List(ctx, h.tenant, RecordQuery{EmployeeID: emp.ID.String(), Status: "present"}, p)
	if err != nil || len(rows) != 1 || meta.TotalItems != 1 {
		t.Fatalf("list: %v %+v %v", rows, meta, err)
	}
	bad := map[string]RecordQuery{
		"employee": {EmployeeID: "x"}, "status": {Status: "sleeping"}, "range": {From: "2026-10-09", To: "2026-10-01"},
	}
	for name, rq := range bad {
		if _, _, err := q.List(ctx, h.tenant, rq, p); codeOf(err) != "VALIDATION_ERROR" {
			t.Errorf("%s: %v", name, err)
		}
	}
	other, _, _ := q.List(ctx, uuid.New(), RecordQuery{}, p)
	if len(other) != 0 {
		t.Fatal("G1: other tenant must see nothing")
	}
	detail, err := q.Detail(ctx, h.tenant, uuid.MustParse(res.Record.ID))
	if err != nil || len(detail.Punches) != 1 || detail.Record.ID != res.Record.ID {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	if _, err := q.Detail(ctx, uuid.New(), uuid.MustParse(res.Record.ID)); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("G1 cross-tenant detail must 404: %v", err)
	}
	h.st.fail["record.list"] = errors.New("db down")
	if _, _, err := q.List(ctx, h.tenant, RecordQuery{}, p); err == nil {
		t.Fatal("repo error must propagate")
	}
	h.st.fail["punch.all"] = errors.New("db down")
	if _, err := q.Detail(ctx, h.tenant, uuid.MustParse(res.Record.ID)); err == nil {
		t.Fatal("punch repo error must propagate")
	}
}

// TestQuery_Summary freezes AT-020 arithmetic.
func TestQuery_Summary(t *testing.T) {
	h := newHarness()
	q := NewQueryService(h.deps)
	ctx := context.Background()
	date := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	add := func(status string, late int) {
		e := h.emps.add(h.tenant, uuid.New(), "active")
		id := uuid.New()
		h.st.records[id] = models.AttendanceRecord{ID: id, TenantID: h.tenant, EmployeeProfileID: e.ID, AttendanceDate: date, Status: status, LateMinutes: late}
	}
	add("present", 5)
	add("present", 0)
	add("half_day", 0)
	add("on_leave", 0)
	h.emps.add(h.tenant, uuid.New(), "probation")  // no record → absent
	h.emps.add(h.tenant, uuid.New(), "terminated") // not counted
	got, err := q.Summary(ctx, h.tenant, "2026-10-08")
	want := &dto.SummaryResponse{Date: "2026-10-08", TotalEmployees: 5, Present: 2, Late: 1, HalfDay: 1, OnLeave: 1, Absent: 1}
	if err != nil || *got != *want {
		t.Fatalf("summary %+v %v, want %+v", got, err, want)
	}
	if def, err := q.Summary(ctx, h.tenant, ""); err != nil || def.Date != "2026-10-08" {
		t.Fatalf("default date: %+v %v", def, err)
	}
	if _, err := q.Summary(ctx, h.tenant, "bad"); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad date: %v", err)
	}
	h.st.fail["record.count"] = errors.New("db down")
	if _, err := q.Summary(ctx, h.tenant, ""); err == nil {
		t.Fatal("repo error must propagate")
	}
	delete(h.st.fail, "record.count")
	h.emps.err = errors.New("module 2 down")
	if _, err := q.Summary(ctx, h.tenant, ""); err == nil {
		t.Fatal("directory error must propagate")
	}
}
