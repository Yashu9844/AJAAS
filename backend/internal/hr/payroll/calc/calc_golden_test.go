package calc

import (
	"errors"
	"testing"
)

func standard() StructureSpec {
	return StructureSpec{PF: true, ESI: true, PT: true, TDS: true, Components: []ComponentSpec{
		{Code: "BASIC", Name: "Basic", Kind: Earning, Calc: PercentOfCTC, Value: 4000, Taxable: true},
		{Code: "HRA", Name: "HRA", Kind: Earning, Calc: PercentOfBasic, Value: 5000, Taxable: true},
		{Code: "SPECIAL", Name: "Special", Kind: Earning, Calc: Balance, Taxable: true},
	}}
}

func amounts(lines []Line) map[string]Money {
	out := map[string]Money{}
	for _, l := range lines {
		out[l.Code] = l.Amount
	}
	return out
}

// TestGolden_G2_Breakdown — PY-001.
func TestGolden_G2_Breakdown(t *testing.T) {
	lines, err := Breakdown(standard(), Rupees(1200000).Monthly())
	got := amounts(lines)
	if err != nil || got["BASIC"] != Rupees(40000) || got["HRA"] != Rupees(20000) || got["SPECIAL"] != Rupees(40000) {
		t.Fatalf("breakdown %v %v", got, err)
	}
	spec := standard()
	spec.Components = append(spec.Components, ComponentSpec{Code: "CANTEEN", Name: "Canteen", Kind: Deduction, Calc: Fixed, Value: Rupees(500)},
		ComponentSpec{Code: "LWF", Name: "Welfare", Kind: Deduction, Calc: PercentOfBasic, Value: 100},
		ComponentSpec{Code: "BONUS", Name: "Bonus", Kind: Earning, Calc: Fixed, Value: Rupees(5000), Taxable: false})
	lines, _ = Breakdown(spec, Rupees(1200000).Monthly())
	got = amounts(lines)
	if got["SPECIAL"] != Rupees(35000) || got["CANTEEN"] != Rupees(500) || got["LWF"] != Rupees(400) {
		t.Fatalf("balance must absorb other earnings only: %v", got)
	}
}

// TestGolden_G3_Overflow — PY-001.
func TestGolden_G3_Overflow(t *testing.T) {
	spec := StructureSpec{Components: []ComponentSpec{
		{Code: "BASIC", Name: "Basic", Kind: Earning, Calc: Fixed, Value: Rupees(120000)},
		{Code: "SPECIAL", Name: "Special", Kind: Earning, Calc: Balance},
	}}
	if _, err := Breakdown(spec, Rupees(600000).Monthly()); !errors.Is(err, ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
	spec.Components = spec.Components[:1]
	if _, err := Breakdown(spec, Rupees(600000).Monthly()); !errors.Is(err, ErrOverflow) {
		t.Fatalf("no balance component still overflows: %v", err)
	}
}

// TestGolden_G4_Proration — PY-002: 30-day month, 15 payable days.
func TestGolden_G4_Proration(t *testing.T) {
	r, err := Compute(Input{Spec: StructureSpec{Components: standard().Components}, AnnualCTC: Rupees(1200000), DaysInMonth: 30, PayableHundredths: 1500})
	got := amounts(r.Lines)
	if err != nil || got["BASIC"] != Rupees(20000) || got["HRA"] != Rupees(10000) || got["SPECIAL"] != Rupees(20000) || r.Gross != Rupees(50000) {
		t.Fatalf("proration %v gross %s %v", got, r.Gross, err)
	}
	r, _ = Compute(Input{Spec: StructureSpec{Components: standard().Components}, AnnualCTC: Rupees(1000000), DaysInMonth: 31, PayableHundredths: 2950})
	if got := amounts(r.Lines)["BASIC"]; got != Money(3333333).Prorate(2950, 3100) {
		t.Fatalf("half-day proration basic = %s", got)
	}
}

// TestGolden_G5_PF — PY-005.
func TestGolden_G5_PF(t *testing.T) {
	for basic, want := range map[Money]Money{Rupees(30000): Rupees(1800), Rupees(10000): Rupees(1200), Money(1000050): Rupees(1200), 0: 0} {
		if got := PF(basic); got != want {
			t.Errorf("PF(%s) = %s, want %s", basic, got, want)
		}
	}
}

// TestGolden_G6_ESI — PY-006: eligibility on full-month gross, 0.75% of earned gross rounded up.
func TestGolden_G6_ESI(t *testing.T) {
	cases := []struct{ full, earned, want Money }{
		{Rupees(20000), Rupees(20000), Rupees(150)},
		{Rupees(21001), Rupees(21001), 0},
		{Rupees(21000), Rupees(21000), Rupees(158)}, // 157.50 → 158
		{Rupees(20123), Rupees(20123), Rupees(151)}, // 150.9225 → 151
		{Rupees(20000), Rupees(10000), Rupees(75)},
		{Rupees(13334), Rupees(13334), Rupees(101)}, // 100.005 → 101 (exact ceiling, not round-then-ceil)
	}
	for _, c := range cases {
		if got := ESI(c.full, c.earned); got != c.want {
			t.Errorf("ESI(%s,%s) = %s, want %s", c.full, c.earned, got, c.want)
		}
	}
}

// TestGolden_G7_PT — PY-007.
func TestGolden_G7_PT(t *testing.T) {
	if PT(Rupees(25000)) != Rupees(200) || PT(Money(2499999)) != 0 {
		t.Fatal("PT slab")
	}
}

// TestGolden_G8_TDS — PY-008, new regime FY 2025-26.
func TestGolden_G8_TDS(t *testing.T) {
	cases := []struct{ taxable, annual Money }{
		{Rupees(1200000), 0},
		{Rupees(1500000), Rupees(109200)},
		{Rupees(3000000), Rupees(499200)},
		{Rupees(1200001), Rupees(62400)}, // just above the 87A limit: 60,000.15 → no marginal relief (D5-07)
		{0, 0},
	}
	for _, c := range cases {
		if got := AnnualTax(c.taxable); got != c.annual {
			t.Errorf("AnnualTax(%s) = %s, want %s", c.taxable, got, c.annual)
		}
	}
	// monthly TDS from full-month taxable earnings: 1,31,250 × 12 − 75,000 = 15,00,000 → 1,09,200 / 12 = 9,100
	if got := MonthlyTDS(Rupees(131250)); got != Rupees(9100) {
		t.Fatalf("MonthlyTDS = %s", got)
	}
	if got := MonthlyTDS(Rupees(5000)); got != 0 {
		t.Fatalf("below standard deduction = %s", got)
	}
}

// TestGolden_G9_NetCap — PY-009: deductions never exceed gross; statutory lines trimmed from TDS backwards.
func TestGolden_G9_NetCap(t *testing.T) {
	spec := StructureSpec{PF: true, Components: []ComponentSpec{
		{Code: "BASIC", Name: "Basic", Kind: Earning, Calc: PercentOfCTC, Value: 10000, Taxable: true},
		{Code: "LOAN", Name: "Loan", Kind: Deduction, Calc: Fixed, Value: Rupees(9500)},
	}}
	r, err := Compute(Input{Spec: spec, AnnualCTC: Rupees(120000), DaysInMonth: 30, PayableHundredths: 3000})
	if err != nil || !r.Capped || r.Net != 0 || r.Deductions != r.Gross {
		t.Fatalf("cap: %+v %v", r, err)
	}
	var sum Money
	for _, l := range r.Lines {
		if l.Kind == Deduction {
			sum += l.Amount
		}
	}
	if sum != r.Deductions {
		t.Fatalf("lines must sum to deductions: %s vs %s", sum, r.Deductions)
	}
}

// Full payslip with every statutory deduction (ties G2, G5–G8 together).
func TestCompute_FullPayslip(t *testing.T) {
	r, err := Compute(Input{Spec: standard(), AnnualCTC: Rupees(1575000), DaysInMonth: 30, PayableHundredths: 3000})
	got := amounts(r.Lines)
	// monthly 1,31,250: basic 52,500, HRA 26,250, special 52,500; PF 1,800; ESI 0; PT 200; TDS 9,100
	if err != nil || r.Gross != Rupees(131250) || got["PF"] != Rupees(1800) || got["PT"] != Rupees(200) || got["TDS"] != Rupees(9100) {
		t.Fatalf("payslip %v gross %s %v", got, r.Gross, err)
	}
	if _, has := got["ESI"]; has {
		t.Fatal("zero statutory lines are omitted")
	}
	if r.Net != r.Gross-r.Deductions || r.Deductions != Rupees(11100) {
		t.Fatalf("net %s deductions %s", r.Net, r.Deductions)
	}
	if _, err := Compute(Input{Spec: StructureSpec{}, AnnualCTC: Rupees(100), DaysInMonth: 30, PayableHundredths: 3000}); err == nil {
		t.Fatal("empty structure must fail validation")
	}
}

func TestCompute_OverflowAndInterleavedCap(t *testing.T) {
	over := StructureSpec{Components: []ComponentSpec{{Code: "BASIC", Name: "B", Kind: Earning, Calc: Fixed, Value: Rupees(50000)}}}
	if _, err := Compute(Input{Spec: over, AnnualCTC: Rupees(120000), DaysInMonth: 30, PayableHundredths: 3000}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("compute must surface overflow: %v", err)
	}
	// an earning listed after a deduction is skipped while trimming
	spec := StructureSpec{Components: []ComponentSpec{
		{Code: "BASIC", Name: "B", Kind: Earning, Calc: Fixed, Value: Rupees(5000)},
		{Code: "LOAN", Name: "Loan", Kind: Deduction, Calc: Fixed, Value: Rupees(9000)},
		{Code: "BONUS", Name: "Bonus", Kind: Earning, Calc: Fixed, Value: Rupees(1000)},
	}}
	r, err := Compute(Input{Spec: spec, AnnualCTC: Rupees(120000), DaysInMonth: 30, PayableHundredths: 3000})
	if err != nil || !r.Capped || r.Net != 0 || amounts(r.Lines)["LOAN"] != Rupees(6000) || amounts(r.Lines)["BONUS"] != Rupees(1000) {
		t.Fatalf("interleaved cap: %+v %v", r, err)
	}
}
