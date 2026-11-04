// Package rrule parses, validates and expands the RFC 5545 recurrence-rule subset M3 accepts (docs/modules/m3-meetings/
// decisions.md DMT-03): FREQ DAILY|WEEKLY|MONTHLY|YEARLY, INTERVAL, COUNT or UNTIL, BYDAY (ordinals for MONTHLY and
// YEARLY), BYMONTHDAY, BYMONTH and WKST. Everything else (BYSETPOS, sub-daily frequencies, BYHOUR...) is refused, so the
// cost of an expansion is bounded. Expansion runs in the series time zone: the wall-clock time stays fixed across DST.
package rrule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Limits of an accepted rule.
const (
	MaxCount    = 500
	MaxInterval = 99
	MaxUntil    = 5 * 365 * 24 * time.Hour // UNTIL at most about five years after the first start
	untilLayout = "20060102T150405Z"
	dateLayout  = "20060102"
)

// Frequencies.
const (
	Daily   = "DAILY"
	Weekly  = "WEEKLY"
	Monthly = "MONTHLY"
	Yearly  = "YEARLY"
)

// ErrRule is wrapped by every validation error.
var ErrRule = errors.New("invalid recurrence rule")

// WeekdayNum is one BYDAY entry: a weekday with an optional ordinal (2MO = second Monday, -1FR = last Friday, 0 = every).
type WeekdayNum struct {
	N   int
	Day time.Weekday
}

// Rule is a parsed and validated rule.
type Rule struct {
	Freq       string
	Interval   int
	Count      int        // 0: none
	Until      *time.Time // UTC; nil: none
	ByDay      []WeekdayNum
	ByMonthDay []int
	ByMonth    []int
	WeekStart  time.Weekday
}

var dayCodes = map[string]time.Weekday{"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday}

func fail(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrRule}, a...)...)
}

// Parse reads and validates a rule; the optional "RRULE:" prefix is accepted.
func Parse(s string) (Rule, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "RRULE:")
	if s == "" || len(s) > 500 {
		return Rule{}, fail("empty or longer than 500 characters")
	}
	r := Rule{Interval: 1, WeekStart: time.Monday}
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, "=")
		k = strings.ToUpper(strings.TrimSpace(k))
		if !ok || v == "" || seen[k] {
			return Rule{}, fail("malformed or repeated part %q", part)
		}
		seen[k] = true
		if err := r.set(k, strings.ToUpper(strings.TrimSpace(v))); err != nil {
			return Rule{}, err
		}
	}
	return r, r.validate()
}

func (r *Rule) set(k, v string) error {
	var err error
	switch k {
	case "FREQ":
		r.Freq = v
	case "INTERVAL":
		r.Interval, err = intIn(v, 1, MaxInterval)
	case "COUNT":
		r.Count, err = intIn(v, 1, MaxCount)
	case "UNTIL":
		r.Until, err = parseUntil(v)
	case "BYDAY":
		r.ByDay, err = parseByDay(v)
	case "BYMONTHDAY":
		r.ByMonthDay, err = intList(v, -31, 31)
	case "BYMONTH":
		r.ByMonth, err = intList(v, 1, 12)
	case "WKST":
		d, ok := dayCodes[v]
		if !ok {
			return fail("WKST %q", v)
		}
		r.WeekStart = d
	default:
		return fail("%s is not supported", k)
	}
	return err
}

func (r Rule) validate() error {
	switch r.Freq {
	case Daily, Weekly, Monthly, Yearly:
	default:
		return fail("FREQ must be DAILY, WEEKLY, MONTHLY or YEARLY")
	}
	if r.Count > 0 && r.Until != nil {
		return fail("COUNT and UNTIL cannot both be set")
	}
	for _, d := range r.ByDay {
		if d.N != 0 && r.Freq != Monthly && r.Freq != Yearly {
			return fail("BYDAY ordinals need FREQ=MONTHLY or YEARLY")
		}
		if d.N != 0 && r.Freq == Yearly && len(r.ByMonth) == 0 {
			return fail("BYDAY ordinals with FREQ=YEARLY need BYMONTH")
		}
	}
	if len(r.ByMonthDay) > 0 && r.Freq == Weekly {
		return fail("BYMONTHDAY is not allowed with FREQ=WEEKLY")
	}
	return nil
}

func intIn(v string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, fail("%q must be between %d and %d", v, lo, hi)
	}
	return n, nil
}

func intList(v string, lo, hi int) ([]int, error) {
	parts := strings.Split(v, ",")
	if len(parts) > 31 {
		return nil, fail("too many values in %q", v)
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := intIn(p, lo, hi)
		if err != nil || n == 0 {
			return nil, fail("value %q out of range", p)
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func parseUntil(v string) (*time.Time, error) {
	for _, layout := range []string{untilLayout, dateLayout} {
		if t, err := time.Parse(layout, v); err == nil {
			if layout == dateLayout {
				t = t.Add(24*time.Hour - time.Second) // a date UNTIL includes the whole day
			}
			return &t, nil
		}
	}
	return nil, fail("UNTIL %q must be YYYYMMDD or YYYYMMDDTHHMMSSZ", v)
}

func parseByDay(v string) ([]WeekdayNum, error) {
	parts := strings.Split(v, ",")
	if len(parts) > 7*5 {
		return nil, fail("too many BYDAY values")
	}
	out := make([]WeekdayNum, 0, len(parts))
	for _, p := range parts {
		if len(p) < 2 {
			return nil, fail("BYDAY %q", p)
		}
		d, ok := dayCodes[p[len(p)-2:]]
		if !ok {
			return nil, fail("BYDAY %q", p)
		}
		n := 0
		if num := p[:len(p)-2]; num != "" {
			var err error
			if n, err = strconv.Atoi(num); err != nil || n == 0 || n < -5 || n > 5 {
				return nil, fail("BYDAY ordinal %q must be 1..5 or -1..-5", p)
			}
		}
		out = append(out, WeekdayNum{N: n, Day: d})
	}
	return out, nil
}
