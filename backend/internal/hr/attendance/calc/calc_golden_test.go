package calc

import (
	"testing"
	"time"
)

// GOLDEN — protected (golden-tests.md G4/G5). Never edit expectations to make red pass.

func utc(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.UTC)
}

func intPtr(i int) *int { return &i }

func dayShift() *ShiftSpec {
	return &ShiftSpec{StartMinute: 9 * 60, EndMinute: 18 * 60, GraceMins: 15, BreakMins: 60, Location: time.UTC}
}

// TestGolden_G4_Totals freezes AT-006..AT-010 on a 09:00–18:00 UTC shift (expected 480).
func TestGolden_G4_Totals(t *testing.T) {
	d := 8
	cases := []struct {
		name    string
		shift   *ShiftSpec
		punches []Punch
		now     time.Time
		want    Totals
	}{
		{"on time full day", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}, {utc(2026, 10, d, 18, 0), Out}}, utc(2026, 10, d, 18, 1),
			Totals{WorkMinutes: 540, OvertimeMinutes: 60, Status: StatusPresent}},
		{"late after grace", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 20), In}, {utc(2026, 10, d, 18, 0), Out}}, utc(2026, 10, d, 18, 1),
			Totals{WorkMinutes: 520, LateMinutes: 5, OvertimeMinutes: 40, Status: StatusPresent}},
		{"inside grace", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 14), In}, {utc(2026, 10, d, 18, 0), Out}}, utc(2026, 10, d, 18, 1),
			Totals{WorkMinutes: 526, OvertimeMinutes: 46, Status: StatusPresent}},
		{"lunch break", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}, {utc(2026, 10, d, 13, 0), Out}, {utc(2026, 10, d, 14, 0), In}, {utc(2026, 10, d, 18, 0), Out}}, utc(2026, 10, d, 18, 1),
			Totals{WorkMinutes: 480, BreakMinutes: 60, Status: StatusPresent}},
		{"half day", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}, {utc(2026, 10, d, 13, 30), Out}}, utc(2026, 10, d, 14, 0),
			Totals{WorkMinutes: 270, Status: StatusHalfDay}},
		{"below half day", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}, {utc(2026, 10, d, 10, 0), Out}}, utc(2026, 10, d, 10, 5),
			Totals{WorkMinutes: 60, Status: StatusAbsent}},
		{"open session counts present", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}}, utc(2026, 10, d, 12, 0),
			Totals{OpenSession: true, Status: StatusPresent}},
		{"abandoned session after 20h", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}}, utc(2026, 10, d+1, 9, 0),
			Totals{Status: StatusAbsent}},
		{"no shift uses 480 and no lateness", nil,
			[]Punch{{utc(2026, 10, d, 10, 0), In}, {utc(2026, 10, d, 18, 0), Out}}, utc(2026, 10, d, 18, 1),
			Totals{WorkMinutes: 480, Status: StatusPresent}},
		{"custom thresholds", &ShiftSpec{StartMinute: 540, EndMinute: 1080, GraceMins: 15, BreakMins: 60, FullDayMinutes: intPtr(450), HalfDayMinutes: intPtr(200), Location: time.UTC},
			[]Punch{{utc(2026, 10, d, 9, 0), In}, {utc(2026, 10, d, 16, 35), Out}}, utc(2026, 10, d, 17, 0),
			Totals{WorkMinutes: 455, Status: StatusPresent}},
		{"unsorted input + seconds floored", dayShift(),
			[]Punch{{time.Date(2026, 10, d, 9, 59, 59, 0, time.UTC), Out}, {time.Date(2026, 10, d, 9, 0, 30, 0, time.UTC), In}}, utc(2026, 10, d, 10, 5),
			Totals{WorkMinutes: 59, Status: StatusAbsent}},
		{"double in drops the dangling first", dayShift(),
			[]Punch{{utc(2026, 10, d, 9, 0), In}, {utc(2026, 10, d, 9, 30), In}, {utc(2026, 10, d, 18, 0), Out}}, utc(2026, 10, d, 18, 1),
			Totals{WorkMinutes: 510, OvertimeMinutes: 30, Status: StatusPresent}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeTotals(tc.punches, tc.shift, utc(2026, 10, d, 0, 0), tc.now)
			got.FirstIn, got.LastOut = nil, nil
			if got != tc.want {
				t.Fatalf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// TestGolden_G4_FirstLast freezes first_punch_in / last_punch_out selection.
func TestGolden_G4_FirstLast(t *testing.T) {
	p := []Punch{{utc(2026, 10, 8, 13, 0), Out}, {utc(2026, 10, 8, 9, 0), In}, {utc(2026, 10, 8, 14, 0), In}, {utc(2026, 10, 8, 18, 0), Out}}
	got := ComputeTotals(p, dayShift(), utc(2026, 10, 8, 0, 0), utc(2026, 10, 8, 19, 0))
	if got.FirstIn == nil || !got.FirstIn.Equal(utc(2026, 10, 8, 9, 0)) {
		t.Fatalf("first in = %v", got.FirstIn)
	}
	if got.LastOut == nil || !got.LastOut.Equal(utc(2026, 10, 8, 18, 0)) {
		t.Fatalf("last out = %v", got.LastOut)
	}
	empty := ComputeTotals(nil, dayShift(), utc(2026, 10, 8, 0, 0), utc(2026, 10, 8, 19, 0))
	if empty.FirstIn != nil || empty.LastOut != nil || empty.Status != StatusAbsent {
		t.Fatalf("empty punches = %+v", empty)
	}
}

// TestGolden_G5_NightShift freezes cross-midnight attribution and totals in Asia/Kolkata.
func TestGolden_G5_NightShift(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("tz: %v", err)
	}
	night := &ShiftSpec{StartMinute: 22 * 60, EndMinute: 6 * 60, GraceMins: 15, BreakMins: 60, Location: ist}
	dayD := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

	in := time.Date(2026, 10, 8, 21, 55, 0, 0, ist)
	out := time.Date(2026, 10, 9, 6, 10, 0, 0, ist)
	if got := AttendanceDate(in, night); !got.Equal(dayD) {
		t.Fatalf("IN 21:55 D attributed to %v, want %v", got, dayD)
	}
	if got := AttendanceDate(time.Date(2026, 10, 9, 1, 0, 0, 0, ist), night); !got.Equal(dayD) {
		t.Fatalf("IN 01:00 D+1 attributed to %v, want D", got)
	}
	if got := AttendanceDate(time.Date(2026, 10, 9, 12, 0, 0, 0, ist), night); !got.Equal(dayD.AddDate(0, 0, 1)) {
		t.Fatalf("IN 12:00 D+1 attributed to %v, want D+1", got)
	}

	tot := ComputeTotals([]Punch{{in, In}, {out, Out}}, night, dayD, out.Add(time.Minute))
	want := Totals{WorkMinutes: 495, OvertimeMinutes: 75, Status: StatusPresent}
	tot.FirstIn, tot.LastOut = nil, nil
	if tot != want {
		t.Fatalf("night totals %+v, want %+v", tot, want)
	}
	late := ComputeTotals([]Punch{{time.Date(2026, 10, 9, 1, 0, 0, 0, ist), In}}, night, dayD, time.Date(2026, 10, 9, 2, 0, 0, 0, ist))
	if late.LateMinutes != 165 {
		t.Fatalf("late for 01:00 on 22:00+15 shift = %d, want 165", late.LateMinutes)
	}
	if ExpectedMinutes(night) != 420 {
		t.Fatalf("night expected = %d, want 420", ExpectedMinutes(night))
	}
}

// TestGolden_G5_TimeZoneLateness freezes local-time lateness across a DST change.
func TestGolden_G5_TimeZoneLateness(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("tz: %v", err)
	}
	shift := &ShiftSpec{StartMinute: 9 * 60, EndMinute: 17 * 60, GraceMins: 0, BreakMins: 0, Location: ny}
	// 2026-03-08 is the US DST start: 09:00 EDT = 13:00 UTC.
	date := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	in := time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC)
	if got := AttendanceDate(in, shift); !got.Equal(date) {
		t.Fatalf("date = %v", got)
	}
	tot := ComputeTotals([]Punch{{in, In}}, shift, date, in.Add(time.Hour))
	if tot.LateMinutes != 0 {
		t.Fatalf("09:00 EDT must be on time, late = %d", tot.LateMinutes)
	}
	tot = ComputeTotals([]Punch{{in.Add(10 * time.Minute), In}}, shift, date, in.Add(time.Hour))
	if tot.LateMinutes != 10 {
		t.Fatalf("09:10 EDT late = %d, want 10", tot.LateMinutes)
	}
	// Late punch 23:30 local stays on the local date even though it is the next UTC day.
	if got := AttendanceDate(time.Date(2026, 3, 8, 23, 30, 0, 0, ny), shift); !got.Equal(date) {
		t.Fatalf("23:30 local attributed to %v", got)
	}
}
