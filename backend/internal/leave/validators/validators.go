// Package validators holds Module 4 field rules that struct tags cannot express (FR-LT001, LV-003..LV-005, LV-017).
package validators

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jaas/jaas/internal/leave/calc"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// Bounds for day amounts (spec §3, LV-017).
const (
	MaxAllowance    = calc.Days(36500)
	MaxAdjustment   = calc.Days(36500)
	BackdateWindow  = 30
	minYear         = 2000
	maxYear         = 2200
	typeCodePattern = `^[A-Z0-9_]{2,10}$`
)

var typeCodeRegex = regexp.MustCompile(typeCodePattern)

// Field errors returned to clients as VALIDATION_ERROR details.
var (
	ErrTypeCode   = errors.New("code must match " + typeCodePattern + " after upper-casing")
	ErrDate       = errors.New("date must be YYYY-MM-DD")
	ErrYear       = errors.New("year must be between 2000 and 2200")
	ErrAmount     = errors.New("must be between 0 and 365 days with at most 2 decimals")
	ErrAdjustment = errors.New("days must be non-zero and at most 365 in magnitude")
)

// NormalizeTypeCode upper-cases and checks a leave type code (FR-LT001).
func NormalizeTypeCode(code string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(code))
	if !typeCodeRegex.MatchString(c) {
		return "", ErrTypeCode
	}
	return c, nil
}

// ValidateAllowance checks an allowance/carry-forward amount in [0, 365].
func ValidateAllowance(d calc.Days) error {
	if d < 0 || d > MaxAllowance {
		return ErrAmount
	}
	return nil
}

// ValidateAdjustment checks an HR adjustment (LV-017).
func ValidateAdjustment(d calc.Days) error {
	if d == 0 || d > MaxAdjustment || d < -MaxAdjustment {
		return ErrAdjustment
	}
	return nil
}

// ParseDate parses a YYYY-MM-DD calendar date as UTC midnight.
func ParseDate(s string) (time.Time, error) {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, ErrDate
	}
	return d, nil
}

// ParseYear parses an optional ?year= query; empty means fallback.
func ParseYear(s string, fallback int) (int, error) {
	if s == "" {
		return fallback, nil
	}
	y, err := strconv.Atoi(s)
	if err != nil || y < minYear || y > maxYear {
		return 0, ErrYear
	}
	return y, nil
}

// GenderApplies reports whether a type's applicable_gender admits the employee (LV-002, A4-01).
func GenderApplies(applicable, employeeGender string) bool {
	return applicable == "all" || strings.EqualFold(strings.TrimSpace(employeeGender), applicable)
}

// NoticeOK applies LV-005: future notice when minNotice > 0, else a 30-day backdating window.
func NoticeOK(start, today time.Time, minNotice int) bool {
	if minNotice > 0 {
		return !start.Before(today.AddDate(0, 0, minNotice))
	}
	return !start.Before(today.AddDate(0, 0, -BackdateWindow))
}
