// Package automation is the workflow-rules context of M2 (specification section 9): when a task event happens and every
// condition holds, a fixed list of actions runs as the rule creator. There is no free text matching, no expression
// language and no code: conditions and actions are closed vocabularies validated on write.
package automation

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Limits of a project's rules.
const (
	maxRules      = 50
	maxActions    = 10
	maxConditions = 10
	maxName       = 80
	maxText       = 500
	maxDays       = 365
	// MaxActionsPerMinute caps what one rule may do per minute (loop and flood protection).
	MaxActionsPerMinute = 100
	// MaxConsecutiveFailures disables a rule that failed this many runs in a row.
	MaxConsecutiveFailures = 20
)

// Triggers are the routing keys a rule can react to.
var Triggers = []string{"task.created", "task.moved", "task.assigned", "task.completed", "task.commented", "task.due_changed", "task.pr_merged"}

// Condition fields. State fields read the task now; event fields read the triggering event.
const (
	FieldColumn         = "column_id"
	FieldCategory       = "column_category"
	FieldPriority       = "priority"
	FieldLabel          = "label_id"
	FieldAssignee       = "assignee_id"
	FieldToColumn       = "to_column_id"
	FieldFromColumn     = "from_column_id"
	FieldAssignedUser   = "assigned_user_id"
	ActionMove          = "move_to_column"
	ActionPriority      = "set_priority"
	ActionLabel         = "add_label"
	ActionAssign        = "assign_user"
	ActionComment       = "add_comment"
	ActionDue           = "set_due_in_days"
	StatusOK            = "ok"
	StatusSkipped       = "skipped"
	StatusFailed        = "failed"
	CodeNotAllowed      = "NOT_ALLOWED"
	CodeRateLimited     = "RATE_LIMITED"
	CodeInterrupted     = "INTERRUPTED"
	CodeActionFailed    = "ACTION_FAILED"
	FlagTooManyFailures = "TOO_MANY_FAILURES"
)

// Condition is one all-of test: Field equals Value (a UUID, a priority or a column category depending on the field).
type Condition struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// Action is one step of a rule; only the member that belongs to Type is set.
type Action struct {
	Type     string     `json:"type"`
	ColumnID *uuid.UUID `json:"column_id,omitempty"`
	Priority string     `json:"priority,omitempty"`
	LabelID  *uuid.UUID `json:"label_id,omitempty"`
	UserID   *uuid.UUID `json:"user_id,omitempty"`
	Text     string     `json:"text,omitempty"`
	Days     *int       `json:"days,omitempty"`
}

// Conditions is the JSONB list of conditions of a rule.
type Conditions []Condition

// Actions is the JSONB list of actions of a rule.
type Actions []Action

// Value stores the conditions as JSON; nil is an empty array.
func (c Conditions) Value() (driver.Value, error) { return jsonValue(c, c == nil) }

// Scan reads a JSONB array.
func (c *Conditions) Scan(src any) error { return jsonScan(src, c) }

// Value stores the actions as JSON; nil is an empty array.
func (a Actions) Value() (driver.Value, error) { return jsonValue(a, a == nil) }

// Scan reads a JSONB array.
func (a *Actions) Scan(src any) error { return jsonScan(src, a) }

func jsonValue(v any, empty bool) (driver.Value, error) {
	if empty {
		return "[]", nil
	}
	raw, err := json.Marshal(v)
	return string(raw), err
}

func jsonScan(src any, dst any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, dst)
	case string:
		return json.Unmarshal([]byte(v), dst)
	case nil:
		return nil
	}
	return fmt.Errorf("automation: cannot scan %T", src)
}

// Rule is one automation rule of a project.
type Rule struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID            uuid.UUID  `gorm:"type:uuid" json:"-"`
	ProjectID           uuid.UUID  `gorm:"type:uuid" json:"project_id"`
	Name                string     `json:"name"`
	Trigger             string     `json:"trigger"`
	Conditions          Conditions `gorm:"type:jsonb" json:"conditions"`
	Actions             Actions    `gorm:"type:jsonb" json:"actions"`
	Active              bool       `json:"active"`
	RunCount            int        `json:"run_count"`
	LastRunAt           *time.Time `json:"last_run_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	FlaggedCode         *string    `json:"flagged"`
	CreatedBy           uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	Version             int        `json:"version"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TableName maps to workflow_rules.
func (Rule) TableName() string { return "workflow_rules" }

// Run is the outcome of one rule for one event.
type Run struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid" json:"-"`
	RuleID      uuid.UUID  `gorm:"type:uuid" json:"rule_id"`
	EventID     uuid.UUID  `gorm:"type:uuid" json:"event_id"`
	TaskID      *uuid.UUID `gorm:"type:uuid" json:"task_id"`
	Status      string     `json:"status"`
	ErrorCode   *string    `json:"error_code"`
	ActionCount int        `json:"action_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TableName maps to workflow_rule_runs.
func (Run) TableName() string { return "workflow_rule_runs" }

// RuleInput creates a rule; an omitted Active means active.
type RuleInput struct {
	Name       string     `json:"name"`
	Trigger    string     `json:"trigger"`
	Conditions Conditions `json:"conditions"`
	Actions    Actions    `json:"actions"`
	Active     *bool      `json:"active"`
}

// RuleUpdate edits a rule; absent members stay. Version, when given, must match.
type RuleUpdate struct {
	Name       *string     `json:"name"`
	Trigger    *string     `json:"trigger"`
	Conditions *Conditions `json:"conditions"`
	Actions    *Actions    `json:"actions"`
	Active     *bool       `json:"active"`
	Version    *int        `json:"version"`
}

// TaskState is the part of a task the rules read.
type TaskState struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	ColumnID       uuid.UUID
	ColumnCategory string
	Priority       string
	LabelIDs       []uuid.UUID
	AssigneeIDs    []uuid.UUID
}
