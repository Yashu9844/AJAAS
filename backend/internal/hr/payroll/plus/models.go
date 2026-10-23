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

// Settlement statuses (PP-07).
const (
	StatusDraft     = "draft"
	StatusApproved  = "approved"
	StatusPaid      = "paid"
	StatusCancelled = "cancelled"
)

// UUIDList is a JSON array of ids.
type UUIDList []uuid.UUID

// Value encodes the list.
func (l UUIDList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]uuid.UUID(l))
	return string(b), err
}

// Scan decodes the list.
func (l *UUIDList) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), l)
	case []byte:
		return json.Unmarshal(v, l)
	case nil:
		*l = nil
		return nil
	}
	return fmt.Errorf("payroll plus: cannot scan %T as ids", src)
}

// SettlementLine is one line of a settlement.
type SettlementLine struct {
	ID           uuid.UUID    `gorm:"type:uuid;primaryKey" json:"-"`
	SettlementID uuid.UUID    `gorm:"type:uuid;not null" json:"-"`
	Code         string       `gorm:"type:varchar(30);not null" json:"code"`
	Name         string       `gorm:"type:varchar(100);not null" json:"name"`
	Kind         string       `gorm:"type:varchar(10);not null" json:"kind"`
	Amount       kernel.Money `gorm:"type:numeric(14,2);not null" json:"amount"`
	Position     int          `gorm:"not null" json:"-"`
}

// TableName maps to payroll_settlement_lines.
func (SettlementLine) TableName() string { return "payroll_settlement_lines" }

// BeforeCreate assigns a UUID v4 when unset.
func (l *SettlementLine) BeforeCreate(*gorm.DB) error { return assignID(&l.ID) }

// Settlement is a leaver final settlement (PP-06..PP-08).
type Settlement struct {
	ID                 uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID           uuid.UUID        `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID  uuid.UUID        `gorm:"type:uuid;not null" json:"employee_id"`
	Status             string           `gorm:"type:varchar(20);not null" json:"status"`
	LastWorkingDay     Date             `gorm:"type:date;not null" json:"last_working_day"`
	ServiceYears       int              `gorm:"not null" json:"service_years"`
	MonthlyBasic       kernel.Money     `gorm:"type:numeric(14,2);not null" json:"monthly_basic"`
	Gratuity           kernel.Money     `gorm:"type:numeric(14,2);not null" json:"gratuity"`
	LeaveEncashment    kernel.Money     `gorm:"type:numeric(14,2);not null" json:"leave_encashment"`
	Encashments        kernel.Money     `gorm:"type:numeric(14,2);not null" json:"encashments"`
	NoticeRecovery     kernel.Money     `gorm:"type:numeric(14,2);not null" json:"notice_recovery"`
	RecoveryShortfall  kernel.Money     `gorm:"type:numeric(14,2);not null" json:"recovery_shortfall"`
	Net                kernel.Money     `gorm:"type:numeric(14,2);not null" json:"net"`
	EncashmentIDs      UUIDList         `gorm:"type:jsonb;not null" json:"-"`
	CalculatedByUserID uuid.UUID        `gorm:"type:uuid;not null" json:"calculated_by_user_id"`
	ApprovedByUserID   *uuid.UUID       `gorm:"type:uuid" json:"approved_by_user_id,omitempty"`
	CalculatedAt       time.Time        `gorm:"not null" json:"calculated_at"`
	ApprovedAt         *time.Time       `json:"approved_at,omitempty"`
	PaidAt             *time.Time       `json:"paid_at,omitempty"`
	CreatedAt          time.Time        `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time        `gorm:"not null" json:"updated_at"`
	Lines              []SettlementLine `gorm:"-" json:"lines"`
}

// TableName maps to payroll_settlements.
func (Settlement) TableName() string { return "payroll_settlements" }

// BeforeCreate assigns a UUID v4 when unset.
func (s *Settlement) BeforeCreate(*gorm.DB) error { return assignID(&s.ID) }

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
		return fmt.Errorf("payroll plus: cannot scan %T as date", src)
	}
	return nil
}
