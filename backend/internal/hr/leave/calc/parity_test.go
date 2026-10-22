package calc

import (
	"testing"
	"time"
)

func dt(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

// TestAccruedFromMatchesCalendarYear pins that startMonth 1 equals the original calendar-year formula (LG1).
func TestAccruedFromMatchesCalendarYear(t *testing.T) {
	join := dt(2026, time.March, 10)
	for _, p := range []Policy{{Allowance: FromWhole(12)}, {Allowance: FromWhole(12), Monthly: true}, {Allowance: 1000, Monthly: true}} {
		for _, asOf := range []time.Time{dt(2025, 12, 31), dt(2026, 1, 5), dt(2026, 6, 30), dt(2026, 12, 31), dt(2027, 3, 1)} {
			for _, j := range []*time.Time{nil, &join} {
				if got, want := AccruedFrom(p, j, 2026, 1, asOf), Accrued(p, j, 2026, asOf); got != want {
					t.Fatalf("%+v join=%v asOf=%v: %v != %v", p, j, asOf, got, want)
				}
			}
		}
	}
}

// TestAccruedFromAprilYear freezes LG1 for an April–March leave year.
func TestAccruedFromAprilYear(t *testing.T) {
	annual, monthly := Policy{Allowance: FromWhole(12)}, Policy{Allowance: FromWhole(12), Monthly: true}
	cases := []struct {
		name   string
		p      Policy
		join   *time.Time
		year   int
		asOf   time.Time
		expect Days
	}{
		{"annual credits the whole year up front", annual, nil, 2026, dt(2026, 4, 1), FromWhole(12)},
		{"monthly before the year starts", monthly, nil, 2026, dt(2026, 3, 31), 0},
		{"monthly in the first month", monthly, nil, 2026, dt(2026, 4, 20), FromWhole(1)},
		{"monthly in February credits eleven months", monthly, nil, 2026, dt(2027, 2, 3), FromWhole(11)},
		{"monthly after the year lands on the allowance", monthly, nil, 2026, dt(2027, 9, 1), FromWhole(12)},
		{"joined in August, annual", annual, ptr(dt(2026, 8, 5)), 2026, dt(2026, 8, 5), FromWhole(8)},
		{"joined in January 2027 is month 10 of year 2026", annual, ptr(dt(2027, 1, 5)), 2026, dt(2027, 1, 5), FromWhole(3)},
		{"joined after the year ends", annual, ptr(dt(2027, 4, 1)), 2026, dt(2027, 4, 1), 0},
		{"joined before the year, monthly at September", monthly, ptr(dt(2020, 1, 1)), 2026, dt(2026, 9, 30), FromWhole(6)},
	}
	for _, c := range cases {
		if got := AccruedFrom(c.p, c.join, c.year, 4, c.asOf); got != c.expect {
			t.Errorf("%s: got %v want %v", c.name, got, c.expect)
		}
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestYearOf(t *testing.T) {
	for _, c := range []struct {
		date  time.Time
		start int
		want  int
	}{{dt(2026, 5, 10), 4, 2026}, {dt(2027, 2, 1), 4, 2026}, {dt(2026, 3, 31), 4, 2025}, {dt(2026, 4, 1), 4, 2026}, {dt(2026, 1, 1), 1, 2026}, {dt(2026, 12, 31), 1, 2026}, {dt(2026, 2, 1), 0, 2026}, {dt(2026, 2, 1), 13, 2026}} {
		if got := YearOf(c.date, c.start); got != c.want {
			t.Errorf("YearOf(%v, %d) = %d want %d", c.date, c.start, got, c.want)
		}
	}
}

// TestCountDaysWeekOffOverride freezes LG2: a Friday/Saturday weekly off prices a Sunday as a working day.
func TestCountDaysWeekOffOverride(t *testing.T) {
	friSat := func(day time.Time) bool { return day.Weekday() == time.Friday || day.Weekday() == time.Saturday }
	// 2026-06-07 is a Sunday; the range Sun..Sat has five working days with a Fri/Sat weekly off.
	in := CountInput{Start: dt(2026, 6, 7), End: dt(2026, 6, 13), WeekOff: friSat}
	if got := CountDays(in).Total; got != FromWhole(5) {
		t.Fatalf("Fri/Sat off: %v", got)
	}
	in.WeekOff = nil
	if got := CountDays(in).Total; got != FromWhole(5) {
		t.Fatalf("Sat/Sun off: %v", got)
	}
	in = CountInput{Start: dt(2026, 6, 13), End: dt(2026, 6, 14), WeekOff: friSat}
	if got := CountDays(in).Total; got != FromWhole(1) {
		t.Fatalf("Sunday works with Fri/Sat off: %v", got)
	}
	in.WeekOff = nil
	if got := CountDays(in).Total; got != 0 {
		t.Fatalf("weekend by default: %v", got)
	}
}
