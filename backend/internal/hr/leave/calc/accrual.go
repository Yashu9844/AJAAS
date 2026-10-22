package calc

import "time"

// Policy is the accrual part of a leave type.
type Policy struct {
	Allowance Days
	Monthly   bool
}

// Accrued is the entitlement earned in leave year `year` as of asOf (FR-BL002):
// allowance × creditedMonths / 12 in hundredths. Annual credits every month from the joining month
// (or January) through December up front; monthly credits only through asOf's month. The cumulative
// formula makes month 12 land exactly on the allowance, so December absorbs the rounding remainder.
func Accrued(p Policy, joining *time.Time, year int, asOf time.Time) Days {
	return AccruedFrom(p, joining, year, 1, asOf)
}

// AccruedFrom is Accrued for a leave year that starts in startMonth (1–12) of `year` and runs twelve months (LP-02).
// Months are credited from the later of the year start and the joining month through the year end (annual) or through
// asOf's month (monthly); the cumulative formula still lands month 12 exactly on the allowance.
func AccruedFrom(p Policy, joining *time.Time, year, startMonth int, asOf time.Time) Days {
	idx := func(y int, m time.Month) int { return y*12 + int(m) - 1 }
	yearStart := idx(year, time.Month(startMonth))
	yearEnd := yearStart + 11
	first, last := yearStart, yearEnd
	if joining != nil {
		j := idx(joining.Year(), joining.Month())
		if j > yearEnd {
			return 0
		}
		first = max(first, j)
	}
	if p.Monthly {
		a := idx(asOf.Year(), asOf.Month())
		if a < yearStart {
			return 0
		}
		last = min(last, a)
	}
	months := last - first + 1
	if months <= 0 {
		return 0
	}
	return p.Allowance * Days(months) / 12
}

// YearOf labels the leave year of a date by the calendar year in which it starts (LP-02).
func YearOf(d time.Time, startMonth int) int {
	if startMonth < 1 || startMonth > 12 {
		startMonth = 1
	}
	if int(d.Month()) < startMonth {
		return d.Year() - 1
	}
	return d.Year()
}

// CarryForward is the opening balance moved into a new year: previous available clamped to [0, limit] (FR-BL003).
func CarryForward(prevAvailable, limit Days) Days {
	switch {
	case prevAvailable <= 0:
		return 0
	case prevAvailable > limit:
		return limit
	}
	return prevAvailable
}
