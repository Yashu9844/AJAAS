// Package score is Module 7's pure, exact arithmetic (D7-02): hundredths fixed point, key-result and goal progress,
// weighted goal score, review overall score and 9-box bands. No floats are used for any stored or compared value.
package score

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Fixed is a decimal with exactly two fractional digits, stored as hundredths (12.5 = 1250).
type Fixed int64

// Hundred is 100.00 — the upper bound of progress and weight.
const Hundred = Fixed(10000)

// maxWhole bounds parsed values to NUMERIC(14,2) and keeps every product below int64 overflow.
const maxWhole = 999_999_999_999

var errFixed = errors.New("must be a number with at most 2 decimals")

// FromInt converts a whole number.
func FromInt(n int) Fixed { return Fixed(n) * 100 }

// Parse reads "1", "1.5", "-2.25"; more than 2 decimals, exponents or out-of-range values are errors.
func Parse(s string) (Fixed, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || (hasDot && (frac == "" || len(frac) > 2)) || !digits(whole) || !digits(frac) {
		return 0, errFixed
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > maxWhole {
		return 0, errFixed
	}
	fr, _ := strconv.ParseInt((frac + "00")[:2], 10, 64)
	v := Fixed(w*100 + fr)
	if neg {
		v = -v
	}
	return v, nil
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String renders two decimals ("1.50", "-0.05").
func (v Fixed) String() string {
	sign, n := "", int64(v)
	if n < 0 {
		sign, n = "-", -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}

// Float is for display only — never for arithmetic.
func (v Fixed) Float() float64 { return float64(v) / 100 }

// Value stores an exact decimal string for NUMERIC columns.
func (v Fixed) Value() (driver.Value, error) { return v.String(), nil }

// Scan reads NUMERIC (string/[]byte), float64 or int64.
func (v *Fixed) Scan(src interface{}) error {
	switch x := src.(type) {
	case nil:
		*v = 0
	case string:
		return v.parseInto(x)
	case []byte:
		return v.parseInto(string(x))
	case float64:
		*v = Fixed(math.Round(x * 100))
	case int64:
		*v = Fixed(x * 100)
	default:
		return fmt.Errorf("score.Fixed: cannot scan %T", src)
	}
	return nil
}

func (v *Fixed) parseInto(s string) error {
	p, err := Parse(s)
	if err != nil {
		return err
	}
	*v = p
	return nil
}

// MarshalJSON emits a JSON number with 2 decimals.
func (v Fixed) MarshalJSON() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalJSON accepts a JSON number or numeric string with at most 2 decimals.
func (v *Fixed) UnmarshalJSON(b []byte) error { return v.parseInto(strings.Trim(string(b), `"`)) }

// divRound divides with rounding half away from zero; a zero divisor yields 0.
func divRound(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	if d < 0 {
		n, d = -n, -d
	}
	if n >= 0 {
		return (n + d/2) / d
	}
	return -((-n + d/2) / d)
}

// KRProgress is PM-006: clamp((current − start) / (target − start) × 100, 0, 100) in hundredths.
// A degenerate key result (target = start) yields 0; validators reject it before it is stored.
func KRProgress(start, target, current Fixed) Fixed {
	p := Fixed(divRound(int64(current-start)*int64(Hundred), int64(target-start)))
	if p < 0 {
		return 0
	}
	if p > Hundred {
		return Hundred
	}
	return p
}

// Mean is the rounded arithmetic mean (goal progress from key results, PM-006); empty → 0.
func Mean(values []Fixed) Fixed {
	var sum int64
	for _, v := range values {
		sum += int64(v)
	}
	return Fixed(divRound(sum, int64(len(values))))
}

// WeightedScore is PM-009: Σ(weight × progress) / Σ weight, rounded; empty or zero weight → 0.
func WeightedScore(weights, progress []Fixed) Fixed {
	var num, den int64
	for i := range weights {
		num += int64(weights[i]) * int64(progress[i])
		den += int64(weights[i])
	}
	return Fixed(divRound(num, den))
}

// Overall is PM-012: the mean of integer ratings as a 2-decimal score; empty → 0.
func Overall(ratings []int) Fixed {
	var sum int64
	for _, r := range ratings {
		sum += int64(r) * 100
	}
	return Fixed(divRound(sum, int64(len(ratings))))
}

// Performance bands on the 9-box axis (PM-015).
const (
	BandLow    = "low"
	BandMedium = "medium"
	BandHigh   = "high"
)

// Band maps an overall score to its 9-box performance band: < 2.50 low, < 3.50 medium, else high.
func Band(overall Fixed) string {
	switch {
	case overall < 250:
		return BandLow
	case overall < 350:
		return BandMedium
	default:
		return BandHigh
	}
}
