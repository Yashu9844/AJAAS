package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

func codeOf(err error) string {
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return ""
}

func createShift(t *testing.T, h *harness, req dto.CreateShiftRequest) *dto.ShiftResponse {
	t.Helper()
	res, err := NewShiftService(h.deps).Create(context.Background(), h.actor(uuid.New()), req)
	if err != nil {
		t.Fatalf("create shift: %v", err)
	}
	return res
}

func TestShiftService_CreateDefaultsAndDerivedFields(t *testing.T) {
	h := newHarness()
	res := createShift(t, h, dto.CreateShiftRequest{Name: "Night", Code: strp("night"), StartTime: "22:00", EndTime: "06:00", Timezone: "Asia/Kolkata"})
	if !res.IsNightShift || res.GracePeriodMins != 15 || res.BreakDurationMins != 60 || res.ExpectedMinutes != 420 || res.Status != "active" {
		t.Fatalf("unexpected shift %+v", res)
	}
	zero := createShift(t, h, dto.CreateShiftRequest{Name: "Strict", StartTime: "09:00", EndTime: "17:00", GracePeriodMins: intp(0), BreakDurationMins: intp(0)})
	if zero.GracePeriodMins != 0 || zero.BreakDurationMins != 0 || zero.Timezone != "UTC" || zero.IsNightShift {
		t.Fatalf("explicit zeros must persist: %+v", zero)
	}
	if got := h.audit.actions(); len(got) != 2 || got[0] != "shift.created" {
		t.Fatalf("audit = %v", got)
	}
}

func TestShiftService_CreateValidation(t *testing.T) {
	h := newHarness()
	svc := NewShiftService(h.deps)
	createShift(t, h, dto.CreateShiftRequest{Name: "Day", Code: strp("day"), StartTime: "09:00", EndTime: "18:00"})
	cases := map[string]struct {
		req  dto.CreateShiftRequest
		code string
	}{
		"bad start":     {dto.CreateShiftRequest{Name: "A", StartTime: "25:00", EndTime: "18:00"}, "VALIDATION_ERROR"},
		"bad end":       {dto.CreateShiftRequest{Name: "A", StartTime: "09:00", EndTime: "9:00x"}, "VALIDATION_ERROR"},
		"start==end":    {dto.CreateShiftRequest{Name: "A", StartTime: "09:00", EndTime: "09:00"}, "VALIDATION_ERROR"},
		"bad code":      {dto.CreateShiftRequest{Name: "A", Code: strp("Bad Code"), StartTime: "09:00", EndTime: "18:00"}, "VALIDATION_ERROR"},
		"bad tz":        {dto.CreateShiftRequest{Name: "A", StartTime: "09:00", EndTime: "18:00", Timezone: "Mars/X"}, "VALIDATION_ERROR"},
		"half > full":   {dto.CreateShiftRequest{Name: "A", StartTime: "09:00", EndTime: "18:00", FullDayMinutes: intp(100), HalfDayMinutes: intp(200)}, "VALIDATION_ERROR"},
		"dup name (ci)": {dto.CreateShiftRequest{Name: "DAY", StartTime: "09:00", EndTime: "18:00"}, "CONFLICT"},
		"dup code":      {dto.CreateShiftRequest{Name: "Other", Code: strp("day"), StartTime: "09:00", EndTime: "18:00"}, "CONFLICT"},
	}
	for name, tc := range cases {
		if _, err := svc.Create(context.Background(), h.actor(uuid.New()), tc.req); codeOf(err) != tc.code {
			t.Errorf("%s: got %v, want %s", name, err, tc.code)
		}
	}
	h.st.fail["shift.create"] = errors.New("db down")
	if _, err := svc.Create(context.Background(), h.actor(uuid.New()), dto.CreateShiftRequest{Name: "Z", StartTime: "09:00", EndTime: "18:00"}); err == nil {
		t.Fatal("repo error must propagate")
	}
}

func TestShiftService_GetListUpdate(t *testing.T) {
	h := newHarness()
	svc := NewShiftService(h.deps)
	ctx := context.Background()
	a := createShift(t, h, dto.CreateShiftRequest{Name: "Alpha", Code: strp("alpha"), StartTime: "09:00", EndTime: "18:00"})
	createShift(t, h, dto.CreateShiftRequest{Name: "Beta", StartTime: "10:00", EndTime: "19:00"})
	id := uuid.MustParse(a.ID)

	if got, err := svc.Get(ctx, h.tenant, id); err != nil || got.Name != "Alpha" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := svc.Get(ctx, uuid.New(), id); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("cross-tenant get must 404, got %v", err)
	}
	list, meta, err := svc.List(ctx, h.tenant, "active", dto.Page{Page: 1, PerPage: 1})
	if err != nil || len(list) != 1 || meta.TotalItems != 2 || meta.TotalPages != 2 || list[0].Name != "Alpha" {
		t.Fatalf("list: %+v %+v %v", list, meta, err)
	}
	if _, _, err := svc.List(ctx, h.tenant, "bogus", dto.Page{Page: 1, PerPage: 20}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad status filter: %v", err)
	}

	up, err := svc.Update(ctx, h.actor(uuid.New()), id, dto.UpdateShiftRequest{
		Name: strp("Alpha 2"), StartTime: strp("22:00"), EndTime: strp("06:00"), GracePeriodMins: intp(5),
		BreakDurationMins: intp(30), FullDayMinutes: intp(400), HalfDayMinutes: intp(200), Timezone: strp("Asia/Kolkata"),
	})
	if err != nil || up.Name != "Alpha 2" || !up.IsNightShift || up.GracePeriodMins != 5 || up.Timezone != "Asia/Kolkata" || up.Code == nil || *up.Code != "alpha" {
		t.Fatalf("update: %+v %v", up, err)
	}
	bad := map[string]dto.UpdateShiftRequest{
		"dup name":   {Name: strp("beta")},
		"bad start":  {StartTime: strp("99:99")},
		"bad end":    {EndTime: strp("xx:xx")},
		"same times": {StartTime: strp("06:00")},
		"bad tz":     {Timezone: strp("Nope/Zone")},
		"thresholds": {HalfDayMinutes: intp(500)},
	}
	for name, req := range bad {
		if _, err := svc.Update(ctx, h.actor(uuid.New()), id, req); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := svc.Update(ctx, h.actor(uuid.New()), uuid.New(), dto.UpdateShiftRequest{}); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("update unknown: %v", err)
	}
}

func TestShiftService_DeactivateGuard(t *testing.T) {
	h := newHarness()
	svc := NewShiftService(h.deps)
	ctx := context.Background()
	s := createShift(t, h, dto.CreateShiftRequest{Name: "Day", StartTime: "09:00", EndTime: "18:00"})
	id := uuid.MustParse(s.ID)
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	h.st.assigns[uuid.New()] = models.ShiftAssignment{ID: uuid.New(), TenantID: h.tenant, EmployeeProfileID: emp.ID, ShiftID: id,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	if _, err := svc.Deactivate(ctx, h.actor(uuid.New()), id); codeOf(err) != "SHIFT_IN_USE" {
		t.Fatalf("AT-012: expected SHIFT_IN_USE, got %v", err)
	}
	h.st.assigns = map[uuid.UUID]models.ShiftAssignment{}
	res, err := svc.Deactivate(ctx, h.actor(uuid.New()), id)
	if err != nil || res.Status != "inactive" {
		t.Fatalf("deactivate: %+v %v", res, err)
	}
	if _, err := svc.Deactivate(ctx, h.actor(uuid.New()), id); codeOf(err) != "CONFLICT" {
		t.Fatalf("double deactivate: %v", err)
	}
	if _, err := svc.Deactivate(ctx, h.actor(uuid.New()), uuid.New()); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("unknown: %v", err)
	}
}
