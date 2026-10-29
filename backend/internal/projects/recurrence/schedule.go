package recurrence

import (
	"time"

	"github.com/jaas/jaas/internal/projects/kernel"
)

func kernelDate(t time.Time) kernel.Date { return kernel.Date{Time: day(t)} }

// day truncates t to midnight UTC.
func day(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// onMonthDay is the date in the month of t on dom, clamped to the last day of a short month.
func onMonthDay(t time.Time, dom int) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, min(dom, daysIn(y, m)), 0, 0, 0, 0, time.UTC)
}

// step is the occurrence after cur: daily and weekly add whole days, monthly moves interval months and lands on the
// configured day of month (clamped), so a 31st never drifts to the 28th for good.
func step(cur time.Time, r Recurrence) time.Time {
	switch r.Frequency {
	case Weekly:
		return day(cur).AddDate(0, 0, 7*r.Interval)
	case Monthly:
		first := time.Date(cur.Year(), cur.Month()+time.Month(r.Interval), 1, 0, 0, 0, 0, time.UTC)
		dom := cur.Day()
		if r.DayOfMonth != nil {
			dom = *r.DayOfMonth
		}
		return onMonthDay(first, dom)
	}
	return day(cur).AddDate(0, 0, r.Interval)
}

// firstAfter is the first occurrence strictly after now (00:00 UTC of a later day).
func firstAfter(now time.Time, r Recurrence) time.Time {
	d := day(now)
	for i := 1; i <= 62; i++ {
		c := d.AddDate(0, 0, i)
		switch {
		case r.Frequency == Weekly && r.Weekday != nil && int(c.Weekday()) == *r.Weekday:
			return c
		case r.Frequency == Monthly && r.DayOfMonth != nil && c.Day() == min(*r.DayOfMonth, daysIn(c.Year(), c.Month())):
			return c
		case r.Frequency == Daily:
			return c
		}
	}
	return d.AddDate(0, 0, 1)
}

// advancePast moves from the occurrence that just ran to the first one after now: missed occurrences are dropped, so a
// scheduler that was down creates one catch-up task, never a burst.
func advancePast(cur, now time.Time, r Recurrence) time.Time {
	next := step(cur, r)
	for i := 0; i < 1000 && !next.After(now); i++ {
		next = step(next, r)
	}
	if !next.After(now) {
		return firstAfter(now, r)
	}
	return next
}

// datesFor dates a copy relative to the run: the start moves to the run day and the due date keeps its distance from the
// template start (or from the day the template was created when it has no start), with the same time of day.
func datesFor(t Template, run time.Time) Dates {
	var d Dates
	runDay := day(run)
	ref := day(t.CreatedAt)
	if t.StartOn != nil {
		ref = day(t.StartOn.Time)
		start := kernelDate(runDay)
		d.StartOn = &start
	}
	if t.DueAt != nil {
		offset := int(day(*t.DueAt).Sub(ref).Hours() / 24)
		if offset < 0 {
			offset = 0
		}
		due := runDay.AddDate(0, 0, offset).Add(t.DueAt.UTC().Sub(day(*t.DueAt)))
		d.DueAt = &due
	}
	return d
}
