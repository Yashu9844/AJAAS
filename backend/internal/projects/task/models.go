// Package task is the task core context of M2: tasks, assignees, labels, subtasks, relations, ordering and the task
// activity reads (specification sections 2 and 3). It depends on the api package only, never on the project context.
package task

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Task is the stored row of a work item (TK-01).
type Task struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID    `gorm:"type:uuid" json:"-"`
	ProjectID       uuid.UUID    `gorm:"type:uuid" json:"project_id"`
	Number          int          `json:"number"`
	ParentTaskID    *uuid.UUID   `gorm:"type:uuid" json:"parent_task_id"`
	Depth           int          `json:"depth"`
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	ColumnID        uuid.UUID    `gorm:"type:uuid" json:"column_id"`
	Priority        string       `json:"priority"`
	ReporterUserID  uuid.UUID    `gorm:"type:uuid" json:"reporter_user_id"`
	StartOn         *kernel.Date `gorm:"type:date" json:"start_on"`
	DueAt           *time.Time   `json:"due_at"`
	EstimateMinutes *int         `json:"estimate_minutes"`
	MilestoneID     *uuid.UUID   `gorm:"type:uuid" json:"milestone_id"`
	SprintID        *uuid.UUID   `gorm:"type:uuid" json:"sprint_id"`
	Rank            string       `json:"rank"`
	CompletedAt     *time.Time   `json:"completed_at"`
	ArchivedAt      *time.Time   `json:"archived_at"`
	DeletedAt       *time.Time   `json:"-"`
	Version         int          `json:"version"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// TableName maps to tasks.
func (Task) TableName() string { return "tasks" }

// View is a task as the API returns it: the row plus ticket id, column category and derived flags (TK-06, TK-07).
type View struct {
	Task
	Key            string      `gorm:"->;-:migration" json:"key"`
	ColumnCategory string      `gorm:"->;-:migration" json:"column_category"`
	Blocked        bool        `gorm:"->;-:migration" json:"blocked"`
	Overdue        bool        `gorm:"->;-:migration" json:"overdue"`
	SubtaskTotal   int         `gorm:"->;-:migration" json:"subtask_total"`
	SubtaskDone    int         `gorm:"->;-:migration" json:"subtask_done"`
	SortKey        string      `gorm:"->;-:migration" json:"-"`
	Progress       int         `gorm:"-" json:"progress"`
	AssigneeIDs    []uuid.UUID `gorm:"-" json:"assignee_ids"` // active assignees only
	Assignees      []Assignee  `gorm:"-" json:"assignees"`    // every assignee with the active flag; inactive ones are history
	LabelIDs       []uuid.UUID `gorm:"-" json:"label_ids"`
	Warnings       []string    `gorm:"-" json:"warnings,omitempty"`
}

// Assignee is one assignment; a deactivated login stays as history with Active false.
type Assignee struct {
	UserID uuid.UUID `json:"user_id"`
	Active bool      `json:"active"`
}

// Opt carries an optional JSON field that may be absent (leave), null (clear) or a value (set).
type Opt[T any] struct {
	Set bool
	Val *T
}

// UnmarshalJSON records that the field was present and keeps null as a clear.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Val = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Val = &v
	return nil
}

// MarshalJSON writes the value, or null.
func (o Opt[T]) MarshalJSON() ([]byte, error) { return json.Marshal(o.Val) }

// CreateInput creates a task (TK-01).
type CreateInput struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	ColumnID        *uuid.UUID   `json:"column_id"`
	Priority        string       `json:"priority"`
	StartOn         *kernel.Date `json:"start_on"`
	DueAt           *time.Time   `json:"due_at"`
	EstimateMinutes *int         `json:"estimate_minutes"`
	MilestoneID     *uuid.UUID   `json:"milestone_id"`
	SprintID        *uuid.UUID   `json:"sprint_id"`
	ParentTaskID    *uuid.UUID   `json:"parent_task_id"`
	AssigneeIDs     []uuid.UUID  `json:"assignee_ids"`
	LabelIDs        []uuid.UUID  `json:"label_ids"`
}

// UpdateInput edits fields; absent fields stay, null clears the nullable ones. Version is required (TK-03).
type UpdateInput struct {
	Title           *string          `json:"title"`
	Description     *string          `json:"description"`
	Priority        *string          `json:"priority"`
	StartOn         Opt[kernel.Date] `json:"start_on"`
	DueAt           Opt[time.Time]   `json:"due_at"`
	EstimateMinutes Opt[int]         `json:"estimate_minutes"`
	MilestoneID     Opt[uuid.UUID]   `json:"milestone_id"`
	SprintID        Opt[uuid.UUID]   `json:"sprint_id"`
	ParentTaskID    Opt[uuid.UUID]   `json:"parent_task_id"`
	Version         *int             `json:"version"`
}

// MoveInput moves a task to a column and/or position (TK-04, TK-05). Before and After name a neighbour task.
type MoveInput struct {
	ColumnID *uuid.UUID `json:"column_id"`
	BeforeID *uuid.UUID `json:"before_id"`
	AfterID  *uuid.UUID `json:"after_id"`
	Version  *int       `json:"version"`
	Force    bool       `json:"force"`
}

// MoveProjectInput moves a task to another project (TK-08).
type MoveProjectInput struct {
	ProjectID uuid.UUID  `json:"project_id"`
	ColumnID  *uuid.UUID `json:"column_id"`
	Version   *int       `json:"version"`
}

// DuplicateInput copies a task (TK-11).
type DuplicateInput struct {
	CopyAssignees bool `json:"copy_assignees"`
	CopyLabels    bool `json:"copy_labels"`
}

// UserIDsInput names users for an assignee change.
type UserIDsInput struct {
	UserIDs []uuid.UUID `json:"user_ids"`
}

// LabelIDsInput names labels for a label change.
type LabelIDsInput struct {
	LabelIDs []uuid.UUID `json:"label_ids"`
}

// RelationInput relates the task to another (TK-07).
type RelationInput struct {
	TargetTaskID uuid.UUID `json:"target_task_id"`
	Type         string    `json:"type"`
}

// Relation is one stored relation.
type Relation struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid" json:"-"`
	SourceTaskID uuid.UUID `gorm:"type:uuid" json:"source_task_id"`
	TargetTaskID uuid.UUID `gorm:"type:uuid" json:"target_task_id"`
	Type         string    `json:"type"`
	CreatedBy    uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName maps to task_relations.
func (Relation) TableName() string { return "task_relations" }

// RelationView is a relation seen from one task: the other end is resolved when the caller can read it.
type RelationView struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	Direction   string    `json:"direction"` // outgoing: this task is the source; incoming: it is the target
	OtherTaskID uuid.UUID `json:"other_task_id"`
	OtherKey    string    `json:"other_key"`
	OtherTitle  string    `json:"other_title"`
	OtherDone   bool      `json:"other_done"`
	ProjectID   uuid.UUID `json:"other_project_id"`
}

// ActivityRow is one history entry as returned by the reads (CM-04).
type ActivityRow struct {
	Seq        int64           `json:"seq"`
	ID         uuid.UUID       `json:"id"`
	ProjectID  uuid.UUID       `json:"project_id"`
	TaskID     *uuid.UUID      `json:"task_id"`
	ActorID    *uuid.UUID      `gorm:"column:actor_user_id" json:"actor_user_id"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data"`
	Source     string          `json:"source"`
	OccurredAt time.Time       `json:"occurred_at"`
}
