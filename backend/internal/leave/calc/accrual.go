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
	startMonth := 1
	if joining != nil {
		switch {
		case joining.Year() > year:
			return 0
		case joining.Year() == year:
			startMonth = int(joining.Month())
		}
	}
	endMonth := 12
	if p.Monthly {
		switch {
		case asOf.Year() < year:
			return 0
		case asOf.Year() == year:
			endMonth = int(asOf.Month())
		}
	}
	months := endMonth - startMonth + 1
	if months <= 0 {
		return 0
	}
	return p.Allowance * Days(months) / 12
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
