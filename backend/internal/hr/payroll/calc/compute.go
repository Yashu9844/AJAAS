package calc

// Input is everything needed to compute one payslip; PayableHundredths is payable days × 100 (half days allowed).
type Input struct {
	Spec              StructureSpec
	AnnualCTC         Money
	DaysInMonth       int
	PayableHundredths int64
}

// Result is a computed payslip; Capped reports that deductions were trimmed to gross (PY-009).
type Result struct {
	Lines                  []Line
	Gross, Deductions, Net Money
	Capped                 bool
}

// Compute runs the PY-001..PY-009 pipeline: breakdown → proration → statutory → net with cap.
func Compute(in Input) (Result, error) {
	if err := ValidateStructure(in.Spec); err != nil {
		return Result{}, err
	}
	full, err := Breakdown(in.Spec, in.AnnualCTC.Monthly())
	if err != nil {
		return Result{}, err
	}
	var r Result
	var fullGross, fullTaxable, basic Money
	for _, l := range full {
		earned := l
		earned.Amount = l.Amount.Prorate(in.PayableHundredths, int64(in.DaysInMonth)*100)
		r.Lines = append(r.Lines, earned)
		if l.Kind == Earning {
			fullGross += l.Amount
			r.Gross += earned.Amount
			if l.Taxable {
				fullTaxable += l.Amount
			}
		}
		if l.Code == BasicCode {
			basic = earned.Amount
		}
	}
	r.Lines = append(r.Lines, statutory(in.Spec, statutoryBase{basic, fullGross, r.Gross, fullTaxable})...)
	for _, l := range r.Lines {
		if l.Kind == Deduction {
			r.Deductions += l.Amount
		}
	}
	r.capDeductions()
	r.Net = r.Gross - r.Deductions
	return r, nil
}

type statutoryBase struct{ earnedBasic, fullGross, earnedGross, fullTaxable Money }

// statutory returns the enabled, non-zero statutory deduction lines in fixed order PF, ESI, PT, TDS.
func statutory(s StructureSpec, b statutoryBase) []Line {
	candidates := []struct {
		on   bool
		code string
		name string
		amt  Money
	}{
		{s.PF, "PF", "Provident Fund", PF(b.earnedBasic)},
		{s.ESI, "ESI", "Employee State Insurance", ESI(b.fullGross, b.earnedGross)},
		{s.PT, "PT", "Professional Tax", PT(b.earnedGross)},
		{s.TDS, "TDS", "Income Tax (TDS)", MonthlyTDS(b.fullTaxable)},
	}
	var out []Line
	for _, c := range candidates {
		if c.on && c.amt > 0 {
			out = append(out, Line{Code: c.code, Name: c.name, Kind: Deduction, Amount: c.amt})
		}
	}
	return out
}

// capDeductions trims deduction lines from the last one backwards so deductions never exceed gross (PY-009).
func (r *Result) capDeductions() {
	excess := r.Deductions - r.Gross
	if excess <= 0 {
		return
	}
	r.Capped = true
	for i := len(r.Lines) - 1; i >= 0 && excess > 0; i-- {
		if r.Lines[i].Kind != Deduction {
			continue
		}
		cut := Min(r.Lines[i].Amount, excess)
		r.Lines[i].Amount -= cut
		excess -= cut
	}
	r.Deductions = r.Gross
}
