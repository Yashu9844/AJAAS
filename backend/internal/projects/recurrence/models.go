// Package recurrence is the recurring-tasks context of M2 (specification section 9): a scheduler copies a template task on
// a daily, weekly or monthly schedule, as the person who set the recurrence up. It reaches tasks only through the Tasks
// interface that the composition root binds, and project access only through the api contracts.
package recurrence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Frequencies.
const (
	Daily   = "daily"
	Weekly  = "weekly"
	Monthly = "monthly"
)

// Flag codes stored on a recurrence the scheduler could not run.
const (
	FlagNotAllowed      = "NOT_ALLOWED"
	FlagProjectArchived = "PROJECT_ARCHIVED"
	FlagTemplateGone    = "TEMPLATE_GONE"
	FlagFailed          = "RUN_FAILED"
	FlagDatesFailed     = "SCHEDULE_FAILED"
)

const (
	maxPerProject = 20
	maxInterval   = 52
)

// Recurrence is one recurring task schedule of a project.
type Recurrence struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid" json:"-"`
	ProjectID      uuid.UUID    `gorm:"type:uuid" json:"project_id"`
	TemplateTaskID uuid.UUID    `gorm:"type:uuid" json:"template_task_id"`
	Frequency      string       `json:"frequency"`
	Interval       int          `gorm:"column:interval" json:"interval"`
	Weekday        *int         `json:"weekday"`
	DayOfMonth     *int         `json:"day_of_month"`
	NextRunAt      time.Time    `json:"next_run_at"`
	UntilOn        *kernel.Date `gorm:"type:date" json:"until_on"`
	Active         bool         `json:"active"`
	FlaggedCode    *string      `json:"flagged"`
	LastRunAt      *time.Time   `json:"last_run_at"`
	LastTaskID     *uuid.UUID   `gorm:"type:uuid" json:"last_task_id"`
	CreatedBy      uuid.UUID    `gorm:"type:uuid" json:"created_by"`
	Version        int          `json:"version"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// TableName maps to task_recurrences.
func (Recurrence) TableName() string { return "task_recurrences" }

// Input creates a recurrence. Interval defaults to 1; Weekday applies to weekly and DayOfMonth to monthly schedules and
// default to the day of the first run. FirstRunAt defaults to the first occurrence after now (00:00 UTC).
type Input struct {
	TemplateTaskID uuid.UUID    `json:"template_task_id"`
	Frequency      string       `json:"frequency"`
	Interval       int          `json:"interval"`
	Weekday        *int         `json:"weekday"`
	DayOfMonth     *int         `json:"day_of_month"`
	FirstRunAt     *time.Time   `json:"first_run_at"`
	UntilOn        *kernel.Date `json:"until_on"`
}

// OptDate is an optional date that may be absent (leave), null (clear) or a value (set).
type OptDate struct {
	Set bool
	Val *kernel.Date
}

// UnmarshalJSON records presence and keeps null as a clear.
func (o *OptDate) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		return nil
	}
	var d kernel.Date
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	o.Val = &d
	return nil
}

// Update edits a recurrence; absent members stay. Changing the schedule re-plans the next run unless NextRunAt is given.
type Update struct {
	Frequency  *string    `json:"frequency"`
	Interval   *int       `json:"interval"`
	Weekday    *int       `json:"weekday"`
	DayOfMonth *int       `json:"day_of_month"`
	NextRunAt  *time.Time `json:"next_run_at"`
	UntilOn    OptDate    `json:"until_on"`
	Active     *bool      `json:"active"`
	Version    *int       `json:"version"`
}

// Template is the part of a template task a recurrence needs to date the copies.
type Template struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	StartOn   *kernel.Date
	DueAt     *time.Time
	CreatedAt time.Time
}

// Dates are the schedule dates of a new copy; nil clears the date.
type Dates struct {
	StartOn *kernel.Date
	DueAt   *time.Time
}

// Tasks is what recurrences need from the task context. Spawn copies the template as the actor (same use case a person
// runs: permissions, WIP limits and numbering apply), titles the copy like the template and sets its dates.
type Tasks interface {
	// Template reads the template with the actor's access (404 when gone or unreadable).
	Template(ctx context.Context, a api.Actor, id uuid.UUID) (Template, error)
	// Spawn creates the copy. When the copy exists but its dates could not be set, it returns the id with the error.
	Spawn(ctx context.Context, a api.Actor, templateID uuid.UUID, d Dates) (uuid.UUID, error)
}
