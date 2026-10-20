package calc

import (
	"sort"
	"time"
)

// Punch kinds and record statuses produced by ComputeTotals (mirror models constants).
const (
	In            = "in"
	Out           = "out"
	StatusPresent = "present"
	StatusHalfDay = "half_day"
	StatusAbsent  = "absent"
)

// SessionAbandonAfter is how long an open IN stays usable (AT-006).
const SessionAbandonAfter = 20 * time.Hour

// Punch is one non-superseded IN/OUT instant.
type Punch struct {
	Time time.Time
	Type string
}

// Totals is the computed state of one attendance record (FR-AR002).
type Totals struct {
	FirstIn         *time.Time
	LastOut         *time.Time
	WorkMinutes     int
	BreakMinutes    int
	LateMinutes     int
	OvertimeMinutes int
	OpenSession     bool
	Status          string
}

func sorted(punches []Punch) []Punch {
	out := append([]Punch(nil), punches...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

// OpenSessionStart returns the last IN without a later OUT, if younger than SessionAbandonAfter (AT-003/AT-006).
func OpenSessionStart(punches []Punch, now time.Time) (time.Time, bool) {
	ps := sorted(punches)
	if len(ps) == 0 || ps[len(ps)-1].Type != In {
		return time.Time{}, false
	}
	start := ps[len(ps)-1].Time
	if now.Sub(start) > SessionAbandonAfter {
		return time.Time{}, false
	}
	return start, true
}

// ComputeTotals derives work, break, late, overtime and status for one record (AT-006..AT-010).
// A dangling IN followed by another IN is dropped; durations are summed then floored to minutes.
func ComputeTotals(punches []Punch, s *ShiftSpec, date, now time.Time) Totals {
	ps := sorted(punches)
	var t Totals
	var work, brk time.Duration
	var openIn, lastOut *time.Time
	for i := range ps {
		p := ps[i].Time
		switch ps[i].Type {
		case In:
			if t.FirstIn == nil {
				t.FirstIn = &p
			}
			if lastOut != nil && openIn == nil {
				brk += p.Sub(*lastOut)
			}
			openIn = &p
		case Out:
			if openIn != nil {
				work += p.Sub(*openIn)
				openIn = nil
			}
			lastOut = &p
			t.LastOut = &p
		}
	}
	t.WorkMinutes = int(work / time.Minute)
	t.BreakMinutes = int(brk / time.Minute)
	t.OpenSession = openIn != nil && now.Sub(*openIn) <= SessionAbandonAfter
	t.LateMinutes = lateMinutes(t.FirstIn, s, date)
	if over := t.WorkMinutes - ExpectedMinutes(s); over > 0 {
		t.OvertimeMinutes = over
	}
	t.Status = status(t, s)
	return t
}

// lateMinutes is AT-007: minutes past shift start + grace; 0 without a shift.
func lateMinutes(firstIn *time.Time, s *ShiftSpec, date time.Time) int {
	if s == nil || firstIn == nil {
		return 0
	}
	deadline := ShiftStart(date, s).Add(time.Duration(s.GraceMins) * time.Minute)
	if late := int(firstIn.Sub(deadline) / time.Minute); late > 0 {
		return late
	}
	return 0
}

// status is AT-010: open session → present; else by full/half-day thresholds.
func status(t Totals, s *ShiftSpec) string {
	if t.OpenSession {
		return StatusPresent
	}
	full, half := Thresholds(s)
	switch {
	case t.WorkMinutes >= full:
		return StatusPresent
	case t.WorkMinutes >= half:
		return StatusHalfDay
	default:
		return StatusAbsent
	}
}
