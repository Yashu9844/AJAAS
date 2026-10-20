// Package calc is HR/Leave's pure leave arithmetic: day counting, accrual, carry-forward. The Days amount type lives
// in the HR kernel; the aliases keep this package's API unchanged.
package calc

import "github.com/jaas/jaas/internal/hr/kernel"

// Days is an amount of leave in hundredths of a day (kernel.Days).
type Days = kernel.Days

// FromWhole converts whole days to Days.
func FromWhole(n int) Days { return kernel.FromWhole(n) }

// ParseDays parses a decimal day amount with at most two decimals.
func ParseDays(s string) (Days, error) { return kernel.ParseDays(s) }
