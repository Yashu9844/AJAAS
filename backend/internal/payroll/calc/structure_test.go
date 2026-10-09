package calc

import (
	"errors"
	"testing"
)

// FR-ST002 structure validation.
func TestValidateStructure(t *testing.T) {
	basic := ComponentSpec{Code: "BASIC", Name: "Basic", Kind: Earning, Calc: PercentOfCTC, Value: 4000}
	bal := ComponentSpec{Code: "SPECIAL", Name: "S", Kind: Earning, Calc: Balance}
	cases := []struct {
		name string
		comp []ComponentSpec
		ok   bool
	}{
		{"valid", []ComponentSpec{basic, bal}, true},
		{"no basic", []ComponentSpec{bal}, false},
		{"two basics", []ComponentSpec{basic, basic}, false},
		{"basic percent_of_basic", []ComponentSpec{{Code: "BASIC", Name: "B", Kind: Earning, Calc: PercentOfBasic, Value: 10}}, false},
		{"basic as deduction", []ComponentSpec{{Code: "BASIC", Name: "B", Kind: Deduction, Calc: Fixed, Value: 10}}, false},
		{"two balances", []ComponentSpec{basic, bal, {Code: "OTHER", Name: "O", Kind: Earning, Calc: Balance}}, false},
		{"duplicate code", []ComponentSpec{basic, {Code: "HRA", Name: "H", Kind: Earning, Calc: Fixed, Value: 1}, {Code: "HRA", Name: "H", Kind: Earning, Calc: Fixed, Value: 1}}, false},
		{"bad code", []ComponentSpec{basic, {Code: "h-ra", Name: "H", Kind: Earning, Calc: Fixed, Value: 1}}, false},
		{"deduction percent_of_ctc", []ComponentSpec{basic, {Code: "DED", Name: "D", Kind: Deduction, Calc: PercentOfCTC, Value: 1}}, false},
		{"deduction balance", []ComponentSpec{basic, {Code: "DED", Name: "D", Kind: Deduction, Calc: Balance}}, false},
		{"percent over 100", []ComponentSpec{basic, {Code: "HRA", Name: "H", Kind: Earning, Calc: PercentOfBasic, Value: 10001}}, false},
		{"negative value", []ComponentSpec{basic, {Code: "HRA", Name: "H", Kind: Earning, Calc: Fixed, Value: -1}}, false},
		{"unknown kind", []ComponentSpec{basic, {Code: "X1", Name: "X", Kind: "bonus", Calc: Fixed, Value: 1}}, false},
		{"unknown calc", []ComponentSpec{basic, {Code: "X1", Name: "X", Kind: Earning, Calc: "formula", Value: 1}}, false},
		{"empty name", []ComponentSpec{basic, {Code: "X1", Kind: Earning, Calc: Fixed, Value: 1}}, false},
		{"too many", make([]ComponentSpec, 41), false},
	}
	for _, c := range cases {
		err := ValidateStructure(StructureSpec{Components: c.comp})
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidStructure) {
			t.Errorf("%s: must wrap ErrInvalidStructure, got %v", c.name, err)
		}
	}
}

func TestMonthlyAndDaysInMonth(t *testing.T) {
	if Rupees(1200000).Monthly() != Rupees(100000) || Money(1000).Monthly() != 83 {
		t.Fatal("Monthly")
	}
	for ym, want := range map[[2]int]int{{2026, 2}: 28, {2024, 2}: 29, {2026, 10}: 31, {2026, 11}: 30} {
		if got := DaysInMonth(ym[0], ym[1]); got != want {
			t.Errorf("DaysInMonth(%v) = %d", ym, got)
		}
	}
}
