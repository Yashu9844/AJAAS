// Package lifecycle is HR/Core's pure rules: the HR action state graph (CO-030) and the action types (CO-033..CO-038).
package lifecycle

// Action statuses (CO-030, HR-013).
const (
	Draft     = "draft"
	Submitted = "submitted"
	Completed = "completed"
	Rejected  = "rejected"
	Cancelled = "cancelled"
	Rescinded = "rescinded"
)

// Action types (CO-033..CO-038).
const (
	Hire             = "hire"
	Promote          = "promote"
	Demote           = "demote"
	Transfer         = "transfer"
	ChangeJob        = "change_job"
	ChangeManager    = "change_manager"
	ChangeLocation   = "change_location"
	ProbationConfirm = "probation_confirm"
	Terminate        = "terminate"
	Rehire           = "rehire"
)

var moves = map[string][]string{
	Draft:     {Submitted, Cancelled},
	Submitted: {Completed, Rejected, Cancelled},
	Completed: {Rescinded},
}

// CanMove reports a legal action status change.
func CanMove(from, to string) bool {
	for _, s := range moves[from] {
		if s == to {
			return true
		}
	}
	return false
}

// ValidStatus reports a known action status (list filters).
func ValidStatus(s string) bool {
	_, ok := moves[s]
	return ok || s == Rejected || s == Cancelled || s == Rescinded
}

var types = map[string]bool{Hire: true, Promote: true, Demote: true, Transfer: true, ChangeJob: true, ChangeManager: true,
	ChangeLocation: true, ProbationConfirm: true, Terminate: true, Rehire: true}

// ValidType reports a known action type.
func ValidType(t string) bool { return types[t] }

// Starts reports types that open a worker's first (or a new) employment: they need no current assignment.
func Starts(t string) bool { return t == Hire || t == Rehire }

// Changes reports types that replace the current assignment with a modified copy.
func Changes(t string) bool {
	switch t {
	case Promote, Demote, Transfer, ChangeJob, ChangeManager, ChangeLocation, ProbationConfirm:
		return true
	}
	return false
}

// Worker and employment types and job categories (CO-001, CO-033).
var (
	WorkerTypes     = map[string]bool{"employee": true, "contractor": true, "intern": true}
	EmploymentTypes = map[string]bool{"full_time": true, "part_time": true, "contract": true, "intern": true}
	Categories      = map[string]bool{"staff": true, "management": true, "executive": true}
	Termination     = map[string]bool{"voluntary": true, "involuntary": true}
)

// Position status moves (CO-013): active ↔ frozen, active|frozen → closed; closed is final.
func CanMovePosition(from, to string) bool {
	switch from {
	case "active":
		return to == "frozen" || to == "closed"
	case "frozen":
		return to == "active" || to == "closed"
	}
	return false
}
