package rrule

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxPeriods bounds the number of periods (days, weeks, months or years) one expansion walks.
const maxPeriods = 20000

var dayNames = [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// String renders the rule in canonical form (parts in a fixed order).
func (r Rule) String() string {
	parts := []string{"FREQ=" + r.Freq}
	if r.Interval > 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(r.Interval))
	}
	if r.Count > 0 {
		parts = append(parts, "COUNT="+strconv.Itoa(r.Count))
	}
	if r.Until != nil {
		parts = append(parts, "UNTIL="+r.Until.UTC().Format(untilLayout))
	}
	if len(r.ByDay) > 0 {
		days := make([]string, len(r.ByDay))
		for i, d := range r.ByDay {
			days[i] = dayNames[d.Day]
			if d.N != 0 {
				days[i] = strconv.Itoa(d.N) + days[i]
			}
		}
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	}
	if len(r.ByMonthDay) > 0 {
		parts = append(parts, "BYMONTHDAY="+joinInts(r.ByMonthDay))
	}
	if len(r.ByMonth) > 0 {
		parts = append(parts, "BYMONTH="+joinInts(r.ByMonth))
	}
	if r.WeekStart != time.Monday {
		parts = append(parts, "WKST="+dayNames[r.WeekStart])
	}
	return strings.Join(parts, ";")
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}

// Bounded reports whether the rule ends (COUNT or UNTIL).
func (r Rule) Bounded() bool { return r.Count > 0 || r.Until != nil }

// CheckUntil refuses an UNTIL more than MaxUntil after the first start.
func (r Rule) CheckUntil(start time.Time) error {
	if r.Until != nil && r.Until.Sub(start) > MaxUntil {
		return fail("UNTIL must be at most five years after the first start")
	}
	if r.Until != nil && r.Until.Before(start) {
		return fail("UNTIL is before the first start")
	}
	return nil
}

// Expand returns the start instants of the series, in start's location, from the first start up to and including
// windowEnd, honouring COUNT and UNTIL, at most limit instants. Only instants that match the rule are returned; the
// first start itself is included when it matches.
func (r Rule) Expand(start, windowEnd time.Time, limit int) []time.Time {
	var out []time.Time
	emitted := 0
	for p := 0; p < maxPeriods; p++ {
		cands := r.period(start, p)
		if cands == nil {
			break
		}
		for _, t := range cands {
			if t.Before(start) {
				continue
			}
			if t.After(windowEnd) || (r.Until != nil && t.After(*r.Until)) || (r.Count > 0 && emitted >= r.Count) || len(out) >= limit {
				return out
			}
			out = append(out, t)
			emitted++
		}
	}
	return out
}

// period returns the sorted candidate instants of the p-th period after start; nil means the walk is beyond any
// representable date.
func (r Rule) period(start time.Time, p int) []time.Time {
	loc, step := start.Location(), p*r.Interval
	h, m, s := start.Clock()
	at := func(y int, mo time.Month, d int) time.Time { return wallClock(y, mo, d, [3]int{h, m, s}, loc) }
	var days []time.Time
	switch r.Freq {
	case Daily:
		days = []time.Time{at(start.Year(), start.Month(), start.Day()+step)}
	case Weekly:
		days = r.weekDays(start, step, at)
	case Monthly:
		first := at(start.Year(), start.Month()+time.Month(step), 1)
		days = r.monthDays(first.Year(), first.Month(), start, at)
	case Yearly:
		for _, mo := range r.months(start) {
			days = append(days, r.monthDays(start.Year()+step, mo, start, at)...)
		}
	}
	if len(days) > 0 && days[0].Year() > 9000 {
		return nil
	}
	return r.filter(days)
}

// wallClock is the instant of a local wall-clock time. A time inside a DST gap does not exist; RFC 5545 section 3.3.5
// interprets it with the UTC offset in force before the gap (02:30 on a spring-forward night becomes 03:30).
func wallClock(y int, mo time.Month, d int, hms [3]int, loc *time.Location) time.Time {
	t := time.Date(y, mo, d, hms[0], hms[1], hms[2], 0, loc)
	if t.Hour() == hms[0] && t.Minute() == hms[1] {
		return t
	}
	_, before := t.Add(-12 * time.Hour).Zone()
	return time.Date(y, mo, d, hms[0], hms[1], hms[2], 0, time.UTC).Add(-time.Duration(before) * time.Second).In(loc)
}

func (r Rule) weekDays(start time.Time, step int, at func(int, time.Month, int) time.Time) []time.Time {
	offset := (int(start.Weekday()) - int(r.WeekStart) + 7) % 7
	weekStart := start.Day() - offset + 7*step
	wanted := r.ByDay
	if len(wanted) == 0 {
		wanted = []WeekdayNum{{Day: start.Weekday()}}
	}
	var out []time.Time
	for _, w := range wanted {
		out = append(out, at(start.Year(), start.Month(), weekStart+(int(w.Day)-int(r.WeekStart)+7)%7))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func (r Rule) months(start time.Time) []time.Month {
	if len(r.ByMonth) == 0 {
		return []time.Month{start.Month()}
	}
	out := make([]time.Month, len(r.ByMonth))
	for i, m := range r.ByMonth {
		out[i] = time.Month(m)
	}
	return out
}

// WallClock returns the instant in loc of the wall-clock fields of local (DST gaps resolved as RFC 5545 does).
func WallClock(local time.Time, loc *time.Location) time.Time {
	return wallClock(local.Year(), local.Month(), local.Day(), [3]int{local.Hour(), local.Minute(), local.Second()}, loc)
}
