// Package plus is the HR/Leave parity package (docs/modules/m1-hr-management/contexts/leave, LP-01..LP-18): leave
// year and policy, per-location calendars, manager routing and team calendar, comp-off and encashment. It adjusts
// balances only through leave/services.BalanceMover.
package plus

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	"gorm.io/gorm"
)

// Statuses shared by comp-off and encashment requests.
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"

	defaultYearStart  = 1
	defaultExpiryDays = 90
	defaultWeekOff    = 1<<0 | 1<<6 // Sunday and Saturday
)

// Policy is the tenant leave policy (LP-01).
type Policy struct {
	TenantID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"-"`
	YearStartMonth     int        `gorm:"not null" json:"year_start_month"`
	CompOffLeaveTypeID *uuid.UUID `gorm:"type:uuid" json:"comp_off_leave_type_id,omitempty"`
	CompOffExpiryDays  int        `gorm:"not null" json:"comp_off_expiry_days"`
	UpdatedByUserID    uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt          time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to leave_policies.
func (Policy) TableName() string { return "leave_policies" }

func defaults(tenantID uuid.UUID) *Policy {
	return &Policy{TenantID: tenantID, YearStartMonth: defaultYearStart, CompOffExpiryDays: defaultExpiryDays}
}

// LocationHoliday is a holiday of one organization location (LP-03).
type LocationHoliday struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	LocationID      uuid.UUID `gorm:"type:uuid;not null" json:"location_id"`
	HolidayDate     Date      `gorm:"type:date;not null" json:"date"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`
	IsOptional      bool      `gorm:"not null" json:"is_optional"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
}

// TableName maps to leave_location_holidays.
func (LocationHoliday) TableName() string { return "leave_location_holidays" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *LocationHoliday) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// WeekOff is a location's weekly-off pattern (LP-04).
type WeekOff struct {
	TenantID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	LocationID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"location_id"`
	WeekOffMask     int       `gorm:"column:week_off_mask;not null" json:"-"`
	UpdatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to leave_week_offs.
func (WeekOff) TableName() string { return "leave_week_offs" }

func (w WeekOff) days() []int { return maskDays(w.WeekOffMask) }

func maskDays(mask int) []int {
	out := []int{}
	for d := 0; d < 7; d++ {
		if mask&(1<<uint(d)) != 0 {
			out = append(out, d)
		}
	}
	return out
}

func maskOf(mask int) func(time.Time) bool {
	return func(d time.Time) bool { return mask&(1<<uint(d.Weekday())) != 0 }
}

// CompOff is a comp-off credit request (LP-08..LP-11).
type CompOff struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null" json:"employee_id"`
	WorkedDate        Date       `gorm:"type:date;not null" json:"worked_date"`
	Days              calc.Days  `gorm:"type:numeric(7,2);not null" json:"days"`
	Reason            string     `gorm:"type:varchar(500);not null" json:"reason"`
	Status            string     `gorm:"type:varchar(20);not null" json:"status"`
	RequestedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	DecidedByUserID   *uuid.UUID `gorm:"type:uuid" json:"decided_by_user_id,omitempty"`
	DecisionComment   *string    `gorm:"type:varchar(500)" json:"decision_comment,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	ExpiresOn         *Date      `gorm:"type:date" json:"expires_on,omitempty"`
	ExpiredDays       *calc.Days `gorm:"type:numeric(7,2)" json:"expired_days,omitempty"`
	ExpiredAt         *time.Time `json:"expired_at,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to leave_comp_offs.
func (CompOff) TableName() string { return "leave_comp_offs" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *CompOff) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

// EncashmentRule makes a leave type encashable (LP-12).
type EncashmentRule struct {
	TenantID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	LeaveTypeID     uuid.UUID `gorm:"type:uuid;primaryKey" json:"leave_type_id"`
	MaxDaysPerYear  calc.Days `gorm:"type:numeric(7,2);not null" json:"max_days_per_year"`
	MinBalanceAfter calc.Days `gorm:"type:numeric(7,2);not null" json:"min_balance_after"`
	UpdatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to leave_encashment_rules.
func (EncashmentRule) TableName() string { return "leave_encashment_rules" }

// Encashment is an encashment request (LP-13..LP-15).
type Encashment struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null" json:"employee_id"`
	LeaveTypeID       uuid.UUID  `gorm:"type:uuid;not null" json:"leave_type_id"`
	Year              int        `gorm:"not null" json:"year"`
	Days              calc.Days  `gorm:"type:numeric(7,2);not null" json:"days"`
	Reason            *string    `gorm:"type:varchar(500)" json:"reason,omitempty"`
	Status            string     `gorm:"type:varchar(20);not null" json:"status"`
	RequestedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	DecidedByUserID   *uuid.UUID `gorm:"type:uuid" json:"decided_by_user_id,omitempty"`
	DecisionComment   *string    `gorm:"type:varchar(500)" json:"decision_comment,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	PaidOutAt         *time.Time `json:"paid_out_at,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to leave_encashments.
func (Encashment) TableName() string { return "leave_encashments" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Encashment) BeforeCreate(*gorm.DB) error { return assignID(&r.ID) }

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
		return fmt.Errorf("plus: cannot scan %T as date", src)
	}
	return nil
}
