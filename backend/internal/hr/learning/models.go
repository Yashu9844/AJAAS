// Package learning is HR/Learning (docs/modules/m1-hr-management/contexts/learning): a course catalogue with versions,
// ordered plans, rule and individual assignments with recurrence, enrolments, immutable completions with optional
// manager or HR verification, and a compliance report. Content is never hosted.
package learning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Statuses and kinds (LN-C01..LN-C13).
const (
	KindLink     = "link"
	KindDocument = "document"

	VerifySelf    = "self"
	VerifyManager = "manager"
	VerifyHR      = "hr"

	CourseDraft     = "draft"
	CoursePublished = "published"
	CourseRetired   = "retired"

	PlanActive   = "active"
	PlanArchived = "archived"

	TargetEmployee   = "employee"
	TargetDepartment = "department"
	TargetProfile    = "job_profile"
	TargetLocation   = "location"
	TargetAll        = "all"

	AssignActive    = "active"
	AssignCancelled = "cancelled"

	SourceAssigned = "assigned"
	SourceSelf     = "self"

	EnrolNotStarted = "not_started"
	EnrolInProgress = "in_progress"
	EnrolCompleted  = "completed"
	EnrolWaived     = "waived"
	// FilterOverdue is a status filter only: overdue is computed, never stored (LN-C09).
	FilterOverdue = "overdue"

	CompPending    = "pending"
	CompVerified   = "verified"
	CompRejected   = "rejected"
	CompSuperseded = "superseded"
)

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
		return fmt.Errorf("learning: cannot scan %T as date", src)
	}
	return nil
}

// Tags is a course's tag list stored as JSON.
type Tags []string

// Value encodes the tags.
func (t Tags) Value() (driver.Value, error) {
	if t == nil {
		t = Tags{}
	}
	b, err := json.Marshal([]string(t))
	return string(b), err
}

// Scan decodes the tags.
func (t *Tags) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), t)
	case []byte:
		return json.Unmarshal(v, t)
	}
	return fmt.Errorf("learning: cannot scan %T as tags", src)
}

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}

// Settings are the tenant's learning settings (LN-C09).
type Settings struct {
	TenantID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	TimeZone        string    `gorm:"type:varchar(64);not null" json:"time_zone"`
	UpdatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to lrn_settings.
func (Settings) TableName() string { return "lrn_settings" }

// Course is a catalogue entry (LN-C01).
type Course struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Title           string    `gorm:"type:varchar(150);not null" json:"title"`
	Description     *string   `gorm:"type:varchar(2000)" json:"description,omitempty"`
	Provider        *string   `gorm:"type:varchar(100)" json:"provider,omitempty"`
	ContentKind     string    `gorm:"type:varchar(10);not null" json:"content_kind"`
	ContentURL      *string   `gorm:"type:varchar(500)" json:"content_url,omitempty"`
	DocumentRef     *string   `gorm:"type:varchar(200)" json:"document_ref,omitempty"`
	ThumbnailRef    *string   `gorm:"type:varchar(500)" json:"thumbnail_ref,omitempty"`
	DurationMins    int       `gorm:"not null" json:"duration_mins"`
	Tags            Tags      `gorm:"type:jsonb;not null" json:"tags"`
	Verification    string    `gorm:"type:varchar(10);not null" json:"verification"`
	SelfEnrol       bool      `gorm:"not null" json:"self_enrol"`
	Status          string    `gorm:"type:varchar(10);not null" json:"status"`
	Version         int       `gorm:"not null" json:"version"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to lrn_courses.
func (Course) TableName() string { return "lrn_courses" }

// BeforeCreate assigns a UUID v4 when unset.
func (c *Course) BeforeCreate(*gorm.DB) error { return assignID(&c.ID) }

// CourseVersion is a snapshot of a course's content at a version (LN-C02).
type CourseVersion struct {
	TenantID        uuid.UUID       `gorm:"type:uuid;not null" json:"-"`
	CourseID        uuid.UUID       `gorm:"type:uuid;primaryKey" json:"course_id"`
	Version         int             `gorm:"primaryKey" json:"version"`
	Snapshot        json.RawMessage `gorm:"type:jsonb;not null" json:"snapshot"`
	ChangedByUserID uuid.UUID       `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time       `gorm:"not null" json:"created_at"`
}

// TableName maps to lrn_course_versions.
func (CourseVersion) TableName() string { return "lrn_course_versions" }

// Plan is an ordered set of courses (LN-C04).
type Plan struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	Name            string     `gorm:"type:varchar(150);not null" json:"name"`
	Description     *string    `gorm:"type:varchar(1000)" json:"description,omitempty"`
	Status          string     `gorm:"type:varchar(10);not null" json:"status"`
	CreatedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"not null" json:"updated_at"`
	Items           []PlanItem `gorm:"-" json:"items,omitempty"`
}

// TableName maps to lrn_plans.
func (Plan) TableName() string { return "lrn_plans" }

// BeforeCreate assigns a UUID v4 when unset.
func (p *Plan) BeforeCreate(*gorm.DB) error { return assignID(&p.ID) }

// PlanItem is one course of a plan.
type PlanItem struct {
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	PlanID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	CourseID  uuid.UUID `gorm:"type:uuid;primaryKey" json:"course_id"`
	Position  int       `gorm:"not null" json:"position"`
	Mandatory bool      `gorm:"not null" json:"mandatory"`
}

// TableName maps to lrn_plan_items.
func (PlanItem) TableName() string { return "lrn_plan_items" }

// Assignment gives a course or plan to a worker or a rule's matches (LN-C05).
type Assignment struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	TargetKind      string     `gorm:"type:varchar(12);not null" json:"target_kind"`
	TargetID        *uuid.UUID `gorm:"type:uuid" json:"target_id,omitempty"`
	CourseID        *uuid.UUID `gorm:"type:uuid" json:"course_id,omitempty"`
	PlanID          *uuid.UUID `gorm:"type:uuid" json:"plan_id,omitempty"`
	Mandatory       bool       `gorm:"not null" json:"mandatory"`
	DueDate         *Date      `gorm:"type:date" json:"due_date,omitempty"`
	DueInDays       *int       `json:"due_in_days,omitempty"`
	RecurrenceDays  *int       `json:"recurrence_days,omitempty"`
	Status          string     `gorm:"type:varchar(10);not null" json:"status"`
	Cycle           int        `gorm:"not null" json:"cycle"`
	NextCycleOn     *Date      `gorm:"type:date" json:"next_cycle_on,omitempty"`
	CreatedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to lrn_assignments.
func (Assignment) TableName() string { return "lrn_assignments" }

// BeforeCreate assigns a UUID v4 when unset.
func (a *Assignment) BeforeCreate(*gorm.DB) error { return assignID(&a.ID) }

// Enrolment is one worker's place in one course (LN-C06..LN-C13).
type Enrolment struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null" json:"employee_id"`
	CourseID          uuid.UUID  `gorm:"type:uuid;not null" json:"course_id"`
	PlanID            *uuid.UUID `gorm:"type:uuid" json:"plan_id,omitempty"`
	AssignmentID      *uuid.UUID `gorm:"type:uuid" json:"assignment_id,omitempty"`
	Source            string     `gorm:"type:varchar(10);not null" json:"source"`
	Mandatory         bool       `gorm:"not null" json:"mandatory"`
	Cycle             int        `gorm:"not null" json:"cycle"`
	Status            string     `gorm:"type:varchar(12);not null" json:"status"`
	DueDate           *Date      `gorm:"type:date" json:"due_date,omitempty"`
	DepartmentID      *uuid.UUID `gorm:"type:uuid" json:"department_id,omitempty"`
	DedupeKey         string     `gorm:"type:varchar(120);not null" json:"-"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	WaiveReason       *string    `gorm:"type:varchar(500)" json:"waive_reason,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
	// Overdue is computed from the due date in the tenant's time zone (LN-C09); it is never stored.
	Overdue bool `gorm:"-" json:"overdue"`
}

// TableName maps to lrn_enrolments.
func (Enrolment) TableName() string { return "lrn_enrolments" }

// BeforeCreate assigns a UUID v4 when unset.
func (e *Enrolment) BeforeCreate(*gorm.DB) error { return assignID(&e.ID) }

// Live reports whether the enrolment can still be worked on.
func (e *Enrolment) Live() bool { return e.Status == EnrolNotStarted || e.Status == EnrolInProgress }

// Completion is an immutable attestation of completing an enrolment (LN-C10..LN-C12).
type Completion struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EnrolmentID       uuid.UUID  `gorm:"type:uuid;not null" json:"enrolment_id"`
	Attestation       string     `gorm:"type:varchar(10);not null" json:"attestation"`
	Status            string     `gorm:"type:varchar(12);not null" json:"status"`
	Score             *int       `json:"score,omitempty"`
	EvidenceRef       *string    `gorm:"type:varchar(300)" json:"evidence_ref,omitempty"`
	CertificateRef    *string    `gorm:"type:varchar(300)" json:"certificate_ref,omitempty"`
	CompletedOn       Date       `gorm:"type:date;not null" json:"completed_on"`
	SubmittedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	DecidedByUserID   *uuid.UUID `gorm:"type:uuid" json:"-"`
	DecisionComment   *string    `gorm:"type:varchar(500)" json:"decision_comment,omitempty"`
	SupersedesID      *uuid.UUID `gorm:"type:uuid" json:"supersedes_id,omitempty"`
	CorrectionReason  *string    `gorm:"type:varchar(500)" json:"correction_reason,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
}

// TableName maps to lrn_completions.
func (Completion) TableName() string { return "lrn_completions" }

// BeforeCreate assigns a UUID v4 when unset.
func (c *Completion) BeforeCreate(*gorm.DB) error { return assignID(&c.ID) }
