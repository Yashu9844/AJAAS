// Package models holds Module 6 (Recruitment) persistence models; migrations 000045–000050 are the SQL twin.
package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
	"gorm.io/gorm"
)

// Interview and offer statuses (FR-IV, FR-OF).
const (
	InterviewScheduled = "scheduled"
	InterviewCompleted = "completed"
	InterviewCancelled = "cancelled"

	OfferOffered   = "offered"
	OfferAccepted  = "accepted"
	OfferDeclined  = "declined"
	OfferWithdrawn = "withdrawn"
)

// Job is a job opening (FR-JB001).
type Job struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null;index:idx_recruitment_jobs_tenant_status,priority:1"`
	Title              string     `gorm:"type:varchar(150);not null"`
	DepartmentID       *uuid.UUID `gorm:"type:uuid"`
	DesignationID      *uuid.UUID `gorm:"type:uuid"`
	Headcount          int        `gorm:"type:integer;not null"`
	HiredCount         int        `gorm:"type:integer;not null"`
	Location           *string    `gorm:"type:varchar(100)"`
	EmploymentType     string     `gorm:"type:varchar(20);not null"`
	MinExperienceYears int        `gorm:"type:integer;not null"`
	Description        string     `gorm:"type:text;not null"`
	Status             string     `gorm:"type:varchar(20);not null;index:idx_recruitment_jobs_tenant_status,priority:2"`
	OpenedAt           *time.Time
	ClosedAt           *time.Time
	CreatedByUserID    uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

// TableName maps to recruitment_jobs (migration 000045).
func (Job) TableName() string { return "recruitment_jobs" }

// BeforeCreate assigns a UUID v4 when unset.
func (j *Job) BeforeCreate(tx *gorm.DB) error { return assignID(&j.ID) }

// Candidate is one applicant for one job (FR-CD001).
type Candidate struct {
	ID               uuid.UUID   `gorm:"type:uuid;primaryKey"`
	TenantID         uuid.UUID   `gorm:"type:uuid;not null;index:idx_recruitment_candidates_job_stage,priority:1"`
	JobID            uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_recruitment_candidates_job_email,priority:1;index:idx_recruitment_candidates_job_stage,priority:2"`
	FirstName        string      `gorm:"type:varchar(100);not null"`
	LastName         string      `gorm:"type:varchar(100);not null"`
	Email            string      `gorm:"type:varchar(255);not null;uniqueIndex:uq_recruitment_candidates_job_email,priority:2,expression:lower(email)"`
	Phone            *string     `gorm:"type:varchar(30)"`
	Source           string      `gorm:"type:varchar(20);not null"`
	ResumeURL        *string     `gorm:"type:varchar(500)"`
	ExpectedCTC      *calc.Money `gorm:"column:expected_ctc;type:numeric(14,2)"`
	NoticePeriodDays *int        `gorm:"type:integer"`
	Stage            string      `gorm:"type:varchar(20);not null;index:idx_recruitment_candidates_job_stage,priority:3"`
	HiredUserID      *uuid.UUID  `gorm:"type:uuid"`
	HiredEmployeeID  *uuid.UUID  `gorm:"type:uuid"`
	CreatedAt        time.Time   `gorm:"not null"`
	UpdatedAt        time.Time   `gorm:"not null"`
}

// TableName maps to recruitment_candidates (migration 000046).
func (Candidate) TableName() string { return "recruitment_candidates" }

// BeforeCreate assigns a UUID v4 when unset.
func (c *Candidate) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }

// StageEvent is one append-only pipeline change (RC-003).
type StageEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null"`
	CandidateID uuid.UUID `gorm:"type:uuid;not null;index:idx_recruitment_stage_events_candidate"`
	FromStage   *string   `gorm:"type:varchar(20)"`
	ToStage     string    `gorm:"type:varchar(20);not null"`
	ActorUserID uuid.UUID `gorm:"type:uuid;not null"`
	Note        *string   `gorm:"type:varchar(500)"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to recruitment_stage_events (migration 000047).
func (StageEvent) TableName() string { return "recruitment_stage_events" }

// BeforeCreate assigns a UUID v4 when unset.
func (e *StageEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&e.ID) }

// Interview is one scheduled round with optional feedback (FR-IV).
type Interview struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID              uuid.UUID `gorm:"type:uuid;not null;index:idx_recruitment_interviews_interviewer,priority:1"`
	CandidateID           uuid.UUID `gorm:"type:uuid;not null;index:idx_recruitment_interviews_candidate"`
	RoundName             string    `gorm:"type:varchar(100);not null"`
	InterviewerEmployeeID uuid.UUID `gorm:"type:uuid;not null;index:idx_recruitment_interviews_interviewer,priority:2"`
	ScheduledAt           time.Time `gorm:"not null"`
	DurationMins          int       `gorm:"type:integer;not null"`
	MeetingLink           *string   `gorm:"type:varchar(500)"`
	Status                string    `gorm:"type:varchar(20);not null"`
	Rating                *int      `gorm:"type:integer"`
	Recommendation        *string   `gorm:"type:varchar(20)"`
	Feedback              *string   `gorm:"type:text"`
	FeedbackAt            *time.Time
	CreatedAt             time.Time `gorm:"not null"`
	UpdatedAt             time.Time `gorm:"not null"`
}

// TableName maps to recruitment_interviews (migration 000048).
func (Interview) TableName() string { return "recruitment_interviews" }

// BeforeCreate assigns a UUID v4 when unset.
func (i *Interview) BeforeCreate(tx *gorm.DB) error { return assignID(&i.ID) }

// Offer is an offer to a candidate (FR-OF); at most one `offered` per candidate (RC-006).
type Offer struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null"`
	CandidateID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_recruitment_offers_open,where:status = 'offered';index:idx_recruitment_offers_candidate"`
	OfferedCTC      calc.Money `gorm:"column:offered_ctc;type:numeric(14,2);not null"`
	JoiningDate     time.Time  `gorm:"type:date;not null"`
	ExpiresOn       *time.Time `gorm:"type:date"`
	Status          string     `gorm:"type:varchar(20);not null"`
	DecidedAt       *time.Time
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

// TableName maps to recruitment_offers (migration 000049).
func (Offer) TableName() string { return "recruitment_offers" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *Offer) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }

// OutboxEvent is a transactional-outbox row (FR-EV001).
type OutboxEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_recruitment_outbox_tenant"`
	EventID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_recruitment_outbox_event_id"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null;index:idx_recruitment_outbox_published,where:published = false"`
	PublishedAt *time.Time
	Attempts    int       `gorm:"type:integer;not null"`
	LastError   *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to recruitment_events_outbox (migration 000050).
func (OutboxEvent) TableName() string { return "recruitment_events_outbox" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}
