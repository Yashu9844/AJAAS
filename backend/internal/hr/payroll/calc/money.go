// Package calc is HR/Payroll's pure payroll arithmetic: structure breakdown, proration and Indian statutory
// deductions. No I/O; every rule is golden-tested. The Money amount type lives in the HR kernel; the aliases keep this
// package's API unchanged.
package calc

import "github.com/jaas/jaas/internal/hr/kernel"

// Money is an exact 2-decimal amount in hundredths (kernel.Money).
type Money = kernel.Money

// Rupees converts whole rupees to Money.
func Rupees(n int64) Money { return kernel.Rupees(n) }

// ParseMoney parses a decimal amount with at most two decimals.
func ParseMoney(s string) (Money, error) { return kernel.ParseMoney(s) }

// Min returns the smaller amount.
func Min(a, b Money) Money { return kernel.Min(a, b) }

func divRound(n, d int64) Money { return kernel.DivRound(n, d) }
