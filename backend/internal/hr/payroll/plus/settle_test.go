package plus

import (
	"testing"
	"time"

	"github.com/jaas/jaas/internal/hr/kernel"
)

func dt(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptr(t time.Time) *time.Time { return &t }

// TestServiceYears freezes the completed-years and month remainder counting the last day.
func TestServiceYears(t *testing.T) {
	cases := []struct {
		join, last time.Time
		years, mo  int
	}{
		{dt(2020, 1, 1), dt(2024, 12, 31), 5, 0},
		{dt(2020, 1, 1), dt(2024, 12, 30), 4, 11},
		{dt(2020, 3, 15), dt(2026, 9, 14), 6, 6},
		{dt(2020, 3, 15), dt(2026, 9, 13), 6, 5},
		{dt(2026, 1, 10), dt(2026, 1, 20), 0, 0},
		{dt(2026, 6, 1), dt(2026, 5, 1), 0, 0},
	}
	for _, c := range cases {
		if y, m := serviceYears(c.join, c.last); y != c.years || m != c.mo {
			t.Errorf("%v â†’ %v: got %dy %dm want %dy %dm", c.join.Format("2006-01-02"), c.last.Format("2006-01-02"), y, m, c.years, c.mo)
		}
	}
}

// TestSettleGratuity freezes PG5: five-year eligibility, the six-month rounding and the â‚¹20 lakh cap.
func TestSettleGratuity(t *testing.T) {
	basic := kernel.Rupees(52000) // 52,000 Ã— 15 / 26 = 30,000 per year
	cases := []struct {
		name string
		join time.Time
		last time.Time
		want kernel.Money
		yrs  int
	}{
		{"four years: nothing", dt(2022, 1, 1), dt(2026, 6, 30), 0, 0},
		{"exactly five years", dt(2020, 1, 1), dt(2024, 12, 31), kernel.Rupees(150000), 5},
		{"five years five months does not round up", dt(2020, 1, 1), dt(2025, 5, 31), kernel.Rupees(150000), 5},
		{"five years six months rounds up", dt(2020, 1, 1), dt(2025, 6, 30), kernel.Rupees(180000), 6},
		{"long service", dt(2000, 1, 1), dt(2025, 12, 31), kernel.Rupees(780000), 26},
	}
	for _, c := range cases {
		r := Settle(SettleInput{Joining: c.join, LastDay: c.last, MonthlyBasic: basic, MonthlyGross: basic})
		if r.Gratuity != c.want || r.GratuityYears != c.yrs {
			t.Errorf("%s: got %s over %d years, want %s over %d", c.name, r.Gratuity, r.GratuityYears, c.want, c.yrs)
		}
	}
	rich := Settle(SettleInput{Joining: dt(1990, 1, 1), LastDay: dt(2025, 12, 31), MonthlyBasic: kernel.Rupees(900000), MonthlyGross: kernel.Rupees(900000)})
	if rich.Gratuity != kernel.Rupees(2000000) {
		t.Fatalf("cap: %s", rich.Gratuity)
	}
}

// TestSettlePayouts freezes leave, encashment and notice arithmetic.
func TestSettlePayouts(t *testing.T) {
	in := SettleInput{Joining: dt(2025, 1, 1), LastDay: dt(2026, 6, 30), MonthlyBasic: kernel.Rupees(26000), MonthlyGross: kernel.Rupees(60000),
		LeaveDays: 1050, EncashedDays: 200}
	r := Settle(in)
	// BASIC Ã· 26 = 1,000 a day: 10.5 days and 2 days.
	if r.LeaveEncash != kernel.Rupees(10500) || r.Encashments != kernel.Rupees(2000) || r.Gratuity != 0 || r.Net != kernel.Rupees(12500) {
		t.Fatalf("payouts: %+v", r)
	}
	in.Resignation, in.NoticeDays = ptr(dt(2026, 6, 15)), 60 // served 15 of 60 days
	r = Settle(in)
	if r.NoticeShort != 45 || r.NoticeRecovery != kernel.Rupees(12500) || r.Shortfall != kernel.Rupees(90000)-kernel.Rupees(12500) || r.Net != 0 {
		t.Fatalf("recovery is capped at earnings, the rest is the shortfall: %+v", r)
	}
	in.Resignation, in.NoticeDays = ptr(dt(2026, 4, 1)), 60 // served 90 days
	if r = Settle(in); r.NoticeRecovery != 0 || r.NoticeShort != 0 || r.Net != kernel.Rupees(12500) {
		t.Fatalf("notice served: %+v", r)
	}
	in.Resignation, in.NoticeDays = ptr(dt(2026, 6, 20)), 30 // short by 20 days: 60,000 Ã— 20 / 30 = 40,000
	in.LeaveDays, in.EncashedDays = kernel.FromWhole(50), 0
	if r = Settle(in); r.NoticeRecovery != kernel.Rupees(40000) || r.Net != kernel.Rupees(50000)-kernel.Rupees(40000) || r.Shortfall != 0 {
		t.Fatalf("partial recovery: %+v", r)
	}
	in.Resignation = ptr(dt(2026, 7, 5)) // recorded after the last day: inconsistent data never produces a recovery
	if r = Settle(in); r.NoticeRecovery != 0 || r.NoticeShort != 0 {
		t.Fatalf("resignation after the last day: %+v", r)
	}
	in.Resignation = nil
	if r = Settle(in); r.NoticeRecovery != 0 {
		t.Fatal("no resignation date, no recovery")
	}
}
