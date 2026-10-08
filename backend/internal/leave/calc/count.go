package calc

import "time"

const dayLayout = "2006-01-02"

// DateOf returns the UTC calendar date of t at midnight (A4-03).
func DateOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// IsWeekend reports Saturday/Sunday — the fixed v1 weekly off (D4-05).
func IsWeekend(day time.Time) bool {
	wd := day.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// DaysBetween counts whole days from a to b (dates at midnight UTC).
func DaysBetween(a, b time.Time) int { return int(b.Sub(a).Hours() / 24) }

// CountInput describes a leave range; Holidays lists non-optional holiday dates only.
type CountInput struct {
	Start, End time.Time
	Half       bool
	Sandwich   bool
	Holidays   []time.Time
}

// Count is the priced range: Total leave days and the full working dates to mark in attendance.
type Count struct {
	Total        Days
	WorkingDates []time.Time
}

// CountDays prices a range per LV-006: working days only, sandwich adds non-working days between
// the first and last working day, a half day is 0.50 and marks no attendance date (D4-09).
func CountDays(in CountInput) Count {
	off := make(map[string]bool, len(in.Holidays))
	for _, h := range in.Holidays {
		off[h.Format(dayLayout)] = true
	}
	var working []time.Time
	first, last := -1, -1
	for i, day := 0, in.Start; !day.After(in.End); i, day = i+1, day.AddDate(0, 0, 1) {
		if IsWeekend(day) || off[day.Format(dayLayout)] {
			continue
		}
		working = append(working, day)
		if first < 0 {
			first = i
		}
		last = i
	}
	switch {
	case len(working) == 0:
		return Count{}
	case in.Half:
		return Count{Total: 50}
	case in.Sandwich:
		return Count{Total: FromWhole(last - first + 1), WorkingDates: working}
	}
	return Count{Total: FromWhole(len(working)), WorkingDates: working}
}
