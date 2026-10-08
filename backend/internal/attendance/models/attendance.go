package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AttendanceRecord is one employee's computed day (FR-AR001/FR-AR002). Never deleted (AT-019).
type AttendanceRecord struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_attendance_records_employee_date,priority:1;index:idx_attendance_records_tenant_date_status,priority:1"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_attendance_records_employee_date,priority:2"`
	AttendanceDate    time.Time  `gorm:"type:date;not null;uniqueIndex:uq_attendance_records_employee_date,priority:3;index:idx_attendance_records_tenant_date_status,priority:2"`
	ShiftID           *uuid.UUID `gorm:"type:uuid"`
	FirstPunchIn      *time.Time
	LastPunchOut      *time.Time
	TotalWorkMinutes  int       `gorm:"not null"`
	TotalBreakMinutes int       `gorm:"not null"`
	LateMinutes       int       `gorm:"not null"`
	OvertimeMinutes   int       `gorm:"not null"`
	Status            string    `gorm:"type:varchar(20);not null;index:idx_attendance_records_tenant_date_status,priority:3"`
	Source            string    `gorm:"type:varchar(20);not null"`
	IsRegularized     bool      `gorm:"not null"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

// BeforeCreate assigns a UUID v4 when unset.
func (r *AttendanceRecord) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// AttendancePunch is an immutable IN/OUT event; corrections supersede, never delete (AT-017, AT-019).
type AttendancePunch struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID           uuid.UUID `gorm:"type:uuid;not null;index:idx_attendance_punches_employee_time,priority:1"`
	AttendanceRecordID uuid.UUID `gorm:"type:uuid;not null;index:idx_attendance_punches_record"`
	EmployeeProfileID  uuid.UUID `gorm:"type:uuid;not null;index:idx_attendance_punches_employee_time,priority:2"`
	PunchTime          time.Time `gorm:"not null;index:idx_attendance_punches_employee_time,priority:3"`
	PunchType          string    `gorm:"type:varchar(10);not null"`
	Source             string    `gorm:"type:varchar(20);not null"`
	Latitude           *float64  `gorm:"type:numeric(10,8)"`
	Longitude          *float64  `gorm:"type:numeric(11,8)"`
	DeviceID           *string   `gorm:"type:varchar(100)"`
	IPAddress          *string   `gorm:"type:varchar(45)"`
	IsSuperseded       bool      `gorm:"not null"`
	CreatedAt          time.Time `gorm:"not null"`
}

// BeforeCreate assigns a UUID v4 when unset.
func (p *AttendancePunch) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// Regularization is an employee's timesheet correction request (FR-RG001..FR-RG003).
type Regularization struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null;index:idx_attendance_regularizations_employee_date,priority:1;index:idx_attendance_regularizations_tenant_status,priority:1"`
	EmployeeProfileID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_attendance_regularizations_employee_date,priority:2"`
	AttendanceDate     time.Time  `gorm:"type:date;not null;index:idx_attendance_regularizations_employee_date,priority:3"`
	AttendanceRecordID *uuid.UUID `gorm:"type:uuid"`
	RequestedPunchIn   time.Time  `gorm:"not null"`
	RequestedPunchOut  time.Time  `gorm:"not null"`
	Reason             string     `gorm:"type:varchar(500);not null"`
	Status             string     `gorm:"type:varchar(20);not null;index:idx_attendance_regularizations_tenant_status,priority:2"`
	RequestedByUserID  uuid.UUID  `gorm:"type:uuid;not null"`
	ReviewerUserID     *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt         *time.Time
	ReviewComment      *string   `gorm:"type:varchar(500)"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

// TableName maps to attendance_regularizations (migration 000029).
func (Regularization) TableName() string { return "attendance_regularizations" }

// BeforeCreate assigns a UUID v4 when unset.
func (g *Regularization) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

// OutboxEvent is a transactional-outbox row written with the business change (FR-EV002, D3-06).
type OutboxEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_attendance_outbox_tenant"`
	EventID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_attendance_outbox_event_id"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null;index:idx_attendance_outbox_published"`
	PublishedAt *time.Time
	Attempts    int       `gorm:"not null"`
	LastError   *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to attendance_events_outbox (migration 000030).
func (OutboxEvent) TableName() string { return "attendance_events_outbox" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *OutboxEvent) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}
