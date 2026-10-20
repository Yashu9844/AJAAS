// Package pipeline is Module 6's pure state machines: candidate stages (RC-002) and job statuses (FR-JB003).
package pipeline

// Candidate stages.
const (
	Applied   = "applied"
	Screening = "screening"
	Interview = "interview"
	Offer     = "offer"
	Hired     = "hired"
	Rejected  = "rejected"
	Withdrawn = "withdrawn"
)

// Job statuses.
const (
	JobDraft  = "draft"
	JobOpen   = "open"
	JobOnHold = "on_hold"
	JobClosed = "closed"
	JobFilled = "filled"
)

// manualMoves are the stage changes a recruiter may make directly; offer and hired have their own endpoints (RC-002).
var manualMoves = map[string][]string{
	Applied:   {Screening, Rejected, Withdrawn},
	Screening: {Interview, Rejected, Withdrawn},
	Interview: {Rejected, Withdrawn},
	Offer:     {Rejected, Withdrawn},
}

// jobMoves are the manual job transitions; filled is set only by hire (FR-JB003, RC-008).
var jobMoves = map[string][]string{
	JobDraft:  {JobOpen, JobClosed},
	JobOpen:   {JobOnHold, JobClosed},
	JobOnHold: {JobOpen, JobClosed},
}

// CanMove reports whether a recruiter may move a candidate from → to.
func CanMove(from, to string) bool { return contains(manualMoves[from], to) }

// CanTransitionJob reports whether a job may change status from → to.
func CanTransitionJob(from, to string) bool { return contains(jobMoves[from], to) }

// IsTerminal reports a final candidate stage.
func IsTerminal(stage string) bool { return stage == Hired || stage == Rejected || stage == Withdrawn }

// ValidStage reports a known candidate stage.
func ValidStage(stage string) bool {
	return contains([]string{Applied, Screening, Interview, Offer, Hired, Rejected, Withdrawn}, stage)
}

// Editable reports whether a job's details may still change (FR-JB002).
func Editable(status string) bool { return status != JobClosed && status != JobFilled }

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
