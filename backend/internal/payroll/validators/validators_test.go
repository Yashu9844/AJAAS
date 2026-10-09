package validators

import (
	"testing"
	"time"

	"github.com/jaas/jaas/internal/payroll/calc"
)

func TestParseDateAndCTC(t *testing.T) {
	if d, err := ParseDate("2026-10-01"); err != nil || d.Day() != 1 {
		t.Fatal("ParseDate")
	}
	if _, err := ParseDate("01/10/2026"); err != ErrDate {
		t.Fatal("bad date")
	}
	for _, ok := range []calc.Money{1, calc.Rupees(1200000), MaxAnnualCTC} {
		if ValidateCTC(ok) != nil {
			t.Errorf("CTC %s should pass", ok)
		}
	}
	for _, bad := range []calc.Money{0, -1, MaxAnnualCTC + 1} {
		if ValidateCTC(bad) != ErrCTC {
			t.Errorf("CTC %s should fail", bad)
		}
	}
}

func TestValidatePeriod(t *testing.T) {
	today := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		y, m int
		ok   bool
	}{{2026, 10, true}, {2026, 9, true}, {2025, 12, true}, {2026, 11, false}, {2027, 1, false}} {
		if (ValidatePeriod(c.y, c.m, today) == nil) != c.ok {
			t.Errorf("period %d-%d", c.y, c.m)
		}
	}
}

// PS-T8: formula-leading cells are neutralized.
func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"=SUM(A1)": "'=SUM(A1)", "+91": "'+91", "-5": "'-5", "@x": "'@x", "\tx": "'\tx",
		"Asha": "Asha", "": "", "a\nb\rc": "a b c"} {
		if got := CSVSafe(in); got != want {
			t.Errorf("CSVSafe(%q) = %q, want %q", in, got, want)
		}
	}
}
