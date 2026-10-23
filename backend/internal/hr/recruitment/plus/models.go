// Package plus is the HR/Recruitment parity package (docs/modules/m1-hr-management/contexts/recruitment, RP-01..RP-07):
// seat release, position-linked requisitions, competency scorecards and multi-step offer approval.
package plus

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// Requisition statuses (RP-02, RP-03).
const (
	ReqDraft     = "draft"
	ReqSubmitted = "submitted"
	ReqApproved  = "approved"
	ReqRejected  = "rejected"
	ReqCancelled = "cancelled"

	ScorecardActive   = "active"
	ScorecardArchived = "archived"

	DecisionApproved = "approved"
	DecisionRejected = "rejected"

	// StatusPendingApproval is the offer status while approvals are outstanding (RP-06); it mirrors
	// recruitment/models.OfferPendingApproval.
	StatusPendingApproval = "pending_approval"
	StatusOffered         = "offered"
	StatusWithdrawn       = "withdrawn"
)

// Requisition asks to fill seats of a core position (RP-02).
type Requisition struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	PositionID        uuid.UUID  `gorm:"type:uuid;not null" json:"position_id"`
	Title             string     `gorm:"type:varchar(150);not null" json:"title"`
	DepartmentID      uuid.UUID  `gorm:"type:uuid;not null" json:"department_id"`
	Headcount         int        `gorm:"not null" json:"headcount"`
	Justification     string     `gorm:"type:varchar(1000);not null" json:"justification"`
	Status            string     `gorm:"type:varchar(20);not null" json:"status"`
	RequestedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"requested_by_user_id"`
	DecidedByUserID   *uuid.UUID `gorm:"type:uuid" json:"decided_by_user_id,omitempty"`
	DecisionComment   *string    `gorm:"type:varchar(500)" json:"decision_comment,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	JobID             *uuid.UUID `gorm:"type:uuid" json:"job_id,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to recruitment_requisitions.
func (Requisition) TableName() string { return "recruitment_requisitions" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Requisition) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// Competency is one scorecard line; WeightPct is in hundredths of a percent (2500 = 25.00%).
type Competency struct {
	Name      string       `json:"name"`
	WeightPct kernel.Fixed `json:"weight_pct"`
}

// Competencies is a scorecard's competencies, stored as JSON.
type Competencies []Competency

// Value encodes the competencies.
func (c Competencies) Value() (driver.Value, error) {
	b, err := json.Marshal([]Competency(c))
	return string(b), err
}

// Scan decodes the competencies.
func (c *Competencies) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), c)
	case []byte:
		return json.Unmarshal(v, c)
	}
	return fmt.Errorf("recruitment plus: cannot scan %T as competencies", src)
}

// Scorecard is a competency template (RP-04).
type Scorecard struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	Name            string       `gorm:"type:varchar(150);not null" json:"name"`
	Competencies    Competencies `gorm:"type:jsonb;not null" json:"competencies"`
	Status          string       `gorm:"type:varchar(20);not null" json:"status"`
	CreatedByUserID uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time    `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time    `gorm:"not null" json:"updated_at"`
}

// TableName maps to recruitment_scorecards.
func (Scorecard) TableName() string { return "recruitment_scorecards" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Scorecard) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// JobScorecard links a job to its scorecard (RP-04).
type JobScorecard struct {
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	JobID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"job_id"`
	ScorecardID uuid.UUID `gorm:"type:uuid;not null" json:"scorecard_id"`
}

// TableName maps to recruitment_job_scorecards.
func (JobScorecard) TableName() string { return "recruitment_job_scorecards" }

// InterviewScore is one interviewer score for one competency (RP-05).
type InterviewScore struct {
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	InterviewID uuid.UUID `gorm:"type:uuid;primaryKey" json:"interview_id"`
	Competency  string    `gorm:"type:varchar(100);primaryKey" json:"competency"`
	Score       int       `gorm:"not null" json:"score"`
	Note        *string   `gorm:"type:varchar(500)" json:"note,omitempty"`
	CreatedAt   time.Time `gorm:"not null" json:"-"`
}

// TableName maps to recruitment_interview_scores.
func (InterviewScore) TableName() string { return "recruitment_interview_scores" }

// OfferPolicy is the tenant offer-approval policy (RP-06).
type OfferPolicy struct {
	TenantID          uuid.UUID    `gorm:"type:uuid;primaryKey" json:"-"`
	RequiredApprovals int          `gorm:"not null" json:"required_approvals"`
	ThresholdCTC      kernel.Money `gorm:"column:threshold_ctc;type:numeric(14,2);not null" json:"threshold_ctc"`
	UpdatedByUserID   uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt         time.Time    `gorm:"not null" json:"updated_at"`
}

// TableName maps to recruitment_offer_policies.
func (OfferPolicy) TableName() string { return "recruitment_offer_policies" }

// OfferApproval is one approver decision on an offer (RP-07).
type OfferApproval struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID       uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	OfferID        uuid.UUID `gorm:"type:uuid;not null" json:"offer_id"`
	ApproverUserID uuid.UUID `gorm:"type:uuid;not null" json:"approver_user_id"`
	Decision       string    `gorm:"type:varchar(10);not null" json:"decision"`
	Comment        *string   `gorm:"type:varchar(500)" json:"comment,omitempty"`
	DecidedAt      time.Time `gorm:"not null" json:"decided_at"`
}

// TableName maps to recruitment_offer_approvals.
func (OfferApproval) TableName() string { return "recruitment_offer_approvals" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *OfferApproval) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}
