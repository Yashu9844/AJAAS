package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
)

func punch(t *testing.T, h *harness, user uuid.UUID, typ string) (*dto.PunchResult, error) {
	t.Helper()
	lat, lng, dev := 12.97, 77.59, "kiosk-1"
	return NewPunchService(h.deps).Punch(context.Background(), h.actor(user),
		dto.PunchRequest{Type: typ, Latitude: &lat, Longitude: &lng, DeviceID: &dev})
}

// TestGolden_G2_PunchAlternation — protected (G2, AT-003).
func TestGolden_G2_PunchAlternation(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	steps := []struct {
		typ  string
		code string
	}{{"in", ""}, {"in", "ALREADY_PUNCHED_IN"}, {"out", ""}, {"out", "NOT_PUNCHED_IN"}}
	for i, s := range steps {
		h.advance(2 * time.Minute)
		_, err := punch(t, h, user, s.typ)
		if codeOf(err) != s.code || (s.code == "" && err != nil) {
			t.Fatalf("step %d %s: got %v want %q", i, s.typ, err, s.code)
		}
	}
	if len(h.st.punches) != 2 || len(h.st.records) != 1 {
		t.Fatalf("rejected punches must not persist: punches=%d records=%d", len(h.st.punches), len(h.st.records))
	}
}

// TestGolden_G3_ServerClockOnly — protected (G3, AT-001): client time cannot reach the punch.
func TestGolden_G3_ServerClockOnly(t *testing.T) {
	var req dto.PunchRequest
	if err := json.Unmarshal([]byte(`{"type":"in","punch_time":"2020-01-01T00:00:00Z"}`), &req); err != nil {
		t.Fatal(err)
	}
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	res, err := NewPunchService(h.deps).Punch(context.Background(), h.actor(user), req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Punch.PunchTime.Equal(h.now) || res.Record.AttendanceDate != "2026-10-08" {
		t.Fatalf("punch must use server clock %v, got %v / %s", h.now, res.Punch.PunchTime, res.Record.AttendanceDate)
	}
}

// TestGolden_G11_PunchAudit — protected (G11): audit per state change, no IP/coords in metadata.
func TestGolden_G11_PunchAudit(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Hour)
	if _, err := punch(t, h, user, "out"); err != nil {
		t.Fatalf("audit failure must not fail the request: %v", err)
	}
	if got := h.audit.actions(); len(got) != 2 || got[0] != "attendance.punch_in" || got[1] != "attendance.punch_out" {
		t.Fatalf("audit actions = %v", got)
	}
	for _, e := range h.audit.entries {
		raw, _ := json.Marshal(e.metadata)
		for _, banned := range []string{"203.0.113.7", "12.97", "kiosk"} {
			if strings.Contains(string(raw), banned) {
				t.Errorf("audit metadata leaks %q: %s", banned, raw)
			}
		}
		if e.ip != "203.0.113.7" {
			t.Errorf("IP belongs in the audit column, got %q", e.ip)
		}
	}
}

// TestGolden_G12_OutboxAtomicAndPrivate — protected (G12, FR-EV002/FR-EV003).
func TestGolden_G12_OutboxAtomicAndPrivate(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	if len(h.st.outbox) != 1 || len(h.pub.keys) != 1 || h.pub.keys[0] != "attendance.punch.in" {
		t.Fatalf("outbox=%d published=%v", len(h.st.outbox), h.pub.keys)
	}
	for _, row := range h.st.outbox {
		if !row.Published {
			t.Error("successful publish must mark the row published")
		}
		for _, banned := range []string{"203.0.113.7", "latitude", "kiosk", "device"} {
			if strings.Contains(row.Payload, banned) {
				t.Errorf("event payload leaks %q: %s", banned, row.Payload)
			}
		}
	}
	// Broker down: row stays for the relay, API still succeeds.
	h.pub.fail = true
	h.advance(time.Hour)
	if _, err := punch(t, h, user, "out"); err != nil {
		t.Fatalf("queue down must never block the API: %v", err)
	}
	unpublished := 0
	for _, row := range h.st.outbox {
		if !row.Published {
			unpublished++
		}
	}
	if unpublished != 1 {
		t.Fatalf("expected 1 unpublished row for the relay, got %d", unpublished)
	}
	// Failure inside the tx rolls back punch + record + outbox together.
	h.pub.fail = false
	h.advance(time.Hour)
	h.st.fail["record.update"] = errors.New("db down")
	before := len(h.st.outbox)
	if _, err := punch(t, h, user, "in"); err == nil {
		t.Fatal("expected failure")
	}
	if len(h.st.outbox) != before || len(h.st.punches) != 2 {
		t.Fatalf("rollback failed: outbox %d→%d punches=%d", before, len(h.st.outbox), len(h.st.punches))
	}
}

func TestPunch_EmployeeRules(t *testing.T) {
	h := newHarness()
	if _, err := punch(t, h, uuid.New(), "in"); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("EC-01: %v", err)
	}
	gone := uuid.New()
	h.emps.add(h.tenant, gone, "terminated")
	if _, err := punch(t, h, gone, "in"); codeOf(err) != "EMPLOYEE_NOT_ACTIVE" {
		t.Fatalf("EC-02: %v", err)
	}
	notice := uuid.New()
	h.emps.add(h.tenant, notice, "notice")
	if _, err := punch(t, h, notice, "in"); err != nil {
		t.Fatalf("notice-period employees may punch: %v", err)
	}
	h.emps.err = errors.New("module 2 down")
	if _, err := punch(t, h, notice, "out"); err == nil || codeOf(err) != "" {
		t.Fatalf("directory failure must fail closed with raw error, got %v", err)
	}
}

func TestPunch_DebounceAndExpiry(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	h.advance(30 * time.Second)
	if _, err := punch(t, h, user, "out"); codeOf(err) != "DUPLICATE_PUNCH" {
		t.Fatalf("EC-04 AT-004: %v", err)
	}
	h.advance(21 * time.Hour)
	if _, err := punch(t, h, user, "out"); codeOf(err) != "SESSION_EXPIRED" {
		t.Fatalf("AT-006 out after 20h: %v", err)
	}
	res, err := punch(t, h, user, "in")
	if err != nil || res.Record.AttendanceDate != "2026-10-09" {
		t.Fatalf("EC-05 new IN after abandoned session: %+v %v", res, err)
	}
	if len(h.st.records) != 2 {
		t.Fatalf("expected a new record for the new day, have %d", len(h.st.records))
	}
}

func TestPunch_NightShiftThroughService(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	emp := h.emps.add(h.tenant, user, "active")
	shift := createShift(t, h, dto.CreateShiftRequest{Name: "Night", StartTime: "22:00", EndTime: "06:00", Timezone: "Asia/Kolkata"})
	if _, err := NewAssignmentService(h.deps).Assign(context.Background(), h.actor(uuid.New()), uuid.MustParse(shift.ID),
		dto.AssignShiftRequest{EmployeeID: emp.ID.String(), EffectiveFrom: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	ist, _ := time.LoadLocation("Asia/Kolkata")
	h.now = time.Date(2026, 10, 8, 21, 55, 0, 0, ist)
	in, err := punch(t, h, user, "in")
	if err != nil || in.Record.AttendanceDate != "2026-10-08" || in.Record.ShiftID == nil {
		t.Fatalf("night IN: %+v %v", in, err)
	}
	h.now = time.Date(2026, 10, 9, 6, 10, 0, 0, ist)
	out, err := punch(t, h, user, "out")
	if err != nil {
		t.Fatal(err)
	}
	r := out.Record
	if r.AttendanceDate != "2026-10-08" || r.TotalWorkMinutes != 495 || r.LateMinutes != 0 || r.OvertimeMinutes != 75 || r.Status != models.StatusPresent {
		t.Fatalf("EC-06 night record: %+v", r)
	}
}

func TestPunch_PersistsLocationButNeverReturnsIP(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	res, err := punch(t, h, user, "in")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "203.0.113.7") || !strings.Contains(string(raw), "kiosk-1") {
		t.Fatalf("response: %s", raw)
	}
	for _, p := range h.st.punches {
		if p.IPAddress == nil || *p.IPAddress != "203.0.113.7" || p.Source != "web" {
			t.Fatalf("punch must store IP for audit and default source web: %+v", p)
		}
	}
	h.st.fail["record.lock"] = errors.New("lock timeout")
	h.advance(time.Hour)
	if _, err := punch(t, h, user, "out"); err == nil {
		t.Fatal("lock failure must propagate")
	}
}
