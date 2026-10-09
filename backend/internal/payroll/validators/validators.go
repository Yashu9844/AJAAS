// Package validators holds Module 5 field rules that struct tags cannot express.
package validators

import (
	"errors"
	"strings"
	"time"

	"github.com/jaas/jaas/internal/payroll/calc"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// MaxAnnualCTC bounds CTC (FR-AS001): ₹1,000,000,000.
var MaxAnnualCTC = calc.Rupees(1_000_000_000)

// Field errors returned as VALIDATION_ERROR details.
var (
	ErrDate   = errors.New("date must be YYYY-MM-DD")
	ErrCTC    = errors.New("annual_ctc must be greater than 0 and at most 1000000000")
	ErrPeriod = errors.New("payroll period must not be in the future")
)

// ParseDate parses a YYYY-MM-DD date as UTC midnight.
func ParseDate(s string) (time.Time, error) {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, ErrDate
	}
	return d, nil
}

// ValidateCTC enforces the CTC range.
func ValidateCTC(ctc calc.Money) error {
	if ctc <= 0 || ctc > MaxAnnualCTC {
		return ErrCTC
	}
	return nil
}

// ValidatePeriod rejects periods after the current month (FR-RN001).
func ValidatePeriod(year, month int, today time.Time) error {
	if year > today.Year() || (year == today.Year() && month > int(today.Month())) {
		return ErrPeriod
	}
	return nil
}

// CSVSafe neutralizes spreadsheet formula injection in exported cells (PS-T8) and strips line breaks.
func CSVSafe(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
	if s != "" && strings.ContainsRune("=+-@\t", rune(s[0])) {
		return "'" + s
	}
	return s
}
