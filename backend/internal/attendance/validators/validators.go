// Package validators holds Module 3 field rules that struct tags cannot express (AT-013, spec §5).
package validators

import (
	"errors"
	"regexp"
	"time"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

var shiftCodeRegex = regexp.MustCompile(`^[a-z0-9-]{2,32}$`)

// Field errors returned to clients as VALIDATION_ERROR details.
var (
	ErrShiftCode  = errors.New("code must match ^[a-z0-9-]{2,32}$")
	ErrTimezone   = errors.New("timezone must be a valid IANA zone, e.g. Asia/Kolkata")
	ErrDate       = errors.New("date must be YYYY-MM-DD")
	ErrDateOrder  = errors.New("from must not be after to")
	ErrDateSpan   = errors.New("date range too large")
	ErrThresholds = errors.New("half_day_minutes must not exceed full_day_minutes")
)

// ValidateShiftCode checks the optional shift code format (FR-SH001).
func ValidateShiftCode(code string) error {
	if !shiftCodeRegex.MatchString(code) {
		return ErrShiftCode
	}
	return nil
}

// LoadTimezone resolves an IANA zone; empty means UTC. "Local" is rejected (server-dependent).
func LoadTimezone(name string) (*time.Location, error) {
	if name == "" {
		return time.UTC, nil
	}
	if name == "Local" {
		return nil, ErrTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrTimezone
	}
	return loc, nil
}

// ParseDate parses a YYYY-MM-DD calendar date as UTC midnight.
func ParseDate(s string) (time.Time, error) {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, ErrDate
	}
	return d, nil
}

// DateRange parses an inclusive from/to query; defaults to the 7 days ending today; caps at maxDays.
func DateRange(fromStr, toStr string, today time.Time, maxDays int) (time.Time, time.Time, error) {
	to, from := today, today.AddDate(0, 0, -6)
	var err error
	if toStr != "" {
		if to, err = ParseDate(toStr); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if fromStr != "" {
		if from, err = ParseDate(fromStr); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, ErrDateOrder
	}
	if int(to.Sub(from).Hours()/24)+1 > maxDays {
		return time.Time{}, time.Time{}, ErrDateSpan
	}
	return from, to, nil
}

// ValidateThresholds enforces half ≤ full when both are set (AT-013).
func ValidateThresholds(full, half *int) error {
	if full != nil && half != nil && *half > *full {
		return ErrThresholds
	}
	return nil
}
