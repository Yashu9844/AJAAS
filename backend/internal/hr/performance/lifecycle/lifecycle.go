// Package lifecycle is Module 7's pure state machines: appraisal cycles (PM-002) and goals (FR-GL004).
package lifecycle

// Cycle statuses (PM-002).
const (
	CycleDraft     = "draft"
	CycleActive    = "active"
	CycleReview    = "review"
	CycleCompleted = "completed"
)

// Goal statuses (FR-GL004).
const (
	GoalActive    = "active"
	GoalCompleted = "completed"
	GoalCancelled = "cancelled"
)

var cycleNext = map[string]string{CycleDraft: CycleActive, CycleActive: CycleReview, CycleReview: CycleCompleted}

var goalMoves = map[string][]string{GoalActive: {GoalCompleted, GoalCancelled}, GoalCompleted: {GoalActive}}

// CanTransitionCycle reports the only forward step allowed from → to.
func CanTransitionCycle(from, to string) bool { n, ok := cycleNext[from]; return ok && n == to }

// CanTransitionGoal reports a legal goal status change.
func CanTransitionGoal(from, to string) bool {
	for _, s := range goalMoves[from] {
		if s == to {
			return true
		}
	}
	return false
}

// ValidCycleStatus reports a known cycle status (list filters).
func ValidCycleStatus(s string) bool {
	return s == CycleDraft || s == CycleActive || s == CycleReview || s == CycleCompleted
}

// CycleEditable: title, dates and criteria change only in draft (PM-001).
func CycleEditable(s string) bool { return s == CycleDraft }

// GoalsOpen: goals are created, edited, re-weighted and status-changed in draft or active (PM-003).
func GoalsOpen(s string) bool { return s == CycleDraft || s == CycleActive }

// CheckInsOpen: progress check-ins in active or review (PM-007).
func CheckInsOpen(s string) bool { return s == CycleActive || s == CycleReview }

// ReviewsOpen: reviewers are assigned and reviews submitted only in review (PM-011, PM-012).
func ReviewsOpen(s string) bool { return s == CycleReview }
