// Package validators holds Module 6 field rules that struct tags cannot express.
package validators

import (
	"errors"
	"strings"
	"time"

	"github.com/jaas/jaas/internal/payroll/calc"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// Field errors returned as VALIDATION_ERROR details.
var (
	ErrDate        = errors.New("date must be YYYY-MM-DD")
	ErrPastDate    = errors.New("date must not be in the past")
	ErrPastTime    = errors.New("scheduled_at must be in the future")
	ErrCTC         = errors.New("amount must be greater than 0 and at most 1000000000")
	ErrExpiryOrder = errors.New("expires_on must not be after joining_date")
	maxCTC         = calc.Rupees(1_000_000_000)
)

// ParseDate parses a YYYY-MM-DD date as UTC midnight.
func ParseDate(s string) (time.Time, error) {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, ErrDate
	}
	return d, nil
}

// ParseFutureDate parses a date that must be today or later (FR-OF001 joining date).
func ParseFutureDate(s string, today time.Time) (time.Time, error) {
	d, err := ParseDate(s)
	if err != nil {
		return d, err
	}
	if d.Before(today) {
		return d, ErrPastDate
	}
	return d, nil
}

// FutureInstant enforces RC-004 (scheduled_at after now).
func FutureInstant(at, now time.Time) error {
	if !at.After(now) {
		return ErrPastTime
	}
	return nil
}

// ValidateCTC checks an offered/expected CTC.
func ValidateCTC(m calc.Money) error {
	if m <= 0 || m > maxCTC {
		return ErrCTC
	}
	return nil
}

// NormalizeEmail lower-cases and trims an email for the per-job uniqueness check (FR-CD001).
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
