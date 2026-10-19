// Package validators holds Module 7 field rules that struct tags cannot express.
package validators

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaas/jaas/internal/performance/models"
	"github.com/jaas/jaas/internal/performance/score"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// Field errors returned as VALIDATION_ERROR messages.
var (
	ErrDate      = errors.New("date must be YYYY-MM-DD")
	ErrDateOrder = errors.New("end_date must not be before start_date")
	ErrCriteria  = errors.New("criteria must be 1–10 distinct names of 1–50 characters")
	ErrWeight    = errors.New("weight must be between 0.01 and 100")
	ErrProgress  = errors.New("progress must be between 0 and 100")
	ErrKeyResult = errors.New("target_value must differ from start_value")
	ErrRatings   = errors.New("ratings must score every cycle criterion exactly once with an integer from 1 to 5")
	ErrPotential = errors.New("potential (low|medium|high) is required for manager reviews and not allowed otherwise")
)

// DefaultCriteria apply when a cycle is created without criteria (FR-CY001).
var DefaultCriteria = []string{"Quality", "Delivery", "Collaboration", "Ownership"}

const (
	maxCriteria  = 10
	maxCriterion = 50
	minRating    = 1
	maxRating    = 5
)

// potentials are the 9-box potential levels (PM-012, PM-015).
var potentials = map[string]bool{"low": true, "medium": true, "high": true}

// Criteria normalizes PM-001: trimmed, 1–50 characters, unique ignoring case, at most 10, order kept; empty → defaults.
func Criteria(in []string) ([]string, error) {
	if len(in) == 0 {
		return append([]string(nil), DefaultCriteria...), nil
	}
	if len(in) > maxCriteria {
		return nil, ErrCriteria
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = strings.TrimSpace(c)
		key := strings.ToLower(c)
		if c == "" || utf8.RuneCountInString(c) > maxCriterion || seen[key] {
			return nil, ErrCriteria
		}
		seen[key] = true
		out = append(out, c)
	}
	return out, nil
}

// DateRange parses start/end dates (UTC midnight) and checks end ≥ start.
func DateRange(start, end string) (time.Time, time.Time, error) {
	s, err := time.Parse(DateLayout, start)
	if err != nil {
		return time.Time{}, time.Time{}, ErrDate
	}
	e, err := time.Parse(DateLayout, end)
	if err != nil {
		return time.Time{}, time.Time{}, ErrDate
	}
	if e.Before(s) {
		return time.Time{}, time.Time{}, ErrDateOrder
	}
	return s, e, nil
}

// Weight checks PM-004's per-goal bound 0.01–100.
func Weight(w score.Fixed) error {
	if w <= 0 || w > score.Hundred {
		return ErrWeight
	}
	return nil
}

// Progress checks 0–100.
func Progress(p score.Fixed) error {
	if p < 0 || p > score.Hundred {
		return ErrProgress
	}
	return nil
}

// KeyResult rejects a degenerate key result (FR-GL005).
func KeyResult(start, target score.Fixed) error {
	if start == target {
		return ErrKeyResult
	}
	return nil
}

// Ratings checks PM-012: every criterion exactly once (case-insensitive), scores 1–5.
func Ratings(criteria []string, ratings models.Ratings) error {
	if len(ratings) != len(criteria) {
		return ErrRatings
	}
	want := map[string]bool{}
	for _, c := range criteria {
		want[strings.ToLower(c)] = true
	}
	for _, r := range ratings {
		key := strings.ToLower(strings.TrimSpace(r.Criterion))
		if !want[key] || r.Score < minRating || r.Score > maxRating {
			return ErrRatings
		}
		delete(want, key)
	}
	return nil
}

// Canonical returns validated ratings in the cycle's criteria order and spelling.
func Canonical(criteria []string, ratings models.Ratings) models.Ratings {
	by := map[string]int{}
	for _, r := range ratings {
		by[strings.ToLower(strings.TrimSpace(r.Criterion))] = r.Score
	}
	out := make(models.Ratings, len(criteria))
	for i, c := range criteria {
		out[i] = models.Rating{Criterion: c, Score: by[strings.ToLower(c)]}
	}
	return out
}

// Potential enforces PM-012: required (low|medium|high) for manager reviews, absent otherwise.
func Potential(reviewType string, p *string) error {
	if reviewType != models.ReviewManager {
		if p != nil {
			return ErrPotential
		}
		return nil
	}
	if p == nil || !potentials[*p] {
		return ErrPotential
	}
	return nil
}
