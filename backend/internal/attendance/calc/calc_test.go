package calc

import (
	"testing"
	"time"
)

func TestParseAndFormatHHMM(t *testing.T) {
	valid := map[string]int{"00:00": 0, "09:05": 545, "23:59": 1439, "12:30": 750}
	for in, want := range valid {
		got, err := ParseHHMM(in)
		if err != nil || got != want {
			t.Errorf("ParseHHMM(%q) = %d, %v; want %d", in, got, err, want)
		}
		if FormatHHMM(want) != in {
			t.Errorf("FormatHHMM(%d) = %q; want %q", want, FormatHHMM(want), in)
		}
	}
	for _, bad := range []string{"", "9:00", "24:00", "12:60", "ab:cd", "12-30", "123:00", "12:3"} {
		if _, err := ParseHHMM(bad); err == nil {
			t.Errorf("ParseHHMM(%q) expected error", bad)
		}
	}
}

func TestShiftShape(t *testing.T) {
	if !IsNightShift(1320, 360) || IsNightShift(540, 1080) {
		t.Fatal("night detection wrong")
	}
	if SpanMinutes(540, 1080) != 540 || SpanMinutes(1320, 360) != 480 {
		t.Fatal("span wrong")
	}
	if ExpectedMinutes(nil) != DefaultExpectedMinutes {
		t.Fatal("nil shift expected default")
	}
	// Break larger than span never yields negative expectation.
	if ExpectedMinutes(&ShiftSpec{StartMinute: 540, EndMinute: 570, BreakMins: 60}) != 0 {
		t.Fatal("expected must clamp at 0")
	}
}

func TestThresholdDefaults(t *testing.T) {
	full, half := Thresholds(dayShift())
	if full != 480 || half != 240 {
		t.Fatalf("defaults = %d/%d", full, half)
	}
	full, half = Thresholds(nil)
	if full != 480 || half != 240 {
		t.Fatalf("no-shift defaults = %d/%d", full, half)
	}
}

func TestAttendanceDateNoShiftIsUTC(t *testing.T) {
	at := time.Date(2026, 10, 8, 23, 59, 0, 0, time.UTC)
	if got := AttendanceDate(at, nil); !got.Equal(time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
}

func TestAttendanceDateWindowClampedByShiftStart(t *testing.T) {
	// 23h night shift 06:00→05:00: end+4h (09:00) would pass the 06:00 start; cutoff clamps to start.
	s := &ShiftSpec{StartMinute: 360, EndMinute: 300, Location: time.UTC}
	at := time.Date(2026, 10, 9, 7, 0, 0, 0, time.UTC)
	if got := AttendanceDate(at, s); !got.Equal(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("07:00 after a 06:00 start belongs to the same day, got %v", got)
	}
}

func TestOpenSessionStart(t *testing.T) {
	now := utc(2026, 10, 8, 12, 0)
	open := []Punch{{utc(2026, 10, 8, 9, 0), In}}
	if s, ok := OpenSessionStart(open, now); !ok || !s.Equal(utc(2026, 10, 8, 9, 0)) {
		t.Fatal("expected open session")
	}
	closed := append(open, Punch{utc(2026, 10, 8, 11, 0), Out})
	if _, ok := OpenSessionStart(closed, now); ok {
		t.Fatal("closed session reported open")
	}
	if _, ok := OpenSessionStart(open, utc(2026, 10, 9, 6, 0)); ok {
		t.Fatal("session older than 20h must be abandoned")
	}
	if _, ok := OpenSessionStart(nil, now); ok {
		t.Fatal("no punches reported open")
	}
}
