package automation

import (
	"github.com/google/uuid"
)

// eventFacts are the event-specific values a condition can test; missing ones never match.
type eventFacts struct {
	fromColumn *uuid.UUID
	toColumn   *uuid.UUID
	users      []uuid.UUID
}

func hasID(ids []uuid.UUID, want string) bool {
	for _, id := range ids {
		if id.String() == want {
			return true
		}
	}
	return false
}

func idIs(id *uuid.UUID, want string) bool { return id != nil && id.String() == want }

// isEvent reports whether the condition reads the event rather than the task.
func (c Condition) isEvent() bool {
	_, ok := eventFields[c.Field]
	return ok
}

// holds evaluates one condition against the task now and the event.
func (c Condition) holds(st TaskState, ev eventFacts) bool {
	switch c.Field {
	case FieldColumn:
		return st.ColumnID.String() == c.Value
	case FieldCategory:
		return st.ColumnCategory == c.Value
	case FieldPriority:
		return st.Priority == c.Value
	case FieldLabel:
		return hasID(st.LabelIDs, c.Value)
	case FieldAssignee:
		return hasID(st.AssigneeIDs, c.Value)
	case FieldToColumn:
		return idIs(ev.toColumn, c.Value)
	case FieldFromColumn:
		return idIs(ev.fromColumn, c.Value)
	case FieldAssignedUser:
		return hasID(ev.users, c.Value)
	}
	return false
}

// failing lists the fields of the conditions that do not hold; event conditions are skipped when assume is true
// (a dry run has no event).
func (cs Conditions) failing(st TaskState, ev eventFacts, assume bool) []string {
	var out []string
	for _, c := range cs {
		if assume && c.isEvent() {
			continue
		}
		if !c.holds(st, ev) {
			out = append(out, c.Field)
		}
	}
	return out
}

// assumed lists the event conditions a dry run could not check.
func (cs Conditions) assumed() []string {
	out := []string{}
	for _, c := range cs {
		if c.isEvent() {
			out = append(out, c.Field)
		}
	}
	return out
}

// needs is the project permission an action requires from the rule creator.
func (a Action) needs() string {
	switch a.Type {
	case ActionAssign:
		return "task.assign"
	case ActionComment:
		return "task.comment"
	}
	return "task.update"
}

// noop reports whether the action would change nothing for the task in state st.
func (a Action) noop(st TaskState) bool {
	switch a.Type {
	case ActionMove:
		return a.ColumnID != nil && st.ColumnID == *a.ColumnID
	case ActionPriority:
		return st.Priority == a.Priority
	case ActionLabel:
		return a.LabelID != nil && hasID(st.LabelIDs, a.LabelID.String())
	case ActionAssign:
		return a.UserID != nil && hasID(st.AssigneeIDs, a.UserID.String())
	}
	return false
}

// applied returns the state after the action ran, so later actions of the same run see its effect.
func (a Action) applied(st TaskState) TaskState {
	switch a.Type {
	case ActionMove:
		if a.ColumnID != nil {
			st.ColumnID = *a.ColumnID
		}
	case ActionPriority:
		st.Priority = a.Priority
	case ActionLabel:
		if a.LabelID != nil {
			st.LabelIDs = append(append([]uuid.UUID{}, st.LabelIDs...), *a.LabelID)
		}
	case ActionAssign:
		if a.UserID != nil {
			st.AssigneeIDs = append(append([]uuid.UUID{}, st.AssigneeIDs...), *a.UserID)
		}
	}
	return st
}
