package rrule

// Golden tests (protected, docs/ENGINEERING.md section 8; docs/modules/m3-meetings/testing.md): the expected instants
// were checked by hand against RFC 5545 section 3.8.5.3 examples and real calendars.

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %s unavailable: %v", name, err)
	}
	return loc
}

func expand(t *testing.T, rule string, start time.Time, end time.Time, limit int) []string {
	t.Helper()
	r, err := Parse(rule)
	if err != nil {
		t.Fatalf("%s: %v", rule, err)
	}
	var out []string
	for _, x := range r.Expand(start, end, limit) {
		out = append(out, x.Format("2006-01-02 15:04 MST"))
	}
	return out
}

func same(t *testing.T, name string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s\n got: %v\nwant: %v", name, got, want)
	}
}

func TestGoldenWeeklyKeepsWallClockAcrossEuropeanDST(t *testing.T) {
	berlin := zone(t, "Europe/Berlin")
	start := time.Date(2027, 3, 16, 9, 0, 0, 0, berlin) // a Tuesday; DST starts Sun 28 March 2027
	got := expand(t, "FREQ=WEEKLY;BYDAY=TU;COUNT=3", start, start.AddDate(1, 0, 0), 100)
	same(t, "Berlin", got, "2027-03-16 09:00 CET", "2027-03-23 09:00 CET", "2027-03-30 09:00 CEST")
	r, _ := Parse("FREQ=WEEKLY;COUNT=3")
	in := r.Expand(start, start.AddDate(1, 0, 0), 10)
	if d := in[2].Sub(in[1]); d != 7*24*time.Hour-time.Hour {
		t.Fatalf("the UTC gap shrinks by the DST hour: %v", d)
	}
}

func TestGoldenUSFallBackAndSpringForwardGap(t *testing.T) {
	ny := zone(t, "America/New_York")
	start := time.Date(2027, 10, 29, 9, 30, 0, 0, ny) // Friday; DST ends Sun 7 November 2027
	same(t, "fall back", expand(t, "FREQ=WEEKLY;COUNT=2", start, start.AddDate(0, 2, 0), 10),
		"2027-10-29 09:30 EDT", "2027-11-05 09:30 EDT")
	same(t, "after", expand(t, "FREQ=DAILY;INTERVAL=3;COUNT=4", start, start.AddDate(0, 2, 0), 10),
		"2027-10-29 09:30 EDT", "2027-11-01 09:30 EDT", "2027-11-04 09:30 EDT", "2027-11-07 09:30 EST")
	gap := time.Date(2027, 3, 13, 2, 30, 0, 0, ny) // 02:30 does not exist on 14 March 2027
	got := expand(t, "FREQ=DAILY;COUNT=2", gap, gap.AddDate(0, 1, 0), 10)
	same(t, "spring gap is normalised forward", got, "2027-03-13 02:30 EST", "2027-03-14 03:30 EDT")
}

func TestGoldenMonthlyOrdinalsAndMonthEnds(t *testing.T) {
	utc := time.UTC
	start := time.Date(2027, 1, 1, 10, 0, 0, 0, utc)
	same(t, "last Friday", expand(t, "FREQ=MONTHLY;BYDAY=-1FR;COUNT=3", start, start.AddDate(1, 0, 0), 10),
		"2027-01-29 10:00 UTC", "2027-02-26 10:00 UTC", "2027-03-26 10:00 UTC")
	same(t, "second Monday", expand(t, "FREQ=MONTHLY;BYDAY=2MO;COUNT=3", start, start.AddDate(1, 0, 0), 10),
		"2027-01-11 10:00 UTC", "2027-02-08 10:00 UTC", "2027-03-08 10:00 UTC")
	jan31 := time.Date(2027, 1, 31, 10, 0, 0, 0, utc)
	same(t, "the 31st skips short months", expand(t, "FREQ=MONTHLY;COUNT=3", jan31, jan31.AddDate(1, 0, 0), 10),
		"2027-01-31 10:00 UTC", "2027-03-31 10:00 UTC", "2027-05-31 10:00 UTC")
	same(t, "last day of month", expand(t, "FREQ=MONTHLY;BYMONTHDAY=-1;COUNT=3", start, start.AddDate(1, 0, 0), 10),
		"2027-01-31 10:00 UTC", "2027-02-28 10:00 UTC", "2027-03-31 10:00 UTC")
	same(t, "Friday the 13th", expand(t, "FREQ=MONTHLY;BYDAY=FR;BYMONTHDAY=13;COUNT=2", start, start.AddDate(3, 0, 0), 10),
		"2027-08-13 10:00 UTC", "2028-10-13 10:00 UTC")
}

func TestGoldenYearlyWeeklyAndBounds(t *testing.T) {
	utc := time.UTC
	leap := time.Date(2028, 2, 29, 8, 0, 0, 0, utc)
	same(t, "29 Feb only in leap years", expand(t, "FREQ=YEARLY;COUNT=2", leap, leap.AddDate(10, 0, 0), 10),
		"2028-02-29 08:00 UTC", "2032-02-29 08:00 UTC")
	thanks := time.Date(2027, 1, 1, 12, 0, 0, 0, utc)
	same(t, "fourth Thursday of November", expand(t, "FREQ=YEARLY;BYMONTH=11;BYDAY=4TH;COUNT=2", thanks, thanks.AddDate(3, 0, 0), 10),
		"2027-11-25 12:00 UTC", "2028-11-23 12:00 UTC")
	mon := time.Date(2027, 1, 4, 9, 0, 0, 0, utc) // Monday
	same(t, "biweekly Mon and Thu", expand(t, "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH;COUNT=4", mon, mon.AddDate(1, 0, 0), 10),
		"2027-01-04 09:00 UTC", "2027-01-07 09:00 UTC", "2027-01-18 09:00 UTC", "2027-01-21 09:00 UTC")
	same(t, "UNTIL inclusive", expand(t, "FREQ=DAILY;UNTIL=20270106T090000Z", mon, mon.AddDate(1, 0, 0), 10),
		"2027-01-04 09:00 UTC", "2027-01-05 09:00 UTC", "2027-01-06 09:00 UTC")
	same(t, "weekdays", expand(t, "FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR;COUNT=6", mon, mon.AddDate(1, 0, 0), 10),
		"2027-01-04 09:00 UTC", "2027-01-05 09:00 UTC", "2027-01-06 09:00 UTC", "2027-01-07 09:00 UTC",
		"2027-01-08 09:00 UTC", "2027-01-11 09:00 UTC")
}

func TestGoldenOpenEndedWindowAndLimit(t *testing.T) {
	start := time.Date(2027, 1, 4, 9, 0, 0, 0, time.UTC)
	r, _ := Parse("FREQ=WEEKLY")
	if got := r.Expand(start, start.AddDate(1, 0, 0), 1000); len(got) != 53 || r.Bounded() {
		t.Fatalf("an open-ended weekly series fills a 12-month window: %d", len(got))
	}
	if got := r.Expand(start, start.AddDate(1, 0, 0), 10); len(got) != 10 {
		t.Fatalf("limit caps the expansion: %d", len(got))
	}
	d, _ := Parse("FREQ=DAILY;BYMONTH=2;BYMONTHDAY=30")
	if got := d.Expand(start, start.AddDate(1, 0, 0), 10); len(got) != 0 {
		t.Fatalf("an impossible rule yields nothing and terminates: %v", got)
	}
}

func TestRuleValidationAndCanonicalForm(t *testing.T) {
	bad := []string{"", "FREQ=HOURLY", "FREQ=SECONDLY", "FREQ=WEEKLY;BYSETPOS=1", "FREQ=WEEKLY;COUNT=501",
		"FREQ=WEEKLY;INTERVAL=0", "FREQ=WEEKLY;INTERVAL=100", "FREQ=WEEKLY;COUNT=2;UNTIL=20270101", "FREQ=WEEKLY;BYDAY=2MO",
		"FREQ=YEARLY;BYDAY=1MO", "FREQ=MONTHLY;BYDAY=6MO", "FREQ=MONTHLY;BYDAY=XX", "FREQ=WEEKLY;BYMONTHDAY=3",
		"FREQ=DAILY;BYMONTHDAY=0", "FREQ=DAILY;BYMONTH=13", "FREQ=DAILY;UNTIL=tomorrow", "FREQ=DAILY;FREQ=WEEKLY",
		"FREQ=DAILY;WKST=XY", "FREQ", "FREQ=DAILY;BYHOUR=9", strings.Repeat("A", 501)}
	for _, b := range bad {
		if _, err := Parse(b); !errors.Is(err, ErrRule) {
			t.Errorf("%q must be refused, got %v", b, err)
		}
	}
	r, err := Parse("RRULE:freq=monthly;bymonthday=1,15;interval=2;byday=MO;bymonth=1,7;wkst=SU;count=5")
	if err != nil {
		t.Fatal(err)
	}
	want := "FREQ=MONTHLY;INTERVAL=2;COUNT=5;BYDAY=MO;BYMONTHDAY=1,15;BYMONTH=1,7;WKST=SU"
	if r.String() != want {
		t.Fatalf("canonical form: %s", r.String())
	}
	u, _ := Parse("FREQ=MONTHLY;BYDAY=-1FR;UNTIL=20270301")
	if u.String() != "FREQ=MONTHLY;UNTIL=20270301T235959Z;BYDAY=-1FR" || !u.Bounded() {
		t.Fatalf("date UNTIL covers the day: %s", u.String())
	}
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := u.CheckUntil(start); err != nil {
		t.Fatal(err)
	}
	far, _ := Parse("FREQ=DAILY;UNTIL=20330101")
	if err := far.CheckUntil(start); !errors.Is(err, ErrRule) {
		t.Fatal("UNTIL beyond five years is refused")
	}
	if err := u.CheckUntil(start.AddDate(1, 0, 0)); !errors.Is(err, ErrRule) {
		t.Fatal("UNTIL before the start is refused")
	}
}
