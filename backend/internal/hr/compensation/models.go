// Package compensation is HR/Compensation (docs/modules/m1-hr-management/contexts/compensation): pay ranges,
// effective-dated compensation records, off-cycle changes with maker-checker, and merit cycles.
package compensation

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// Statuses (CO-C13, CO-C25).
const (
	ChangePending   = "pending"
	ChangeApproved  = "approved"
	ChangeRejected  = "rejected"
	ChangeCancelled = "cancelled"

	CycleDraft    = "draft"
	CycleOpen     = "open"
	CycleInReview = "in_review"
	CycleApproved = "approved"
	CycleApplied  = "applied"

	ReasonMerit = "merit"
)

// PayRange is CO-C01.
type PayRange struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	GradeID         uuid.UUID    `gorm:"type:uuid;not null" json:"grade_id"`
	MinAmount       kernel.Money `gorm:"column:min_amount;type:numeric(14,2);not null" json:"min_amount"`
	MidAmount       kernel.Money `gorm:"column:mid_amount;type:numeric(14,2);not null" json:"mid_amount"`
	MaxAmount       kernel.Money `gorm:"column:max_amount;type:numeric(14,2);not null" json:"max_amount"`
	EffectiveFrom   Date         `gorm:"type:date;not null" json:"effective_from"`
	EffectiveTo     *Date        `gorm:"type:date" json:"effective_to,omitempty"`
	CreatedByUserID uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time    `gorm:"not null" json:"created_at"`
}

// TableName maps to comp_pay_ranges.
func (PayRange) TableName() string { return "comp_pay_ranges" }

// Record is a worker's annual base over [EffectiveFrom, EffectiveTo) (CO-C11).
type Record struct {
	ID                uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID    `gorm:"type:uuid;not null" json:"employee_id"`
	AnnualBase        kernel.Money `gorm:"type:numeric(14,2);not null" json:"annual_base"`
	EffectiveFrom     Date         `gorm:"type:date;not null" json:"effective_from"`
	EffectiveTo       *Date        `gorm:"type:date" json:"effective_to,omitempty"`
	Reason            string       `gorm:"type:varchar(20);not null" json:"reason"`
	ChangeID          *uuid.UUID   `gorm:"type:uuid" json:"change_id,omitempty"`
	MeritCycleID      *uuid.UUID   `gorm:"type:uuid" json:"merit_cycle_id,omitempty"`
	PayrollSynced     bool         `gorm:"not null" json:"payroll_synced"`
	CreatedByUserID   uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	CreatedAt         time.Time    `gorm:"not null" json:"created_at"`
}

// TableName maps to comp_records.
func (Record) TableName() string { return "comp_records" }

// Covers reports whether the record is in effect on d.
func (r *Record) Covers(d time.Time) bool {
	return !r.EffectiveFrom.After(d) && (r.EffectiveTo == nil || r.EffectiveTo.After(d))
}

// Change is an off-cycle change request (CO-C10, CO-C13).
type Change struct {
	ID                uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID    `gorm:"type:uuid;not null" json:"employee_id"`
	AnnualBase        kernel.Money `gorm:"type:numeric(14,2);not null" json:"annual_base"`
	EffectiveFrom     Date         `gorm:"type:date;not null" json:"effective_from"`
	Reason            string       `gorm:"type:varchar(20);not null" json:"reason"`
	Comment           *string      `gorm:"type:varchar(1000)" json:"comment,omitempty"`
	Status            string       `gorm:"type:varchar(20);not null" json:"status"`
	RequestedByUserID uuid.UUID    `gorm:"type:uuid;not null" json:"requested_by_user_id"`
	DecidedByUserID   *uuid.UUID   `gorm:"type:uuid" json:"decided_by_user_id,omitempty"`
	DecisionComment   *string      `gorm:"type:varchar(1000)" json:"decision_comment,omitempty"`
	DecidedAt         *time.Time   `json:"decided_at,omitempty"`
	CreatedAt         time.Time    `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time    `gorm:"not null" json:"updated_at"`
}

// TableName maps to comp_changes.
func (Change) TableName() string { return "comp_changes" }

// Guideline is one cell of the merit matrix (CO-C20).
type Guideline struct {
	Rating string       `json:"rating"`
	Compa  string       `json:"compa"`
	MinPct kernel.Fixed `json:"min_pct"`
	MaxPct kernel.Fixed `json:"max_pct"`
}

// Guidelines is the merit matrix, stored as JSON.
type Guidelines []Guideline

// Value encodes the matrix.
func (g Guidelines) Value() (driver.Value, error) {
	if g == nil {
		return "[]", nil
	}
	b, err := json.Marshal(g)
	return string(b), err
}

// Scan decodes the matrix.
func (g *Guidelines) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), g)
	case []byte:
		return json.Unmarshal(v, g)
	case nil:
		*g = nil
		return nil
	}
	return fmt.Errorf("compensation: cannot scan %T as guidelines", src)
}

// Cycle is a merit cycle (CO-C20..CO-C25).
type Cycle struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	Name               string     `gorm:"type:varchar(150);not null" json:"name"`
	EffectiveDate      Date       `gorm:"type:date;not null" json:"effective_date"`
	PerformanceCycleID *uuid.UUID `gorm:"type:uuid" json:"performance_cycle_id,omitempty"`
	Guidelines         Guidelines `gorm:"type:jsonb;not null" json:"guidelines"`
	Status             string     `gorm:"type:varchar(20);not null" json:"status"`
	CreatedByUserID    uuid.UUID  `gorm:"type:uuid;not null" json:"created_by_user_id"`
	OpenedByUserID     *uuid.UUID `gorm:"type:uuid" json:"opened_by_user_id,omitempty"`
	ApprovedByUserID   *uuid.UUID `gorm:"type:uuid" json:"approved_by_user_id,omitempty"`
	OpenedAt           *time.Time `json:"opened_at,omitempty"`
	ApprovedAt         *time.Time `json:"approved_at,omitempty"`
	AppliedAt          *time.Time `json:"applied_at,omitempty"`
	CreatedAt          time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to comp_merit_cycles.
func (Cycle) TableName() string { return "comp_merit_cycles" }

// Budget is a department's merit budget (CO-C21).
type Budget struct {
	TenantID     uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	CycleID      uuid.UUID    `gorm:"type:uuid;primaryKey" json:"cycle_id"`
	DepartmentID uuid.UUID    `gorm:"type:uuid;primaryKey" json:"department_id"`
	Amount       kernel.Money `gorm:"type:numeric(14,2);not null" json:"amount"`
}

// TableName maps to comp_merit_budgets.
func (Budget) TableName() string { return "comp_merit_budgets" }

// Proposal is one worker's merit proposal with its snapshots (CO-C22..CO-C24).
type Proposal struct {
	ID                uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID     `gorm:"type:uuid;not null" json:"-"`
	CycleID           uuid.UUID     `gorm:"type:uuid;not null" json:"cycle_id"`
	EmployeeProfileID uuid.UUID     `gorm:"type:uuid;not null" json:"employee_id"`
	DepartmentID      uuid.UUID     `gorm:"type:uuid;not null" json:"department_id"`
	ManagerEmployeeID *uuid.UUID    `gorm:"type:uuid" json:"manager_employee_id,omitempty"`
	CurrentBase       kernel.Money  `gorm:"type:numeric(14,2);not null" json:"current_base"`
	CompaRatio        *kernel.Fixed `gorm:"type:numeric(6,2)" json:"compa_ratio,omitempty"`
	CompaBand         string        `gorm:"type:varchar(10);not null" json:"compa_band"`
	RatingBand        string        `gorm:"type:varchar(10);not null" json:"rating_band"`
	IncreasePct       kernel.Fixed  `gorm:"type:numeric(5,2);not null" json:"increase_pct"`
	Amount            kernel.Money  `gorm:"type:numeric(14,2);not null" json:"amount"`
	Justification     *string       `gorm:"type:varchar(1000)" json:"justification,omitempty"`
	OutsideGuideline  bool          `gorm:"not null" json:"outside_guideline"`
	OverrideBudget    bool          `gorm:"not null" json:"override_budget"`
	UpdatedByUserID   *uuid.UUID    `gorm:"type:uuid" json:"updated_by_user_id,omitempty"`
	CreatedAt         time.Time     `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time     `gorm:"not null" json:"updated_at"`
}

// TableName maps to comp_merit_proposals.
func (Proposal) TableName() string { return "comp_merit_proposals" }

// BeforeCreate hooks assign UUID v4 ids.
func (r *PayRange) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }
func (r *Record) BeforeCreate(*gorm.DB) error   { return assignID(&r.ID) }
func (r *Change) BeforeCreate(*gorm.DB) error   { return assignID(&r.ID) }
func (r *Cycle) BeforeCreate(*gorm.DB) error    { return assignID(&r.ID) }
func (r *Proposal) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

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
		return fmt.Errorf("compensation: cannot scan %T as date", src)
	}
	return nil
}
