// Package records is HR/Employee's structured worker records (emergency contacts, education, prior employment) and
// approval-gated personal-data change requests (specification.md FR-ER, FR-CR; migrations hr/000054–000057).
package records

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmergencyContact is FR-ER001.
type EmergencyContact struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null" json:"employee_id"`
	Name              string    `gorm:"type:varchar(150);not null" json:"name"`
	Relationship      string    `gorm:"type:varchar(50);not null" json:"relationship"`
	Phone             string    `gorm:"type:varchar(30);not null" json:"phone"`
	Email             *string   `gorm:"type:varchar(255)" json:"email,omitempty"`
	IsPrimary         bool      `gorm:"not null" json:"is_primary"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to employee_emergency_contacts.
func (EmergencyContact) TableName() string { return "employee_emergency_contacts" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *EmergencyContact) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// Education is FR-ER002.
type Education struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null" json:"employee_id"`
	Institution       string    `gorm:"type:varchar(200);not null" json:"institution"`
	Qualification     string    `gorm:"type:varchar(150);not null" json:"qualification"`
	FieldOfStudy      *string   `gorm:"type:varchar(150)" json:"field_of_study,omitempty"`
	StartYear         *int      `json:"start_year,omitempty"`
	EndYear           *int      `json:"end_year,omitempty"`
	Grade             *string   `gorm:"type:varchar(50)" json:"grade,omitempty"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to employee_education.
func (Education) TableName() string { return "employee_education" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Education) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// PriorEmployment is FR-ER003.
type PriorEmployment struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null" json:"employee_id"`
	Employer          string    `gorm:"type:varchar(200);not null" json:"employer"`
	Title             string    `gorm:"type:varchar(150);not null" json:"title"`
	StartDate         Date      `gorm:"type:date;not null" json:"start_date"`
	EndDate           Date      `gorm:"type:date;not null" json:"end_date"`
	ReasonForLeaving  *string   `gorm:"type:varchar(500)" json:"reason_for_leaving,omitempty"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to employee_prior_employment.
func (PriorEmployment) TableName() string { return "employee_prior_employment" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *PriorEmployment) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// Change-request types and statuses (FR-CR001, HR-013).
const (
	TypeName    = "name"
	TypeAddress = "address"
	TypeBank    = "bank"

	StatusSubmitted = "submitted"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// Payload is the requested change (fields per type, FR-CR001).
type Payload struct {
	FirstName         *string `json:"first_name,omitempty"`
	LastName          *string `json:"last_name,omitempty"`
	DisplayName       *string `json:"display_name,omitempty"`
	CurrentAddress    *string `json:"current_address,omitempty"`
	PermanentAddress  *string `json:"permanent_address,omitempty"`
	BankName          *string `json:"bank_name,omitempty"`
	BankAccountNumber *string `json:"bank_account_number,omitempty"`
	BankRoutingSwift  *string `json:"bank_routing_swift,omitempty"`
}

// Value encodes the payload as JSON.
func (p Payload) Value() (driver.Value, error) {
	b, err := json.Marshal(p)
	return string(b), err
}

// Scan decodes the payload from JSON.
func (p *Payload) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), p)
	case []byte:
		return json.Unmarshal(v, p)
	case nil:
		*p = Payload{}
		return nil
	}
	return fmt.Errorf("records: cannot scan %T as payload", src)
}

// ChangeRequest is FR-CR001..FR-CR005.
type ChangeRequest struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null" json:"employee_id"`
	Type              string     `gorm:"type:varchar(20);not null" json:"type"`
	Payload           Payload    `gorm:"type:jsonb;not null" json:"payload"`
	Reason            *string    `gorm:"type:varchar(500)" json:"reason,omitempty"`
	Status            string     `gorm:"type:varchar(20);not null" json:"status"`
	RequestedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"requested_by_user_id"`
	DecidedByUserID   *uuid.UUID `gorm:"type:uuid" json:"decided_by_user_id,omitempty"`
	DecisionComment   *string    `gorm:"type:varchar(1000)" json:"decision_comment,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to employee_change_requests.
func (ChangeRequest) TableName() string { return "employee_change_requests" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *ChangeRequest) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// Date is a calendar date stored as DATE and exchanged as "YYYY-MM-DD".
type Date struct{ time.Time }

// MarshalJSON renders YYYY-MM-DD.
func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.Format(DateLayout) + `"`), nil }

// UnmarshalJSON parses YYYY-MM-DD.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return fmt.Errorf("date must be YYYY-MM-DD")
	}
	d.Time = t
	return nil
}

// Value stores the date.
func (d Date) Value() (driver.Value, error) { return d.Format(DateLayout), nil }

// Scan reads a DATE.
func (d *Date) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		d.Time = v.UTC()
	case string:
		t, err := time.Parse(DateLayout, v[:min(len(v), 10)])
		d.Time = t
		return err
	default:
		return fmt.Errorf("records: cannot scan %T as date", src)
	}
	return nil
}

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}
