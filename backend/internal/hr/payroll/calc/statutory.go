package calc

// Indian statutory constants (A5-03); changing any of them is a code change with a decision entry.
var (
	pfRate          = Money(1200)     // 12.00%
	pfWageCeiling   = Rupees(15000)   // PY-005
	esiGrossCeiling = Rupees(21000)   // PY-006
	ptThreshold     = Rupees(25000)   // PY-007
	ptAmount        = Rupees(200)     //
	stdDeduction    = Rupees(75000)   // PY-008, new regime
	rebateLimit     = Rupees(1200000) // section 87A
)

// slab is an upper bound (exclusive of the next) and its rate in hundredths of a percent.
type slab struct {
	upTo Money
	rate Money
}

// newRegimeFY26 are the FY 2025-26 new-regime slabs (PY-008); the last slab is unbounded.
var newRegimeFY26 = []slab{
	{Rupees(400000), 0}, {Rupees(800000), 500}, {Rupees(1200000), 1000}, {Rupees(1600000), 1500},
	{Rupees(2000000), 2000}, {Rupees(2400000), 2500}, {-1, 3000},
}

// PF is the employee contribution: 12% of earned basic capped at the wage ceiling, to the rupee (PY-005).
func PF(earnedBasic Money) Money { return Min(earnedBasic, pfWageCeiling).Percent(pfRate).RoundRupee() }

// ESI is 0.75% of earned gross, rounded up to the rupee, when full-month gross is within the ceiling (PY-006).
// Computed exactly (paise × 75 / 10000, ceiling to rupee) so round-then-ceil never under-collects.
func ESI(fullGross, earnedGross Money) Money {
	if fullGross > esiGrossCeiling || earnedGross <= 0 {
		return 0
	}
	const den = int64(10000 * 100)
	rupees := (int64(earnedGross)*75 + den - 1) / den
	return Rupees(rupees)
}

// PT is the default professional-tax slab (PY-007, D5-06).
func PT(earnedGross Money) Money {
	if earnedGross >= ptThreshold {
		return ptAmount
	}
	return 0
}

// AnnualTax is new-regime income tax on annual taxable income with the 87A rebate and 4% cess, to the rupee (PY-008).
func AnnualTax(taxable Money) Money {
	if taxable <= rebateLimit {
		return 0
	}
	var tax, lower Money
	for _, s := range newRegimeFY26 {
		upper := s.upTo
		if upper < 0 || taxable < upper {
			upper = taxable
		}
		if upper > lower {
			tax += (upper - lower).Percent(s.rate)
		}
		if s.upTo < 0 || taxable <= s.upTo {
			break
		}
		lower = s.upTo
	}
	return (tax + tax.Percent(400)).RoundRupee()
}

// MonthlyTDS projects full-month taxable earnings over 12 months (PY-008).
func MonthlyTDS(fullMonthTaxable Money) Money {
	taxable := fullMonthTaxable*12 - stdDeduction
	if taxable <= 0 {
		return 0
	}
	return AnnualTax(taxable).Monthly().RoundRupee()
}
