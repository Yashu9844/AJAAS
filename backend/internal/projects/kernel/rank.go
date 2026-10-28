package kernel

import "strings"

// Ranks order tasks inside a column. They are strings over a base-62 alphabet compared bytewise (the column uses
// COLLATE "C"), so a move computes one new value between two neighbours and touches one row (TK-05).
const rankDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// RankBetween returns a rank strictly between lo and hi; "" means unbounded on that side. It returns "" when lo is not
// below hi.
func RankBetween(lo, hi string) string {
	if hi != "" && lo >= hi {
		return ""
	}
	return midpoint(lo, hi)
}

func midpoint(lo, hi string) string {
	if hi != "" {
		n := 0
		for n < len(hi) && digitAt(lo, n) == hi[n] {
			n++
		}
		if n > 0 {
			return hi[:n] + midpoint(tail(lo, n), hi[n:])
		}
	}
	a, b := 0, len(rankDigits)
	if lo != "" {
		a = strings.IndexByte(rankDigits, lo[0])
	}
	if hi != "" {
		b = strings.IndexByte(rankDigits, hi[0])
	}
	if b-a > 1 {
		return string(rankDigits[(a+b+1)/2])
	}
	if len(hi) > 1 {
		return hi[:1]
	}
	return string(rankDigits[a]) + midpoint(tail(lo, 1), "")
}

func digitAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return rankDigits[0]
}

func tail(s string, n int) string {
	if n >= len(s) {
		return ""
	}
	return s[n:]
}

// ValidRank reports whether s is a well-formed rank (non-empty, alphabet only, no trailing zero digit).
func ValidRank(s string) bool {
	if s == "" || len(s) > 64 || s[len(s)-1] == rankDigits[0] {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(rankDigits, s[i]) < 0 {
			return false
		}
	}
	return true
}

// Spread returns n increasing ranks, used to rebalance a column whose ranks have grown long.
func Spread(n int) []string {
	out := make([]string, 0, n)
	prev := ""
	for i := 0; i < n; i++ {
		prev = RankBetween(prev, "")
		out = append(out, prev)
	}
	return out
}
