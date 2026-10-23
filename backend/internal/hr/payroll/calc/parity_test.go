package calc

import "testing"

func paise(rupees, p int64) Money { return Rupees(rupees) + Money(p) }

// TestEmployerPF freezes PG1: the employer PF pieces always add up to the rounded 12%.
func TestEmployerPF(t *testing.T) {
	cases := []struct {
		name                  string
		basic                 Money
		epf, eps, edli, admin Money
	}{
		{"above the ceiling is capped at 15,000", Rupees(20000), Rupees(550), Rupees(1250), Rupees(75), Rupees(75)},
		{"at the ceiling", Rupees(15000), Rupees(550), Rupees(1250), Rupees(75), Rupees(75)},
		{"10,000 splits exactly", Rupees(10000), Rupees(367), Rupees(833), Rupees(50), Rupees(50)},
		{"odd basic rounds to the rupee", paise(12345, 67), paise(453, 0), paise(1028, 0), paise(62, 0), paise(62, 0)},
	}
	for _, c := range cases {
		lines := EmployerPF(c.basic)
		if len(lines) != 4 {
			t.Fatalf("%s: %d lines", c.name, len(lines))
		}
		got := []Money{lines[0].Amount, lines[1].Amount, lines[2].Amount, lines[3].Amount}
		want := []Money{c.epf, c.eps, c.edli, c.admin}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s line %s: got %s want %s", c.name, lines[i].Code, got[i], want[i])
			}
		}
		if lines[0].Kind != Employer {
			t.Errorf("%s: kind %s", c.name, lines[0].Kind)
		}
		if total := Min(c.basic, Rupees(15000)).Percent(1200).RoundRupee(); lines[0].Amount+lines[1].Amount != total {
			t.Errorf("%s: EPF+EPS %s != 12%% %s", c.name, lines[0].Amount+lines[1].Amount, total)
		}
	}
	if EmployerPF(0) != nil || EmployerPF(-5) != nil {
		t.Error("no basic, no contribution")
	}
}

// TestEmployerESI freezes PG1 at the eligibility edges.
func TestEmployerESI(t *testing.T) {
	cases := []struct {
		name               string
		full, earned, want Money
	}{
		{"exact", Rupees(20000), Rupees(20000), Rupees(650)},
		{"rounds up", Rupees(20000), paise(19999, 99), Rupees(650)},
		{"at the ceiling rounds 682.50 up", Rupees(21000), Rupees(21000), Rupees(683)},
		{"above the ceiling", paise(21000, 1), Rupees(10000), 0},
		{"nothing earned", Rupees(10000), 0, 0},
		{"prorated", Rupees(15000), Rupees(7500), Rupees(244)},
	}
	for _, c := range cases {
		got := EmployerESI(c.full, c.earned)
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

// TestPTSlabs freezes PG2: slab selection, the February override and validation.
func TestPTSlabs(t *testing.T) {
	cases := []struct {
		state string
		month int
		gross Money
		want  Money
	}{
		{"KA", 6, paise(24999, 99), 0}, {"KA", 6, Rupees(25000), Rupees(200)}, {"KA", 2, Rupees(25000), Rupees(300)}, {"ka", 3, Rupees(80000), Rupees(200)},
		{"MH", 6, Rupees(7500), 0}, {"MH", 6, Rupees(7501), Rupees(175)}, {"MH", 6, Rupees(10000), Rupees(175)}, {"MH", 6, Rupees(10001), Rupees(200)},
		{"MH", 2, Rupees(10001), Rupees(300)}, {"MH", 2, Rupees(9000), Rupees(175)},
		{"WB", 1, Rupees(10000), 0}, {"WB", 1, Rupees(12000), Rupees(110)}, {"WB", 1, Rupees(20000), Rupees(130)}, {"WB", 1, Rupees(30000), Rupees(150)}, {"WB", 1, Rupees(50000), Rupees(200)},
		{"GJ", 1, Rupees(12000), 0}, {"GJ", 1, Rupees(12001), Rupees(200)},
		{"AP", 1, Rupees(15000), 0}, {"AP", 1, Rupees(18000), Rupees(150)}, {"TS", 1, Rupees(25000), Rupees(200)},
	}
	for _, c := range cases {
		slabs := DefaultPTSlabs(c.state)
		if err := ValidatePTSlabs(slabs); err != nil {
			t.Fatalf("%s defaults invalid: %v", c.state, err)
		}
		if got := PTFromSlabs(slabs, c.month, c.gross); got != c.want {
			t.Errorf("%s month %d gross %s: got %s want %s", c.state, c.month, c.gross, got, c.want)
		}
	}
	if DefaultPTSlabs("ZZ") != nil {
		t.Error("unknown state has no defaults")
	}
	bad := map[string][]PTSlab{
		"empty":              {},
		"no generic series":  {{MinGross: 0, Amount: 1, Month: 2}},
		"generic not from 0": {{MinGross: Rupees(5), Amount: 1}},
		"overlap":            {{MinGross: 0, MaxGross: Rupees(100), Amount: 1}, {MinGross: Rupees(100), Amount: 2}},
		"open ended first":   {{MinGross: 0, Amount: 1}, {MinGross: Rupees(100), Amount: 2}},
		"negative amount":    {{MinGross: 0, Amount: -1}},
		"max below min":      {{MinGross: 0, Amount: 1}, {MinGross: Rupees(10), MaxGross: Rupees(5), Amount: 1, Month: 3}},
		"bad month":          {{MinGross: 0, Amount: 1}, {MinGross: 0, Amount: 1, Month: 13}},
	}
	for name, slabs := range bad {
		if ValidatePTSlabs(slabs) == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	tooMany := make([]PTSlab, 21)
	if ValidatePTSlabs(tooMany) == nil {
		t.Error("21 slabs must be refused")
	}
}

func spec(pf, esi, pt bool) StructureSpec {
	return StructureSpec{PF: pf, ESI: esi, PT: pt, Components: []ComponentSpec{
		{Code: "BASIC", Name: "Basic", Kind: Earning, Calc: PercentOfCTC, Value: 5000, Taxable: true},
		{Code: "SPECIAL", Name: "Special", Kind: Earning, Calc: Balance, Taxable: true},
	}}
}

// TestComputeSegmentsMatchesCompute freezes PG3: one segment equals the original pipeline.
func TestComputeSegmentsMatchesCompute(t *testing.T) {
	for _, flags := range [][3]bool{{false, false, false}, {true, true, true}, {true, false, true}} {
		for _, payable := range []int64{3000, 2250, 1500, 0} {
			s := spec(flags[0], flags[1], flags[2])
			old, err := Compute(Input{Spec: s, AnnualCTC: Rupees(240000), DaysInMonth: 30, PayableHundredths: payable})
			if err != nil {
				t.Fatal(err)
			}
			got, err := ComputeSegments(SegmentsInput{Segments: []Segment{{Spec: s, AnnualCTC: Rupees(240000), PayableHundredths: payable}}, DaysInMonth: 30, Month: 6})
			if err != nil {
				t.Fatal(err)
			}
			if got.Gross != old.Gross || got.Deductions != old.Deductions || got.Net != old.Net || got.Capped != old.Capped {
				t.Fatalf("totals differ: %+v vs %+v", got, old)
			}
			var gotLines []Line
			for _, l := range got.Lines {
				if l.Kind != Employer {
					gotLines = append(gotLines, l)
				}
			}
			if len(gotLines) != len(old.Lines) {
				t.Fatalf("line count %d vs %d", len(gotLines), len(old.Lines))
			}
			for i := range gotLines {
				if gotLines[i] != old.Lines[i] {
					t.Fatalf("line %d: %+v vs %+v", i, gotLines[i], old.Lines[i])
				}
			}
		}
	}
}

// TestComputeSegmentsMidMonth freezes PG3: each segment is priced at its own CTC and the pieces add up.
func TestComputeSegmentsMidMonth(t *testing.T) {
	s := spec(true, false, false)
	r, err := ComputeSegments(SegmentsInput{DaysInMonth: 30, Month: 6, Segments: []Segment{
		{Spec: s, AnnualCTC: Rupees(600000), PayableHundredths: 1000},  // 10 days at 50,000 a month
		{Spec: s, AnnualCTC: Rupees(1200000), PayableHundredths: 2000}, // 20 days at 100,000 a month
	}})
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]Money{}
	for _, l := range r.Lines {
		byCode[l.Code] = l.Amount
	}
	if byCode["BASIC"] != paise(41666, 66) || byCode["SPECIAL"] != paise(41666, 66) || r.Gross != paise(83333, 32) {
		t.Fatalf("earnings: %v gross %s", byCode, r.Gross)
	}
	// PF uses the merged earned basic (41,666.66 → capped at 15,000): 12% = 1,800.
	if byCode["PF"] != Rupees(1800) || r.Net != r.Gross-Rupees(1800) {
		t.Fatalf("PF %s net %s", byCode["PF"], r.Net)
	}
	if r.EmployerCost != Rupees(1950) || byCode["EPS_ER"] != Rupees(1250) {
		t.Fatalf("employer cost %s", r.EmployerCost)
	}
	if _, err := ComputeSegments(SegmentsInput{DaysInMonth: 30}); err == nil {
		t.Fatal("no segments")
	}
	bad := StructureSpec{Components: []ComponentSpec{{Code: "X"}}}
	if _, err := ComputeSegments(SegmentsInput{DaysInMonth: 30, Segments: []Segment{{Spec: bad, AnnualCTC: Rupees(1)}}}); err == nil {
		t.Fatal("invalid structure")
	}
	over := StructureSpec{Components: []ComponentSpec{{Code: "BASIC", Name: "Basic", Kind: Earning, Calc: Fixed, Value: Rupees(999999), Taxable: true}}}
	if _, err := ComputeSegments(SegmentsInput{DaysInMonth: 30, Segments: []Segment{{Spec: over, AnnualCTC: Rupees(12000), PayableHundredths: 3000}}}); err == nil {
		t.Fatal("overflow")
	}
}

// TestComputeSegmentsExtras freezes arrears, benefits, state PT and the net cap in one payslip.
func TestComputeSegmentsExtras(t *testing.T) {
	s := spec(false, false, true)
	base := SegmentsInput{DaysInMonth: 30, Month: 2, Segments: []Segment{{Spec: s, AnnualCTC: Rupees(480000), PayableHundredths: 3000}}}
	plain, _ := ComputeSegments(base)
	if plain.Gross != Rupees(40000) || plain.Deductions != Rupees(200) {
		t.Fatalf("default PT: %+v", plain)
	}
	ka := base
	ka.PT = DefaultPTSlabs("KA")
	if r, _ := ComputeSegments(ka); r.Deductions != Rupees(300) {
		t.Fatalf("PG2 February override in the payslip: %s", r.Deductions)
	}
	withArrears := base
	withArrears.Arrears = Rupees(1000)
	r, _ := ComputeSegments(withArrears)
	if r.Gross != Rupees(41000) || r.Deductions != Rupees(200) || r.Lines[2].Code != ArrearsCode || !r.Lines[2].Taxable {
		t.Fatalf("PG4 positive arrears: %+v", r)
	}
	recovery := base
	recovery.Arrears = -Rupees(500)
	if r, _ := ComputeSegments(recovery); r.Gross != Rupees(40000) || r.Deductions != Rupees(700) || r.Net != Rupees(39300) {
		t.Fatalf("PG4 negative arrears: %+v", r)
	}
	ben := base
	ben.Benefits = []Line{{Code: "BEN_MED", Name: "Medical", Kind: Deduction, Amount: Rupees(500)}}
	if r, _ := ComputeSegments(ben); r.Deductions != Rupees(700) || r.Lines[2].Code != "BEN_MED" {
		t.Fatalf("PP-05 benefit before statutory: %+v", r)
	}
	huge := base
	huge.Benefits = []Line{{Code: "BEN_X", Name: "X", Kind: Deduction, Amount: Rupees(50000)}}
	r, _ = ComputeSegments(huge)
	if !r.Capped || r.Net != 0 || r.Deductions != r.Gross {
		t.Fatalf("PY-009 cap still applies: %+v", r)
	}
	for _, l := range r.Lines {
		if l.Code == "PT" && l.Amount != 0 {
			t.Fatalf("statutory lines are trimmed first: %+v", l)
		}
	}
}
