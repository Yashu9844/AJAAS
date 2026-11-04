package rrule

import (
	"sort"
	"time"
)

func daysIn(y int, mo time.Month) int { return time.Date(y, mo+1, 0, 0, 0, 0, 0, time.UTC).Day() }

// monthDays returns the matching days of one month: BYMONTHDAY and BYDAY (with ordinals) intersect when both are set;
// with neither, the day of month of the first start (months without that day are skipped).
func (r Rule) monthDays(y int, mo time.Month, start time.Time, at func(int, time.Month, int) time.Time) []time.Time {
	n := daysIn(y, mo)
	set := map[int]bool{}
	switch {
	case len(r.ByMonthDay) == 0 && len(r.ByDay) == 0:
		set[start.Day()] = start.Day() <= n
	case len(r.ByDay) == 0:
		set = monthDaySet(r.ByMonthDay, n)
	case len(r.ByMonthDay) == 0:
		set = weekdaySet(r.ByDay, y, mo, n)
	default:
		md, wd := monthDaySet(r.ByMonthDay, n), weekdaySet(r.ByDay, y, mo, n)
		for d := range md {
			set[d] = wd[d]
		}
	}
	var days []int
	for d, ok := range set {
		if ok {
			days = append(days, d)
		}
	}
	sort.Ints(days)
	out := make([]time.Time, len(days))
	for i, d := range days {
		out[i] = at(y, mo, d)
	}
	return out
}

func monthDaySet(byMonthDay []int, n int) map[int]bool {
	set := map[int]bool{}
	for _, d := range byMonthDay {
		if d < 0 {
			d = n + 1 + d
		}
		if d >= 1 && d <= n {
			set[d] = true
		}
	}
	return set
}

func weekdaySet(byDay []WeekdayNum, y int, mo time.Month, n int) map[int]bool {
	set := map[int]bool{}
	firstWeekday := time.Date(y, mo, 1, 0, 0, 0, 0, time.UTC).Weekday()
	for _, w := range byDay {
		first := 1 + (int(w.Day)-int(firstWeekday)+7)%7
		var all []int
		for d := first; d <= n; d += 7 {
			all = append(all, d)
		}
		switch {
		case w.N == 0:
			for _, d := range all {
				set[d] = true
			}
		case w.N > 0 && w.N <= len(all):
			set[all[w.N-1]] = true
		case w.N < 0 && -w.N <= len(all):
			set[all[len(all)+w.N]] = true
		}
	}
	return set
}

// filter applies the parts the period generator does not: BYMONTH everywhere, BYMONTHDAY and BYDAY for DAILY.
func (r Rule) filter(days []time.Time) []time.Time {
	out := make([]time.Time, 0, len(days))
	for _, t := range days {
		if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(t.Month())) {
			continue
		}
		if r.Freq == Daily && !r.dailyMatch(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (r Rule) dailyMatch(t time.Time) bool {
	if len(r.ByMonthDay) > 0 && !monthDaySet(r.ByMonthDay, daysIn(t.Year(), t.Month()))[t.Day()] {
		return false
	}
	if len(r.ByDay) == 0 {
		return true
	}
	for _, w := range r.ByDay {
		if w.Day == t.Weekday() {
			return true
		}
	}
	return false
}

func containsInt(v []int, n int) bool {
	for _, x := range v {
		if x == n {
			return true
		}
	}
	return false
}
