// Package models holds HR/Core persistence models; migrations/hr 000048–000053 are the SQL twin.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// Statuses shared by the job architecture (CO-003).
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// JobFamily groups job profiles (CO-001).
type JobFamily struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_core_job_families_code,priority:1"`
	Code        string    `gorm:"type:varchar(32);not null;uniqueIndex:uq_core_job_families_code,priority:2"`
	Name        string    `gorm:"type:varchar(150);not null"`
	Description *string   `gorm:"type:varchar(1000)"`
	Status      string    `gorm:"type:varchar(20);not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// TableName maps to core_job_families.
func (JobFamily) TableName() string { return "core_job_families" }

// BeforeCreate assigns a UUID v4 when unset.
func (f *JobFamily) BeforeCreate(*gorm.DB) error { return assignID(&f.ID) }

// Grade is a pay/seniority level with a unique rank (CO-001).
type Grade struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_core_grades_code,priority:1;uniqueIndex:uq_core_grades_rank,priority:1"`
	Code      string    `gorm:"type:varchar(32);not null;uniqueIndex:uq_core_grades_code,priority:2"`
	Name      string    `gorm:"type:varchar(150);not null"`
	Rank      int       `gorm:"not null;uniqueIndex:uq_core_grades_rank,priority:2"`
	Status    string    `gorm:"type:varchar(20);not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName maps to core_grades.
func (Grade) TableName() string { return "core_grades" }

// BeforeCreate assigns a UUID v4 when unset.
func (g *Grade) BeforeCreate(*gorm.DB) error { return assignID(&g.ID) }

// JobProfile is a job (CO-001).
type JobProfile struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_core_job_profiles_code,priority:1;index:idx_core_job_profiles_family,priority:1"`
	Code           string     `gorm:"type:varchar(32);not null;uniqueIndex:uq_core_job_profiles_code,priority:2"`
	Title          string     `gorm:"type:varchar(150);not null"`
	FamilyID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_core_job_profiles_family,priority:2"`
	DefaultGradeID *uuid.UUID `gorm:"type:uuid"`
	Category       string     `gorm:"type:varchar(20);not null"`
	ProbationDays  int        `gorm:"not null"`
	Description    *string    `gorm:"type:varchar(2000)"`
	Status         string     `gorm:"type:varchar(20);not null"`
	CreatedAt      time.Time  `gorm:"not null"`
	UpdatedAt      time.Time  `gorm:"not null"`
}

// TableName maps to core_job_profiles.
func (JobProfile) TableName() string { return "core_job_profiles" }

// BeforeCreate assigns a UUID v4 when unset.
func (p *JobProfile) BeforeCreate(*gorm.DB) error { return assignID(&p.ID) }

// Position statuses (CO-013).
const (
	PositionActive = "active"
	PositionFrozen = "frozen"
	PositionClosed = "closed"
)

// Position is a budgeted seat (CO-010..CO-014).
type Position struct {
	ID           uuid.UUID    `gorm:"type:uuid;primaryKey"`
	TenantID     uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_core_positions_code,priority:1;index:idx_core_positions_department,priority:1"`
	Code         string       `gorm:"type:varchar(32);not null;uniqueIndex:uq_core_positions_code,priority:2"`
	Title        string       `gorm:"type:varchar(150);not null"`
	JobProfileID uuid.UUID    `gorm:"type:uuid;not null"`
	DepartmentID uuid.UUID    `gorm:"type:uuid;not null;index:idx_core_positions_department,priority:2"`
	LocationID   *uuid.UUID   `gorm:"type:uuid"`
	BudgetedFTE  kernel.Fixed `gorm:"column:budgeted_fte;type:numeric(5,2);not null"`
	Shared       bool         `gorm:"not null"`
	Status       string       `gorm:"type:varchar(20);not null;index:idx_core_positions_department,priority:3"`
	CreatedAt    time.Time    `gorm:"not null"`
	UpdatedAt    time.Time    `gorm:"not null"`
}

// TableName maps to core_positions.
func (Position) TableName() string { return "core_positions" }

// BeforeCreate assigns a UUID v4 when unset.
func (p *Position) BeforeCreate(*gorm.DB) error { return assignID(&p.ID) }

// Assignment statuses (CO-020, CO-039).
const (
	AssignmentActive    = "active"
	AssignmentRescinded = "rescinded"
)

// Assignment is a worker's job over [EffectiveFrom, EffectiveTo) (CO-020); EffectiveTo nil means open-ended.
type Assignment struct {
	ID                uuid.UUID    `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID    `gorm:"type:uuid;not null;index:idx_core_job_assignments_employee,priority:1"`
	EmployeeID        uuid.UUID    `gorm:"type:uuid;not null;index:idx_core_job_assignments_employee,priority:2"`
	ActionID          uuid.UUID    `gorm:"type:uuid;not null"`
	JobProfileID      *uuid.UUID   `gorm:"type:uuid"`
	PositionID        *uuid.UUID   `gorm:"type:uuid"`
	DepartmentID      *uuid.UUID   `gorm:"type:uuid"`
	LocationID        *uuid.UUID   `gorm:"type:uuid"`
	ManagerEmployeeID *uuid.UUID   `gorm:"type:uuid"`
	GradeID           *uuid.UUID   `gorm:"type:uuid"`
	WorkerType        string       `gorm:"type:varchar(20);not null"`
	EmploymentType    string       `gorm:"type:varchar(20);not null"`
	FTE               kernel.Fixed `gorm:"column:fte;type:numeric(5,2);not null"`
	ProbationEndDate  *time.Time   `gorm:"type:date"`
	EffectiveFrom     time.Time    `gorm:"type:date;not null;index:idx_core_job_assignments_employee,priority:3"`
	EffectiveTo       *time.Time   `gorm:"type:date"`
	Status            string       `gorm:"type:varchar(20);not null"`
	CreatedAt         time.Time    `gorm:"not null"`
	UpdatedAt         time.Time    `gorm:"not null"`
}

// TableName maps to core_job_assignments.
func (Assignment) TableName() string { return "core_job_assignments" }

// BeforeCreate assigns a UUID v4 when unset.
func (a *Assignment) BeforeCreate(*gorm.DB) error { return assignID(&a.ID) }

// CoversDate reports whether the assignment is in effect on d (dates at UTC midnight).
func (a *Assignment) CoversDate(d time.Time) bool {
	return a.Status == AssignmentActive && !a.EffectiveFrom.After(d) && (a.EffectiveTo == nil || a.EffectiveTo.After(d))
}

// Changes is the typed payload of an HR action (CO-033..CO-038), stored as JSON.
type Changes struct {
	JobProfileID        *uuid.UUID    `json:"job_profile_id,omitempty"`
	PositionID          *uuid.UUID    `json:"position_id,omitempty"`
	ClearPosition       bool          `json:"clear_position,omitempty"`
	DepartmentID        *uuid.UUID    `json:"department_id,omitempty"`
	LocationID          *uuid.UUID    `json:"location_id,omitempty"`
	ManagerEmployeeID   *uuid.UUID    `json:"manager_employee_id,omitempty"`
	GradeID             *uuid.UUID    `json:"grade_id,omitempty"`
	GradeOverrideReason *string       `json:"grade_override_reason,omitempty"`
	WorkerType          *string       `json:"worker_type,omitempty"`
	EmploymentType      *string       `json:"employment_type,omitempty"`
	FTE                 *kernel.Fixed `json:"fte,omitempty"`
	ProbationOutcome    *string       `json:"probation_outcome,omitempty"`
	ProbationEndDate    *string       `json:"probation_end_date,omitempty"`
	TerminationType     *string       `json:"termination_type,omitempty"`
	LastWorkingDay      *string       `json:"last_working_day,omitempty"`
	NoticeDate          *string       `json:"notice_date,omitempty"`
	EligibleForRehire   *bool         `json:"eligible_for_rehire,omitempty"`
}

// Value encodes the changes as JSON.
func (c Changes) Value() (driver.Value, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

// Scan decodes the changes from JSON.
func (c *Changes) Scan(src interface{}) error {
	switch v := src.(type) {
	case nil:
		*c = Changes{}
		return nil
	case string:
		return json.Unmarshal([]byte(v), c)
	case []byte:
		return json.Unmarshal(v, c)
	default:
		return fmt.Errorf("models: cannot scan %T as changes", src)
	}
}

// Action is one HR business process (CO-030..CO-039).
type Action struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID               uuid.UUID  `gorm:"type:uuid;not null;index:idx_core_hr_actions_employee,priority:1"`
	Type                   string     `gorm:"type:varchar(30);not null"`
	EmployeeID             uuid.UUID  `gorm:"type:uuid;not null;index:idx_core_hr_actions_employee,priority:2"`
	EffectiveDate          time.Time  `gorm:"type:date;not null"`
	Status                 string     `gorm:"type:varchar(20);not null"`
	ReasonCode             string     `gorm:"type:varchar(50);not null"`
	Comment                *string    `gorm:"type:varchar(2000)"`
	Changes                Changes    `gorm:"type:jsonb;not null"`
	InitiatedByUserID      uuid.UUID  `gorm:"type:uuid;not null"`
	DecidedByUserID        *uuid.UUID `gorm:"type:uuid"`
	DecisionComment        *string    `gorm:"type:varchar(1000)"`
	SubmittedAt            *time.Time
	CompletedAt            *time.Time
	RescindedAt            *time.Time
	RescindReason          *string    `gorm:"type:varchar(1000)"`
	PreviousAssignmentID   *uuid.UUID `gorm:"type:uuid"`
	PreviousEffectiveTo    *time.Time `gorm:"type:date"`
	NewAssignmentID        *uuid.UUID `gorm:"type:uuid"`
	PreviousEmployeeStatus *string    `gorm:"type:varchar(20)"`
	EffectsAppliedAt       *time.Time
	EffectsReversedAt      *time.Time
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
}

// TableName maps to core_hr_actions.
func (Action) TableName() string { return "core_hr_actions" }

// BeforeCreate assigns a UUID v4 when unset.
func (a *Action) BeforeCreate(*gorm.DB) error { return assignID(&a.ID) }

// HeadcountPlan is a department's planned FTE for a period (CO-050).
type HeadcountPlan struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey"`
	TenantID        uuid.UUID    `gorm:"type:uuid;not null"`
	DepartmentID    uuid.UUID    `gorm:"type:uuid;not null"`
	PeriodStart     time.Time    `gorm:"type:date;not null"`
	PeriodEnd       time.Time    `gorm:"type:date;not null"`
	PlannedFTE      kernel.Fixed `gorm:"column:planned_fte;type:numeric(7,2);not null"`
	CreatedByUserID uuid.UUID    `gorm:"type:uuid;not null"`
	CreatedAt       time.Time    `gorm:"not null"`
}

// TableName maps to core_headcount_plans.
func (HeadcountPlan) TableName() string { return "core_headcount_plans" }

// BeforeCreate assigns a UUID v4 when unset.
func (h *HeadcountPlan) BeforeCreate(*gorm.DB) error { return assignID(&h.ID) }

// OutboxEvent is a transactional-outbox row.
type OutboxEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_core_outbox_tenant"`
	EventID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_core_outbox_event_id"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null;index:idx_core_outbox_published,where:published = false"`
	PublishedAt *time.Time
	Attempts    int       `gorm:"type:integer;not null"`
	LastError   *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to core_events_outbox.
func (OutboxEvent) TableName() string { return "core_events_outbox" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *OutboxEvent) BeforeCreate(*gorm.DB) error { return assignID(&o.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}
