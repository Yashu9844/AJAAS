package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/models"
)

func syncDates(ds ...string) []time.Time {
	out := make([]time.Time, len(ds))
	for i, s := range ds {
		out[i], _ = time.Parse("2006-01-02", s)
	}
	return out
}

// TestLeaveSync_MarkAndClear — contract C10 / D3-17: leave-only records are created and removed;
// records with punches keep their attendance status (attendance wins, D4-09).
func TestLeaveSync_MarkAndClear(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	emp := h.emps.add(h.tenant, user, "active")
	if _, err := punch(t, h, user, "in"); err != nil { // creates a punched record on 2026-10-08
		t.Fatal(err)
	}
	sync := NewLeaveSync(h.deps)
	ctx := context.Background()
	dates := syncDates("2026-10-08", "2026-10-12", "2026-10-13")
	if err := sync.MarkLeave(ctx, nil, h.tenant, emp.ID, dates); err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, r := range h.st.records {
		statuses[r.AttendanceDate.Format("2006-01-02")] = r.Status + "/" + r.Source
	}
	want := map[string]string{"2026-10-08": "present/web", "2026-10-12": "on_leave/leave", "2026-10-13": "on_leave/leave"}
	for d, w := range want {
		if statuses[d] != w {
			t.Errorf("after mark %s = %q, want %q", d, statuses[d], w)
		}
	}
	if h.st.locks < 2 {
		t.Error("MarkLeave must take the attendance lock (AT-022)")
	}
	if err := sync.MarkLeave(ctx, nil, h.tenant, emp.ID, dates[1:2]); err != nil || len(h.st.records) != 3 {
		t.Fatalf("mark is idempotent: %v, %d records", err, len(h.st.records))
	}
	if err := sync.ClearLeave(ctx, nil, h.tenant, emp.ID, dates); err != nil {
		t.Fatal(err)
	}
	if len(h.st.records) != 1 {
		t.Fatalf("leave-only records must be removed, punched one kept: %d left", len(h.st.records))
	}
	for _, r := range h.st.records {
		if r.Status != models.StatusPresent {
			t.Fatalf("punched record untouched, got %s", r.Status)
		}
	}
}

// A record without punches that is on_leave but not leave-sourced (e.g. regularization) is recomputed, not deleted.
func TestLeaveSync_ClearRecomputesForeignRecords(t *testing.T) {
	h := newHarness()
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	d := syncDates("2026-10-12")[0]
	h.st.records[uuid.New()] = models.AttendanceRecord{ID: uuid.New(), TenantID: h.tenant, EmployeeProfileID: emp.ID,
		AttendanceDate: d, Status: models.StatusOnLeave, Source: models.SourceRegularization}
	for k, r := range h.st.records {
		r.ID = k
		h.st.records[k] = r
	}
	if err := NewLeaveSync(h.deps).ClearLeave(context.Background(), nil, h.tenant, emp.ID, []time.Time{d}); err != nil {
		t.Fatal(err)
	}
	for _, r := range h.st.records {
		if r.Status == models.StatusOnLeave {
			t.Fatal("status must be recomputed from punches")
		}
	}
	if len(h.st.records) != 1 {
		t.Fatal("non-leave record must not be deleted")
	}
}

func TestLeaveSync_Errors(t *testing.T) {
	ctx, boom := context.Background(), errors.New("boom")
	d := syncDates("2026-10-12")
	for _, op := range []string{"record.lock", "record.create"} {
		h := newHarness()
		h.st.fail[op] = boom
		if err := NewLeaveSync(h.deps).MarkLeave(ctx, nil, h.tenant, uuid.New(), d); !errors.Is(err, boom) {
			t.Errorf("mark %s: %v", op, err)
		}
	}
	h := newHarness()
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	sync := NewLeaveSync(h.deps)
	if err := sync.MarkLeave(ctx, nil, h.tenant, emp.ID, d); err != nil {
		t.Fatal(err)
	}
	h.st.fail["record.delete"] = boom
	if err := sync.ClearLeave(ctx, nil, h.tenant, emp.ID, d); !errors.Is(err, boom) {
		t.Errorf("clear delete: %v", err)
	}
	h.st.fail = map[string]error{"record.lock": boom}
	if err := sync.ClearLeave(ctx, nil, h.tenant, emp.ID, d); !errors.Is(err, boom) {
		t.Errorf("clear lock: %v", err)
	}
	if err := sync.ClearLeave(ctx, nil, h.tenant, emp.ID, syncDates("2026-11-02")); !errors.Is(err, boom) {
		t.Errorf("clear lock (no record): %v", err)
	}
}

// An existing unpunched record (e.g. absent) is switched to on_leave without changing its source.
func TestLeaveSync_MarkExistingUnpunched(t *testing.T) {
	h := newHarness()
	emp := h.emps.add(h.tenant, uuid.New(), "active")
	d := syncDates("2026-10-12")[0]
	id := uuid.New()
	h.st.records[id] = models.AttendanceRecord{ID: id, TenantID: h.tenant, EmployeeProfileID: emp.ID,
		AttendanceDate: d, Status: models.StatusAbsent, Source: models.SourceWeb}
	if err := NewLeaveSync(h.deps).MarkLeave(context.Background(), nil, h.tenant, emp.ID, []time.Time{d}); err != nil {
		t.Fatal(err)
	}
	if r := h.st.records[id]; r.Status != models.StatusOnLeave || r.Source != models.SourceWeb {
		t.Fatalf("got %s/%s", r.Status, r.Source)
	}
	h.st.records[id] = models.AttendanceRecord{ID: id, TenantID: h.tenant, EmployeeProfileID: emp.ID,
		AttendanceDate: d, Status: models.StatusAbsent, Source: models.SourceWeb}
	h.st.fail["record.update"] = errors.New("boom")
	if err := NewLeaveSync(h.deps).MarkLeave(context.Background(), nil, h.tenant, emp.ID, []time.Time{d}); err == nil {
		t.Fatal("update failure must surface")
	}
}
