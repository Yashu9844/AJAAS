package kernel

import "github.com/jaas/jaas/internal/shared/rank"

// RankBetween returns a rank strictly between lo and hi ("" is unbounded); "" when lo is not below hi (TK-05).
func RankBetween(lo, hi string) string { return rank.Between(lo, hi) }

// ValidRank reports whether s is a well-formed rank.
func ValidRank(s string) bool { return rank.Valid(s) }

// Spread returns n increasing ranks, used to rebalance a column whose ranks have grown long.
func Spread(n int) []string { return rank.Spread(n) }
