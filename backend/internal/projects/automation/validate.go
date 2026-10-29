package automation

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

var (
	priorities = map[string]bool{"none": true, "low": true, "medium": true, "high": true, "urgent": true}
	categories = map[string]bool{"backlog": true, "todo": true, "in_progress": true, "done": true, "canceled": true}
	// stateFields read the task as it is now; they are valid for every trigger.
	stateFields = map[string]string{FieldColumn: "uuid", FieldCategory: "category", FieldPriority: "priority", FieldLabel: "uuid", FieldAssignee: "uuid"}
	// eventFields read the triggering event and are valid for one trigger only.
	eventFields = map[string]string{FieldToColumn: "task.moved", FieldFromColumn: "task.moved", FieldAssignedUser: "task.assigned"}
)

func validTrigger(t string) bool {
	for _, k := range Triggers {
		if k == t {
			return true
		}
	}
	return false
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxName {
		return "", kernel.Invalid("name", "length must be between 1 and 80")
	}
	return name, nil
}

// scope is where the ids of a rule are checked: the project and tenant the rule belongs to.
type scope struct {
	s         *Service
	tenantID  uuid.UUID
	projectID uuid.UUID
}

// check validates a whole rule definition and returns it normalised.
func (sc scope) check(ctx context.Context, in RuleInput) (RuleInput, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return in, err
	}
	in.Name = name
	if !validTrigger(in.Trigger) {
		return in, kernel.Invalid("trigger", "must be one of "+strings.Join(Triggers, ", "))
	}
	if len(in.Conditions) > maxConditions {
		return in, kernel.Invalid("conditions", "a rule has at most 10 conditions")
	}
	for _, c := range in.Conditions {
		if err := sc.condition(ctx, in.Trigger, c); err != nil {
			return in, err
		}
	}
	if len(in.Actions) < 1 || len(in.Actions) > maxActions {
		return in, kernel.Invalid("actions", "give between 1 and 10 actions")
	}
	for i, a := range in.Actions {
		if err := sc.action(ctx, a); err != nil {
			return in, kernel.Invalid("actions", "action "+itoa(i+1)+": "+err.Error())
		}
	}
	if in.Conditions == nil {
		in.Conditions = Conditions{}
	}
	return in, nil
}

func (sc scope) condition(ctx context.Context, trigger string, c Condition) error {
	kind, state := stateFields[c.Field]
	if !state {
		only, ok := eventFields[c.Field]
		if !ok {
			return kernel.Invalid("conditions", "unknown field "+c.Field)
		}
		if only != trigger {
			return kernel.Invalid("conditions", c.Field+" only applies to the "+only+" trigger")
		}
		kind = "uuid"
	}
	switch kind {
	case "category":
		if !categories[c.Value] {
			return kernel.Invalid("conditions", "column_category must be backlog, todo, in_progress, done or canceled")
		}
	case "priority":
		if !priorities[c.Value] {
			return kernel.Invalid("conditions", "priority must be none, low, medium, high or urgent")
		}
	default:
		id, err := uuid.Parse(c.Value)
		if err != nil {
			return kernel.Invalid("conditions", c.Field+" must be a UUID")
		}
		return sc.condRef(ctx, c.Field, id)
	}
	return nil
}

// condRef checks that a column or label named by a condition belongs to this project (users are matched, not resolved).
func (sc scope) condRef(ctx context.Context, field string, id uuid.UUID) error {
	switch field {
	case FieldColumn, FieldToColumn, FieldFromColumn:
		return sc.column(ctx, id)
	case FieldLabel:
		return sc.label(ctx, id)
	}
	return nil
}

func (sc scope) column(ctx context.Context, id uuid.UUID) error {
	n, err := countRows(sc.s.db.WithContext(ctx), "board_columns", "id = ? AND project_id = ? AND tenant_id = ?", id, sc.projectID, sc.tenantID)
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("column_id", "is not a column of this project")
	}
	return nil
}

func (sc scope) label(ctx context.Context, id uuid.UUID) error {
	n, err := countRows(sc.s.db.WithContext(ctx), "labels", "id = ? AND tenant_id = ? AND (project_id = ? OR project_id IS NULL)", id, sc.tenantID, sc.projectID)
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("label_id", "is not a label of this project")
	}
	return nil
}

func (sc scope) member(ctx context.Context, id uuid.UUID) error {
	n, err := countRows(sc.s.db.WithContext(ctx), "project_members", "project_id = ? AND user_id = ? AND tenant_id = ? AND active", sc.projectID, id, sc.tenantID)
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("user_id", "must be an active member of the project")
	}
	return nil
}

// action validates one action: exactly the member of its type is set.
func (sc scope) action(ctx context.Context, a Action) error {
	extra := a
	switch a.Type {
	case ActionMove:
		extra.ColumnID = nil
		if a.ColumnID == nil {
			return kernel.Invalid("column_id", "is required")
		}
		return sc.sole(extra, sc.column(ctx, *a.ColumnID))
	case ActionPriority:
		extra.Priority = ""
		if !priorities[a.Priority] {
			return kernel.Invalid("priority", "must be none, low, medium, high or urgent")
		}
	case ActionLabel:
		extra.LabelID = nil
		if a.LabelID == nil {
			return kernel.Invalid("label_id", "is required")
		}
		return sc.sole(extra, sc.label(ctx, *a.LabelID))
	case ActionAssign:
		extra.UserID = nil
		if a.UserID == nil {
			return kernel.Invalid("user_id", "is required")
		}
		return sc.sole(extra, sc.member(ctx, *a.UserID))
	case ActionComment:
		extra.Text = ""
		if n := utf8.RuneCountInString(strings.TrimSpace(a.Text)); n < 1 || n > maxText {
			return kernel.Invalid("text", "length must be between 1 and 500")
		}
	case ActionDue:
		extra.Days = nil
		if a.Days == nil || *a.Days < 0 || *a.Days > maxDays {
			return kernel.Invalid("days", "must be between 0 and 365")
		}
	default:
		return kernel.Invalid("type", "must be move_to_column, set_priority, add_label, assign_user, add_comment or set_due_in_days")
	}
	return sc.sole(extra, nil)
}

// sole fails with err, or when the action carries members that do not belong to its type.
func (sc scope) sole(extra Action, err error) error {
	if err != nil {
		return err
	}
	extra.Type = ""
	if extra.ColumnID != nil || extra.Priority != "" || extra.LabelID != nil || extra.UserID != nil || extra.Text != "" || extra.Days != nil {
		return kernel.Invalid("action", "carries a member that does not belong to its type")
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
