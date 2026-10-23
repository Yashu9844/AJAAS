// Package plus is the HR/Payroll parity package (docs/modules/m1-hr-management/contexts/payroll, PP-02, PP-06..PP-09):
// professional-tax slab management, the worker state lookup and final settlements.
package plus

import (
	"time"

	"github.com/jaas/jaas/internal/hr/kernel"
)

// Settlement constants (PP-06).
const (
	gratuityMinYears = 5
	gratuityDaysNum  = 15
	gratuityDaysDen  = 26
	leaveDaysDen     = 26
	noticeDaysDen    = 30
)

// gratuityCap is the statutory ceiling of â‚¹20,00,000.
var gratuityCap = kernel.Rupees(2000000)

// SettleInput is everything the settlement arithmetic needs; money is exact paise, leave days are hundredths.
type SettleInput struct {
	Joining, LastDay time.Time
	Resignation      *time.Time
	NoticeDays       int
	MonthlyBasic     kernel.Money
	MonthlyGross     kernel.Money
	LeaveDays        kernel.Days // available days of encashable leave types
	EncashedDays     kernel.Days // approved, unpaid encashment requests
}

// SettleResult is the computed settlement (PP-06).
type SettleResult struct {
	CompletedYears int
	GratuityYears  int
	Gratuity       kernel.Money
	LeaveEncash    kernel.Money
	Encashments    kernel.Money
	NoticeShort    int
	NoticeRecovery kernel.Money
	Shortfall      kernel.Money // recovery that earnings could not cover
	Net            kernel.Money
}

// serviceYears returns the completed years of service and the whole months beyond them, counting the last day.
func serviceYears(joining, lastDay time.Time) (years, months int) {
	end := lastDay.AddDate(0, 0, 1)
	years = end.Year() - joining.Year()
	if end.Month() < joining.Month() || (end.Month() == joining.Month() && end.Day() < joining.Day()) {
		years--
	}
	if years < 0 {
		return 0, 0
	}
	anniversary := joining.AddDate(years, 0, 0)
	for m := 1; ; m++ {
		if anniversary.AddDate(0, m, 0).After(end) {
			return years, m - 1
		}
	}
}

// Settle computes the final settlement (PP-06): gratuity from five completed years (a remainder of six months or more
// rounds the years up, capped at â‚¹20,00,000), leave and encashment payouts at BASIC Ã· 26 per day, and notice recovery.
func Settle(in SettleInput) SettleResult {
	var r SettleResult
	years, months := serviceYears(in.Joining, in.LastDay)
	r.CompletedYears = years
	if years >= gratuityMinYears {
		r.GratuityYears = years
		if months >= 6 {
			r.GratuityYears++
		}
		g := kernel.DivRound(int64(in.MonthlyBasic)*gratuityDaysNum*int64(r.GratuityYears), gratuityDaysDen).RoundRupee()
		r.Gratuity = kernel.Min(g, gratuityCap)
	}
	daily := kernel.DivRound(int64(in.MonthlyBasic), leaveDaysDen)
	r.LeaveEncash = kernel.DivRound(int64(daily)*int64(in.LeaveDays), 100).RoundRupee()
	r.Encashments = kernel.DivRound(int64(daily)*int64(in.EncashedDays), 100).RoundRupee()
	if in.Resignation != nil && !in.Resignation.After(in.LastDay) && in.NoticeDays > 0 {
		served := int(in.LastDay.Sub(*in.Resignation).Hours() / 24)
		if short := in.NoticeDays - served; short > 0 {
			r.NoticeShort = short
			r.NoticeRecovery = kernel.DivRound(int64(in.MonthlyGross)*int64(short), noticeDaysDen).RoundRupee()
		}
	}
	earnings := r.Gratuity + r.LeaveEncash + r.Encashments
	if r.NoticeRecovery > earnings {
		r.Shortfall = r.NoticeRecovery - earnings
		r.NoticeRecovery = earnings
	}
	r.Net = earnings - r.NoticeRecovery
	return r
}
