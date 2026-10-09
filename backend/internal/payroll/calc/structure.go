package calc

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Component kinds and calculation types (FR-ST001).
const (
	Earning   = "earning"
	Deduction = "deduction"

	Fixed          = "fixed"
	PercentOfCTC   = "percent_of_ctc"
	PercentOfBasic = "percent_of_basic"
	Balance        = "balance"

	BasicCode     = "BASIC"
	maxComponents = 40
	maxPercent    = Money(10000) // 100.00%
)

// Errors returned by structure validation and breakdown.
var (
	ErrInvalidStructure = errors.New("invalid salary structure")
	ErrOverflow         = errors.New("structure earnings exceed the monthly CTC")
)

var codeRegex = regexp.MustCompile(`^[A-Z0-9_]{2,20}$`)

// ComponentSpec is one structure line; Value is rupees (fixed) or a percentage in hundredths (percent_*).
type ComponentSpec struct {
	Code, Name, Kind, Calc string
	Value                  Money
	Taxable                bool
}

// StructureSpec is a structure plus its statutory switches.
type StructureSpec struct {
	Components       []ComponentSpec
	PF, ESI, PT, TDS bool
}

// Line is a computed payslip line.
type Line struct {
	Code, Name, Kind string
	Amount           Money
	Taxable          bool
}

// Monthly returns annual / 12 rounded to paise.
func (m Money) Monthly() Money { return divRound(int64(m), 12) }

// DaysInMonth returns the calendar days of a month.
func DaysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// ValidateStructure enforces FR-ST002; every failure wraps ErrInvalidStructure.
func ValidateStructure(s StructureSpec) error {
	if len(s.Components) > maxComponents {
		return invalid("at most %d components", maxComponents)
	}
	seen := map[string]bool{}
	basics, balances := 0, 0
	for _, c := range s.Components {
		if err := validateComponent(c); err != nil {
			return err
		}
		if seen[c.Code] {
			return invalid("duplicate component code %s", c.Code)
		}
		seen[c.Code] = true
		if c.Code == BasicCode {
			basics++
		}
		if c.Calc == Balance {
			balances++
		}
	}
	if basics != 1 {
		return invalid("exactly one %s component is required", BasicCode)
	}
	if balances > 1 {
		return invalid("at most one balance component")
	}
	return nil
}

func validateComponent(c ComponentSpec) error {
	switch {
	case !codeRegex.MatchString(c.Code):
		return invalid("component code %q must match ^[A-Z0-9_]{2,20}$", c.Code)
	case c.Name == "" || len(c.Name) > 100:
		return invalid("component %s needs a name of 1-100 characters", c.Code)
	case c.Kind != Earning && c.Kind != Deduction:
		return invalid("component %s kind must be earning or deduction", c.Code)
	case c.Calc != Fixed && c.Calc != PercentOfCTC && c.Calc != PercentOfBasic && c.Calc != Balance:
		return invalid("component %s has unknown calc %q", c.Code, c.Calc)
	case c.Value < 0 || (c.Calc != Fixed && c.Value > maxPercent):
		return invalid("component %s value out of range", c.Code)
	case c.Kind == Deduction && (c.Calc == PercentOfCTC || c.Calc == Balance):
		return invalid("deduction %s must be fixed or percent_of_basic", c.Code)
	case c.Code == BasicCode && (c.Kind != Earning || (c.Calc != Fixed && c.Calc != PercentOfCTC)):
		return invalid("%s must be an earning with fixed or percent_of_ctc", BasicCode)
	}
	return nil
}

func invalid(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrInvalidStructure, fmt.Sprintf(format, args...))
}

// Breakdown computes full-month amounts in component order (PY-001); balance takes CTC minus other earnings.
func Breakdown(s StructureSpec, monthlyCTC Money) ([]Line, error) {
	basic := Money(0)
	for _, c := range s.Components {
		if c.Code == BasicCode {
			basic = componentAmount(c, monthlyCTC, 0)
		}
	}
	lines := make([]Line, len(s.Components))
	var earnings Money
	balanceAt := -1
	for i, c := range s.Components {
		lines[i] = Line{Code: c.Code, Name: c.Name, Kind: c.Kind, Taxable: c.Taxable}
		if c.Calc == Balance {
			balanceAt = i
			continue
		}
		lines[i].Amount = componentAmount(c, monthlyCTC, basic)
		if c.Kind == Earning {
			earnings += lines[i].Amount
		}
	}
	rest := monthlyCTC - earnings
	if rest < 0 {
		return nil, ErrOverflow
	}
	if balanceAt >= 0 {
		lines[balanceAt].Amount = rest
	}
	return lines, nil
}

func componentAmount(c ComponentSpec, monthlyCTC, basic Money) Money {
	switch c.Calc {
	case PercentOfCTC:
		return monthlyCTC.Percent(c.Value)
	case PercentOfBasic:
		return basic.Percent(c.Value)
	}
	return c.Value
}
