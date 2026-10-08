// Package calc holds Module 3's pure time math (spec AT-005..AT-010). No I/O, no dependencies.
package calc

import (
	"errors"
	"fmt"
	"time"
)

// DefaultExpectedMinutes is the expected work for an employee without a shift (AT-008).
const DefaultExpectedMinutes = 480

// nightAttributionWindow extends a night shift's day past its end time (AT-005).
const nightAttributionWindow = 4 * 60

// ErrInvalidHHMM reports a time not in 24h HH:MM form.
var ErrInvalidHHMM = errors.New("time must be HH:MM (00:00-23:59)")

// ShiftSpec is the subset of a shift needed for attendance math. Location nil means UTC.
type ShiftSpec struct {
	StartMinute    int
	EndMinute      int
	GraceMins      int
	BreakMins      int
	FullDayMinutes *int
	HalfDayMinutes *int
	Location       *time.Location
}

// ParseHHMM converts "HH:MM" to minutes from midnight.
func ParseHHMM(s string) (int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, ErrInvalidHHMM
	}
	h, okH := twoDigits(s[0:2])
	m, okM := twoDigits(s[3:5])
	if !okH || !okM || h > 23 || m > 59 {
		return 0, ErrInvalidHHMM
	}
	return h*60 + m, nil
}

func twoDigits(s string) (int, bool) {
	if s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' {
		return 0, false
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), true
}

// FormatHHMM renders minutes from midnight as "HH:MM".
func FormatHHMM(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// IsNightShift reports whether a shift crosses midnight (FR-SH003).
func IsNightShift(start, end int) bool { return end <= start }

// SpanMinutes is the shift length including midnight wrap.
func SpanMinutes(start, end int) int {
	if IsNightShift(start, end) {
		return end + 1440 - start
	}
	return end - start
}

// ExpectedMinutes is span minus break, never negative; nil shift → DefaultExpectedMinutes (AT-008).
func ExpectedMinutes(s *ShiftSpec) int {
	if s == nil {
		return DefaultExpectedMinutes
	}
	if v := SpanMinutes(s.StartMinute, s.EndMinute) - s.BreakMins; v > 0 {
		return v
	}
	return 0
}

// Thresholds returns full- and half-day minutes, defaulting to expected and expected/2 (AT-010).
func Thresholds(s *ShiftSpec) (full, half int) {
	expected := ExpectedMinutes(s)
	full, half = expected, expected/2
	if s != nil && s.FullDayMinutes != nil {
		full = *s.FullDayMinutes
	}
	if s != nil && s.HalfDayMinutes != nil {
		half = *s.HalfDayMinutes
	}
	return full, half
}

func location(s *ShiftSpec) *time.Location {
	if s == nil || s.Location == nil {
		return time.UTC
	}
	return s.Location
}

// AttendanceDate returns the day an IN at `at` belongs to, as UTC midnight (AT-005).
// Night shifts own early-morning punches up to end+4h, clamped so they never pass the shift start.
func AttendanceDate(at time.Time, s *ShiftSpec) time.Time {
	local := at.In(location(s))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if s == nil || !IsNightShift(s.StartMinute, s.EndMinute) {
		return day
	}
	cutoff := s.EndMinute + nightAttributionWindow
	if cutoff > s.StartMinute {
		cutoff = s.StartMinute
	}
	if local.Hour()*60+local.Minute() < cutoff {
		return day.AddDate(0, 0, -1)
	}
	return day
}

// ShiftStart is the shift's start instant on the given attendance date.
func ShiftStart(date time.Time, s *ShiftSpec) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), s.StartMinute/60, s.StartMinute%60, 0, 0, location(s))
}
