package calc

// Segment is one slice of a pay window priced at one structure and CTC (PP-03).
type Segment struct {
	Spec              StructureSpec
	AnnualCTC         Money
	PayableHundredths int64
}

// SegmentsInput is one payslip: its segments (in date order), the month, the state PT slabs (nil keeps the default
// slab of PY-007), benefit deduction lines (PP-05) and the signed arrears amount (PP-04).
type SegmentsInput struct {
	Segments    []Segment
	DaysInMonth int
	Month       int
	PT          []PTSlab
	Benefits    []Line
	Arrears     Money
}

// ArrearsCode and ArrearsRecoveryCode name the arrears lines (PP-04).
const (
	ArrearsCode         = "ARREARS"
	ArrearsRecoveryCode = "ARREARS_RECOVERY"
)

// ComputeSegments runs the PY-001..PY-009 pipeline over one or more segments and adds employer lines (PP-01..PP-05).
// Earned amounts are summed per component code; PF and PT use the merged earned figures, ESI eligibility and TDS the
// last segment full-month figures. One segment without extras equals Compute plus the employer lines.
func ComputeSegments(in SegmentsInput) (Result, error) {
	if len(in.Segments) == 0 {
		return Result{}, invalid("a payslip needs at least one segment")
	}
	var r Result
	index := map[string]int{}
	var fullGross, fullTaxable Money
	last := in.Segments[len(in.Segments)-1]
	for si, seg := range in.Segments {
		if err := ValidateStructure(seg.Spec); err != nil {
			return Result{}, err
		}
		full, err := Breakdown(seg.Spec, seg.AnnualCTC.Monthly())
		if err != nil {
			return Result{}, err
		}
		for _, l := range full {
			if si == len(in.Segments)-1 && l.Kind == Earning {
				fullGross += l.Amount
				if l.Taxable {
					fullTaxable += l.Amount
				}
			}
			l.Amount = l.Amount.Prorate(seg.PayableHundredths, int64(in.DaysInMonth)*100)
			if at, ok := index[l.Code]; ok {
				r.Lines[at].Amount += l.Amount
				continue
			}
			index[l.Code] = len(r.Lines)
			r.Lines = append(r.Lines, l)
		}
	}
	var earnedBasic, earnedGross Money
	for _, l := range r.Lines {
		if l.Kind == Earning {
			earnedGross += l.Amount
		}
		if l.Code == BasicCode {
			earnedBasic = l.Amount
		}
	}
	r.Gross = earnedGross
	if in.Arrears > 0 {
		r.Lines = append(r.Lines, Line{Code: ArrearsCode, Name: "Arrears", Kind: Earning, Amount: in.Arrears, Taxable: true})
		r.Gross += in.Arrears
	}
	r.Lines = append(r.Lines, in.Benefits...)
	if in.Arrears < 0 {
		r.Lines = append(r.Lines, Line{Code: ArrearsRecoveryCode, Name: "Arrears recovery", Kind: Deduction, Amount: -in.Arrears})
	}
	pt := PT(earnedGross)
	if in.PT != nil {
		pt = PTFromSlabs(in.PT, in.Month, earnedGross)
	}
	r.Lines = append(r.Lines, statutory(last.Spec, statutoryBase{earnedBasic, fullGross, earnedGross, fullTaxable}, pt)...)
	for _, l := range r.Lines {
		if l.Kind == Deduction {
			r.Deductions += l.Amount
		}
	}
	r.capDeductions()
	r.Net = r.Gross - r.Deductions
	for _, l := range employerLines(last.Spec, earnedBasic, fullGross, earnedGross) {
		r.Lines = append(r.Lines, l)
		r.EmployerCost += l.Amount
	}
	return r, nil
}

// employerLines returns the enabled, non-zero employer contribution lines (PP-01).
func employerLines(s StructureSpec, earnedBasic, fullGross, earnedGross Money) []Line {
	var out []Line
	if s.PF {
		for _, l := range EmployerPF(earnedBasic) {
			if l.Amount > 0 {
				out = append(out, l)
			}
		}
	}
	if s.ESI {
		if amt := EmployerESI(fullGross, earnedGross); amt > 0 {
			out = append(out, Line{Code: "ESI_ER", Name: "Employer ESI", Kind: Employer, Amount: amt})
		}
	}
	return out
}
