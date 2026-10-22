// Package ops is the HR/Attendance parity package (docs/modules/m1-hr-management/contexts/attendance, AP-01..AP-12):
// manager inbox, tenant policy with overtime rules, rosters, the auto-absent job and bulk import. It writes attendance
// data only through attendance/services.TimeWriter.
package ops

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Roster statuses and policy defaults (AP-03).
const (
	RosterActive   = "active"
	RosterArchived = "archived"

	defaultWeekOffMask  = 1<<0 | 1<<6 // Sunday and Saturday
	defaultOTMinDaily   = 30
	defaultOTMaxDaily   = 240
	defaultOTMaxMonthly = 3600
	defaultRateWeekday  = 150
	defaultRateWeekOff  = 200

	jobAutoAbsent = "auto_absent"
)

// Policy is the tenant attendance policy (AP-03).
type Policy struct {
	TenantID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	WeekOffMask         int       `gorm:"column:week_off_mask;not null" json:"-"`
	OvertimeMinDaily    int       `gorm:"not null" json:"overtime_min_daily"`
	OvertimeMaxDaily    int       `gorm:"not null" json:"overtime_max_daily"`
	OvertimeMaxMonthly  int       `gorm:"not null" json:"overtime_max_monthly"`
	OvertimeRateWeekday int       `gorm:"not null" json:"overtime_rate_weekday"`
	OvertimeRateWeekOff int       `gorm:"column:overtime_rate_week_off;not null" json:"overtime_rate_week_off"`
	AutoAbsentEnabled   bool      `gorm:"not null" json:"auto_absent_enabled"`
	UpdatedByUserID     uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt           time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to attendance_policies.
func (Policy) TableName() string { return "attendance_policies" }

// defaults returns the policy a tenant has before it saves one.
func defaults(tenantID uuid.UUID) *Policy {
	return &Policy{TenantID: tenantID, WeekOffMask: defaultWeekOffMask, OvertimeMinDaily: defaultOTMinDaily, OvertimeMaxDaily: defaultOTMaxDaily,
		OvertimeMaxMonthly: defaultOTMaxMonthly, OvertimeRateWeekday: defaultRateWeekday, OvertimeRateWeekOff: defaultRateWeekOff}
}

func (p *Policy) weekOff(d time.Time) bool { return p.WeekOffMask&(1<<uint(d.Weekday())) != 0 }

// weekOffDays lists the week-off weekday numbers, ascending.
func (p *Policy) weekOffDays() []int {
	out := []int{}
	for d := 0; d < 7; d++ {
		if p.WeekOffMask&(1<<uint(d)) != 0 {
			out = append(out, d)
		}
	}
	return out
}

// Segment is one run of a roster pattern (AP-05).
type Segment struct {
	ShiftID uuid.UUID `json:"shift_id"`
	Days    int       `json:"days"`
}

// Pattern is a roster's segments, stored as JSON.
type Pattern []Segment

// Value encodes the pattern.
func (p Pattern) Value() (driver.Value, error) {
	b, err := json.Marshal([]Segment(p))
	return string(b), err
}

// Scan decodes the pattern.
func (p *Pattern) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), p)
	case []byte:
		return json.Unmarshal(v, p)
	}
	return fmt.Errorf("ops: cannot scan %T as pattern", src)
}

// Roster is a repeating shift pattern (AP-05).
type Roster struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`
	Pattern         Pattern   `gorm:"type:jsonb;not null" json:"pattern"`
	Status          string    `gorm:"type:varchar(20);not null" json:"status"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to attendance_rosters.
func (Roster) TableName() string { return "attendance_rosters" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Roster) BeforeCreate(*gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// ImportLog is one applied bulk import (AP-11).
type ImportLog struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	RowsTotal       int       `gorm:"not null" json:"rows_total"`
	RowsApplied     int       `gorm:"not null" json:"rows_applied"`
	Overwrite       bool      `gorm:"not null" json:"overwrite"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"created_by_user_id"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
}

// TableName maps to attendance_imports.
func (ImportLog) TableName() string { return "attendance_imports" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *ImportLog) BeforeCreate(*gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// JobRun is one auto-absent run (AP-08).
type JobRun struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	Kind              string     `gorm:"type:varchar(30);not null" json:"kind"`
	RunDate           Date       `gorm:"type:date;not null" json:"run_date"`
	Absent            int        `gorm:"not null" json:"absent"`
	WeekOff           int        `gorm:"column:week_off;not null" json:"week_off"`
	Holiday           int        `gorm:"not null" json:"holiday"`
	Skipped           int        `gorm:"not null" json:"skipped"`
	TriggeredByUserID *uuid.UUID `gorm:"type:uuid" json:"triggered_by_user_id,omitempty"`
	RanAt             time.Time  `gorm:"not null" json:"ran_at"`
}

// TableName maps to attendance_job_runs.
func (JobRun) TableName() string { return "attendance_job_runs" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *JobRun) BeforeCreate(*gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
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
		return fmt.Errorf("ops: cannot scan %T as date", src)
	}
	return nil
}
