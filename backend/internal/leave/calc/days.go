// Package calc is Module 4's pure leave arithmetic: day amounts, day counting, accrual, carry-forward.
package calc

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Days is an amount of leave in hundredths of a day (1.5 days = 150); no floats in balances (D4-03, NFR-D002).
type Days int64

// maxDays bounds parsed amounts well inside NUMERIC(7,2).
const maxDays = Days(9_999_999)

var errDays = errors.New("days must be a number with at most 2 decimals")

// FromWhole converts whole days to Days.
func FromWhole(n int) Days { return Days(n) * 100 }

// ParseDays parses "1", "1.5", "-2.25"; more than 2 decimals or out of range is an error.
func ParseDays(s string) (Days, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || (hasDot && (frac == "" || len(frac) > 2)) || !digits(whole) || !digits(frac) {
		return 0, errDays
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > int64(maxDays)/100 {
		return 0, errDays
	}
	f, _ := strconv.ParseInt((frac + "00")[:2], 10, 64)
	d := Days(w*100 + f)
	if neg {
		d = -d
	}
	return d, nil
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String renders two decimals, e.g. "1.50", "-0.05".
func (d Days) String() string {
	sign, v := "", int64(d)
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// Float returns the amount as float64 for display only — never for arithmetic.
func (d Days) Float() float64 { return float64(d) / 100 }

// Value stores Days as an exact decimal string for NUMERIC(7,2).
func (d Days) Value() (driver.Value, error) { return d.String(), nil }

// Scan reads NUMERIC (string/[]byte), float64 or int64 from the driver.
func (d *Days) Scan(src interface{}) error {
	switch v := src.(type) {
	case nil:
		*d = 0
	case string:
		return d.parseInto(v)
	case []byte:
		return d.parseInto(string(v))
	case float64:
		*d = Days(math.Round(v * 100))
	case int64:
		*d = Days(v * 100)
	default:
		return fmt.Errorf("calc.Days: cannot scan %T", src)
	}
	return nil
}

func (d *Days) parseInto(s string) error {
	v, err := ParseDays(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON emits a JSON number with 2 decimals.
func (d Days) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalJSON accepts a JSON number or numeric string with at most 2 decimals.
func (d *Days) UnmarshalJSON(b []byte) error {
	return d.parseInto(strings.Trim(string(b), `"`))
}
