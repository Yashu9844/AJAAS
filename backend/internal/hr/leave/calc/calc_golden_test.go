package calc

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestGolden_G2_CountDays — golden G2 (LV-006). 2026-10-05 is a Monday.
func TestGolden_G2_CountDays(t *testing.T) {
	holiday := []time.Time{d("2026-10-07")} // Wednesday
	cases := []struct {
		name         string
		in           CountInput
		total        Days
		workingDates int
	}{
		{"Mon–Fri", CountInput{Start: d("2026-10-05"), End: d("2026-10-09")}, 500, 5},
		{"Fri–Mon no sandwich", CountInput{Start: d("2026-10-09"), End: d("2026-10-12")}, 200, 2},
		{"Fri–Mon sandwich", CountInput{Start: d("2026-10-09"), End: d("2026-10-12"), Sandwich: true}, 400, 2},
		{"Sat–Sun only", CountInput{Start: d("2026-10-10"), End: d("2026-10-11")}, 0, 0},
		{"Sat–Sun sandwich still zero", CountInput{Start: d("2026-10-10"), End: d("2026-10-11"), Sandwich: true}, 0, 0},
		{"holiday inside Mon–Fri", CountInput{Start: d("2026-10-05"), End: d("2026-10-09"), Holidays: holiday}, 400, 4},
		{"holiday inside, sandwich counts it", CountInput{Start: d("2026-10-05"), End: d("2026-10-09"), Holidays: holiday, Sandwich: true}, 500, 4},
		{"leading/trailing weekend never sandwiched", CountInput{Start: d("2026-10-03"), End: d("2026-10-11"), Sandwich: true}, 500, 5},
		{"half day on working day", CountInput{Start: d("2026-10-06"), End: d("2026-10-06"), Half: true}, 50, 0},
		{"half day on holiday", CountInput{Start: d("2026-10-07"), End: d("2026-10-07"), Half: true, Holidays: holiday}, 0, 0},
		{"single holiday", CountInput{Start: d("2026-10-07"), End: d("2026-10-07"), Holidays: holiday}, 0, 0},
	}
	for _, tc := range cases {
		got := CountDays(tc.in)
		if got.Total != tc.total || len(got.WorkingDates) != tc.workingDates {
			t.Errorf("%s: total %s dates %d, want %s %d", tc.name, got.Total, len(got.WorkingDates), tc.total, tc.workingDates)
		}
	}
}

// TestGolden_G3_Accrual — golden G3 (FR-BL002).
func TestGolden_G3_Accrual(t *testing.T) {
	jul1, nextYear := d("2026-07-01"), d("2027-02-01")
	cases := []struct {
		name    string
		policy  Policy
		joining *time.Time
		asOf    time.Time
		want    Days
	}{
		{"annual 12, joined 1 Jul", Policy{Allowance: 1200}, &jul1, d("2026-08-15"), 600},
		{"annual 12, joined before year", Policy{Allowance: 1200}, nil, d("2026-01-02"), 1200},
		{"monthly 18 in March", Policy{Allowance: 1800, Monthly: true}, nil, d("2026-03-31"), 450},
		{"monthly 15 in December (remainder)", Policy{Allowance: 1500, Monthly: true}, nil, d("2026-12-01"), 1500},
		{"monthly 10 in January", Policy{Allowance: 1000, Monthly: true}, nil, d("2026-01-20"), 83},
		{"monthly 12, joined 1 Jul, asOf Aug", Policy{Allowance: 1200, Monthly: true}, &jul1, d("2026-08-15"), 200},
		{"monthly, joined after asOf", Policy{Allowance: 1200, Monthly: true}, &jul1, d("2026-03-01"), 0},
		{"joined next year", Policy{Allowance: 1200}, &nextYear, d("2026-06-01"), 0},
		{"monthly, past year fully accrued", Policy{Allowance: 1200, Monthly: true}, nil, d("2027-03-01"), 1200},
		{"monthly, future year nothing yet", Policy{Allowance: 1200, Monthly: true}, nil, d("2025-12-31"), 0},
		{"zero allowance", Policy{}, nil, d("2026-06-01"), 0},
	}
	for _, tc := range cases {
		if got := Accrued(tc.policy, tc.joining, 2026, tc.asOf); got != tc.want {
			t.Errorf("%s: accrued %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestGolden_G4_CarryForward — golden G4 (FR-BL003).
func TestGolden_G4_CarryForward(t *testing.T) {
	cases := []struct{ prev, limit, want Days }{{800, 500, 500}, {-200, 500, 0}, {800, 0, 0}, {300, 500, 300}}
	for _, tc := range cases {
		if got := CarryForward(tc.prev, tc.limit); got != tc.want {
			t.Errorf("CarryForward(%s, %s) = %s, want %s", tc.prev, tc.limit, got, tc.want)
		}
	}
}

func TestDateHelpers(t *testing.T) {
	now := time.Date(2026, 10, 9, 23, 30, 0, 0, time.FixedZone("IST", 19800))
	if got := DateOf(now); !got.Equal(d("2026-10-09")) {
		t.Errorf("DateOf keeps the instant's UTC date: %v", got)
	}
	if got := DateOf(time.Date(2026, 10, 10, 1, 0, 0, 0, time.FixedZone("IST", 19800))); !got.Equal(d("2026-10-09")) {
		t.Errorf("DateOf converts to UTC first: %v", got)
	}
	if !IsWeekend(d("2026-10-10")) || !IsWeekend(d("2026-10-11")) || IsWeekend(d("2026-10-12")) {
		t.Error("IsWeekend")
	}
	if got := DaysBetween(d("2026-10-05"), d("2026-10-09")); got != 4 {
		t.Errorf("DaysBetween = %d", got)
	}
}
