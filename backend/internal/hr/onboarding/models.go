// Package onboarding is HR/Onboarding (docs/modules/m1-hr-management/contexts/onboarding): checklist templates,
// onboarding and offboarding checklists started by HR events, tasks, exit clearance and exit interviews.
package onboarding

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Kinds, sources, statuses, roles and clearance areas (specification.md §3).
const (
	KindOnboarding  = "onboarding"
	KindOffboarding = "offboarding"

	SourceCoreAction  = "core_action"
	SourceRecruitment = "recruitment"
	SourceManual      = "manual"

	TemplateActive   = "active"
	TemplateArchived = "archived"

	ChecklistOpen      = "open"
	ChecklistCompleted = "completed"
	ChecklistCancelled = "cancelled"

	TaskOpen    = "open"
	TaskDone    = "done"
	TaskSkipped = "skipped"

	RoleEmployee = "employee"
	RoleManager  = "manager"

	AreaManager = "manager"
)

var (
	kinds        = map[string]bool{KindOnboarding: true, KindOffboarding: true}
	ownerRoles   = map[string]bool{RoleEmployee: true, RoleManager: true, "hr": true, "it": true, "finance": true}
	clearanceSet = []string{AreaManager, "hr", "it", "finance"}
)

// Template is OB-01.
type Template struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID      `gorm:"type:uuid;not null" json:"-"`
	Kind            string         `gorm:"type:varchar(20);not null" json:"kind"`
	Name            string         `gorm:"type:varchar(150);not null" json:"name"`
	DepartmentID    *uuid.UUID     `gorm:"type:uuid" json:"department_id,omitempty"`
	JobProfileID    *uuid.UUID     `gorm:"type:uuid" json:"job_profile_id,omitempty"`
	LocationID      *uuid.UUID     `gorm:"type:uuid" json:"location_id,omitempty"`
	WorkerType      *string        `gorm:"type:varchar(20)" json:"worker_type,omitempty"`
	Status          string         `gorm:"type:varchar(20);not null" json:"status"`
	CreatedByUserID uuid.UUID      `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"not null" json:"updated_at"`
	Tasks           []TemplateTask `gorm:"-" json:"tasks"`
}

// TableName maps to onb_templates.
func (Template) TableName() string { return "onb_templates" }

// specificity counts the criteria the template sets (OB-02).
func (t *Template) specificity() int {
	n := 0
	for _, set := range []bool{t.DepartmentID != nil, t.JobProfileID != nil, t.LocationID != nil, t.WorkerType != nil} {
		if set {
			n++
		}
	}
	return n
}

// TemplateTask is one task of a template (OB-01).
type TemplateTask struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	TemplateID    uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Position      int       `gorm:"not null" json:"position"`
	Title         string    `gorm:"type:varchar(200);not null" json:"title"`
	Description   *string   `gorm:"type:varchar(2000)" json:"description,omitempty"`
	OwnerRole     string    `gorm:"type:varchar(20);not null" json:"owner_role"`
	DueOffsetDays int       `gorm:"not null" json:"due_offset_days"`
	DocumentType  *string   `gorm:"type:varchar(50)" json:"document_type,omitempty"`
	Blocking      bool      `gorm:"not null" json:"blocking"`
}

// TableName maps to onb_template_tasks.
func (TemplateTask) TableName() string { return "onb_template_tasks" }

// Checklist is a worker's onboarding or offboarding checklist (OB-03).
type Checklist struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID             uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID    uuid.UUID  `gorm:"type:uuid;not null" json:"employee_id"`
	Kind                 string     `gorm:"type:varchar(20);not null" json:"kind"`
	Source               string     `gorm:"type:varchar(20);not null" json:"source"`
	SourceID             *uuid.UUID `gorm:"type:uuid" json:"source_id,omitempty"`
	AnchorDate           Date       `gorm:"type:date;not null" json:"anchor_date"`
	DepartmentID         *uuid.UUID `gorm:"type:uuid" json:"department_id,omitempty"`
	ManagerEmployeeID    *uuid.UUID `gorm:"type:uuid" json:"manager_employee_id,omitempty"`
	Status               string     `gorm:"type:varchar(20);not null" json:"status"`
	ClearanceCompletedAt *time.Time `json:"clearance_completed_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	ClosedByUserID       *uuid.UUID `gorm:"type:uuid" json:"closed_by_user_id,omitempty"`
	CloseReason          *string    `gorm:"type:varchar(500)" json:"close_reason,omitempty"`
	CreatedByUserID      *uuid.UUID `gorm:"type:uuid" json:"-"`
	CreatedAt            time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to onb_checklists.
func (Checklist) TableName() string { return "onb_checklists" }

// Task is one checklist task (OB-04, OB-05).
type Task struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	ChecklistID        uuid.UUID  `gorm:"type:uuid;not null" json:"checklist_id"`
	Position           int        `gorm:"not null" json:"position"`
	Title              string     `gorm:"type:varchar(200);not null" json:"title"`
	Description        *string    `gorm:"type:varchar(2000)" json:"description,omitempty"`
	OwnerRole          string     `gorm:"type:varchar(20);not null" json:"owner_role"`
	AssigneeEmployeeID *uuid.UUID `gorm:"type:uuid" json:"assignee_employee_id,omitempty"`
	DueDate            Date       `gorm:"type:date;not null" json:"due_date"`
	DocumentType       *string    `gorm:"type:varchar(50)" json:"document_type,omitempty"`
	Blocking           bool       `gorm:"not null" json:"blocking"`
	Status             string     `gorm:"type:varchar(20);not null" json:"status"`
	EvidenceDocumentID *uuid.UUID `gorm:"type:uuid" json:"evidence_document_id,omitempty"`
	Note               *string    `gorm:"type:varchar(1000)" json:"note,omitempty"`
	DoneByUserID       *uuid.UUID `gorm:"type:uuid" json:"done_by_user_id,omitempty"`
	DoneAt             *time.Time `json:"done_at,omitempty"`
	UpdatedAt          time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to onb_tasks.
func (Task) TableName() string { return "onb_tasks" }

// Clearance is one exit sign-off (OB-07).
type Clearance struct {
	TenantID        uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	ChecklistID     uuid.UUID  `gorm:"type:uuid;primaryKey" json:"checklist_id"`
	Area            string     `gorm:"type:varchar(20);primaryKey" json:"area"`
	ClearedByUserID *uuid.UUID `gorm:"type:uuid" json:"cleared_by_user_id,omitempty"`
	ClearedAt       *time.Time `json:"cleared_at,omitempty"`
	Note            *string    `gorm:"type:varchar(1000)" json:"note,omitempty"`
}

// TableName maps to onb_clearances.
func (Clearance) TableName() string { return "onb_clearances" }

// Answers are exit-interview answers, stored as JSON.
type Answers map[string]string

// Value encodes the answers.
func (a Answers) Value() (driver.Value, error) {
	b, err := json.Marshal(a)
	return string(b), err
}

// Scan decodes the answers.
func (a *Answers) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), a)
	case []byte:
		return json.Unmarshal(v, a)
	}
	return fmt.Errorf("onboarding: cannot scan %T as answers", src)
}

// ExitInterview is OB-08.
type ExitInterview struct {
	ChecklistID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"checklist_id"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Answers           Answers   `gorm:"type:jsonb;not null" json:"answers"`
	SubmittedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	SubmittedAt       time.Time `gorm:"not null" json:"submitted_at"`
}

// TableName maps to onb_exit_interviews.
func (ExitInterview) TableName() string { return "onb_exit_interviews" }

// BeforeCreate hooks assign UUID v4 ids.
func (r *Template) BeforeCreate(*gorm.DB) error     { return assignID(&r.ID) }
func (r *TemplateTask) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }
func (r *Checklist) BeforeCreate(*gorm.DB) error    { return assignID(&r.ID) }
func (r *Task) BeforeCreate(*gorm.DB) error         { return assignID(&r.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// Date is a calendar date stored as DATE and exchanged as "YYYY-MM-DD".
type Date struct{ time.Time }

// MarshalJSON renders YYYY-MM-DD.
func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.Format(DateLayout) + `"`), nil }

// Value stores the date.
func (d Date) Value() (driver.Value, error) { return d.Format(DateLayout), nil }

// Scan reads a DATE.
func (d *Date) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		d.Time = time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
	case string:
		t, err := time.Parse(DateLayout, v[:min(len(v), 10)])
		d.Time = t
		return err
	default:
		return fmt.Errorf("onboarding: cannot scan %T as date", src)
	}
	return nil
}
