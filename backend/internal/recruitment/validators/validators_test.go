package validators

import (
	"testing"
	"time"

	"github.com/jaas/jaas/internal/payroll/calc"
)

func TestValidators(t *testing.T) {
	today := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	if _, err := ParseDate("x"); err != ErrDate {
		t.Fatal("bad date")
	}
	if d, err := ParseFutureDate("2026-10-09", today); err != nil || d.Day() != 9 {
		t.Fatal("today allowed")
	}
	if _, err := ParseFutureDate("2026-10-08", today); err != ErrPastDate {
		t.Fatal("past rejected")
	}
	if _, err := ParseFutureDate("bad", today); err != ErrDate {
		t.Fatal("bad future date")
	}
	now := time.Now()
	if FutureInstant(now.Add(time.Hour), now) != nil || FutureInstant(now, now) != ErrPastTime {
		t.Fatal("FutureInstant")
	}
	if ValidateCTC(calc.Rupees(600000)) != nil || ValidateCTC(0) != ErrCTC || ValidateCTC(calc.Rupees(1_000_000_001)) != ErrCTC {
		t.Fatal("ValidateCTC")
	}
	if NormalizeEmail("  Asha@Example.COM ") != "asha@example.com" {
		t.Fatal("NormalizeEmail")
	}
}
