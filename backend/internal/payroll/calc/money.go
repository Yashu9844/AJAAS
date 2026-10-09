// Package calc is Module 5's pure payroll arithmetic: money, structure breakdown, proration and Indian statutory
// deductions. No I/O; every rule is golden-tested.
package calc

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Money is an exact 2-decimal amount in hundredths: rupees as paise, or a percentage as basis points (D5-02).
type Money int64

const maxWhole = int64(9_000_000_000_000) // well inside NUMERIC(14,2) and int64

var errMoney = errors.New("amount must be a number with at most 2 decimals")

// Rupees converts whole rupees to Money.
func Rupees(n int64) Money { return Money(n * 100) }

// ParseMoney parses "1", "41666.67", "-2.5"; more than 2 decimals is an error.
func ParseMoney(s string) (Money, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || (hasDot && (frac == "" || len(frac) > 2)) || !digits(whole) || !digits(frac) {
		return 0, errMoney
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > maxWhole {
		return 0, errMoney
	}
	f, _ := strconv.ParseInt((frac + "00")[:2], 10, 64)
	m := Money(w*100 + f)
	if neg {
		m = -m
	}
	return m, nil
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String renders two decimals.
func (m Money) String() string {
	sign, v := "", int64(m)
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// Percent returns m × p where p is a percentage in hundredths (40.00% = 4000), rounded half away from zero.
func (m Money) Percent(p Money) Money { return divRound(int64(m)*int64(p), 10000) }

// Prorate returns m × num / den rounded half away from zero; den 0 yields 0 (PY-002).
func (m Money) Prorate(num, den int64) Money {
	if den == 0 {
		return 0
	}
	return divRound(int64(m)*num, den)
}

// RoundRupee rounds to the nearest rupee, half away from zero (PF, PT, TDS).
func (m Money) RoundRupee() Money { return divRound(int64(m), 100) * 100 }

// CeilRupee rounds a non-negative amount up to the next rupee (ESI, PY-006).
func (m Money) CeilRupee() Money { return Money((int64(m) + 99) / 100 * 100) }

// Min returns the smaller amount.
func Min(a, b Money) Money {
	if a < b {
		return a
	}
	return b
}

func divRound(n, d int64) Money {
	q, r := n/d, n%d
	if 2*abs(r) >= abs(d) {
		if (n < 0) != (d < 0) {
			q--
		} else {
			q++
		}
	}
	return Money(q)
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// Value stores Money as an exact decimal string.
func (m Money) Value() (driver.Value, error) { return m.String(), nil }

// Scan reads NUMERIC (string/[]byte), float64 or int64.
func (m *Money) Scan(src interface{}) error {
	switch v := src.(type) {
	case nil:
		*m = 0
	case string:
		return m.parse(v)
	case []byte:
		return m.parse(string(v))
	case float64:
		*m = Money(math.Round(v * 100))
	case int64:
		*m = Money(v * 100)
	default:
		return fmt.Errorf("calc.Money: cannot scan %T", src)
	}
	return nil
}

func (m *Money) parse(s string) error {
	v, err := ParseMoney(s)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// MarshalJSON emits a JSON number with 2 decimals.
func (m Money) MarshalJSON() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalJSON accepts a JSON number or numeric string with at most 2 decimals.
func (m *Money) UnmarshalJSON(b []byte) error { return m.parse(strings.Trim(string(b), `"`)) }
