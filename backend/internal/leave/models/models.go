// Package models holds Module 4 (Leave) persistence models; migrations 000032–000037 are the SQL twin.
package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Status, accrual, half-day and ledger-kind values (specification.md §3–§4).
const (
	TypeActive   = "active"
	TypeInactive = "inactive"

	AccrualAnnual  = "annual"
	AccrualMonthly = "monthly"

	GenderAll    = "all"
	GenderMale   = "male"
	GenderFemale = "female"

	HalfFirst  = "first_half"
	HalfSecond = "second_half"

	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"

	KindCarryForward = "carry_forward"
	KindAccrual      = "accrual"
	KindAdjustment   = "adjustment"
	KindReserve      = "reserve"
	KindRelease      = "release"
	KindConsume      = "consume"
	KindReversal     = "reversal"
)

// LeaveType is a tenant leave policy (FR-LT001). Integer fields carry no GORM default so explicit 0 is stored.
type LeaveType struct {
	database.BaseModel
	TenantID           uuid.UUID `gorm:"type:uuid;not null;index:idx_leave_types_tenant_id;uniqueIndex:uq_leave_types_tenant_name,priority:1,where:deleted_at IS NULL;uniqueIndex:uq_leave_types_tenant_code,priority:1,where:deleted_at IS NULL"`
	Name               string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_leave_types_tenant_name,priority:2,expression:lower(name)"`
	Code               string    `gorm:"type:varchar(10);not null;uniqueIndex:uq_leave_types_tenant_code,priority:2"`
	IsPaid             bool      `gorm:"not null"`
	AnnualAllowance    calc.Days `gorm:"type:numeric(7,2);not null"`
	Accrual            string    `gorm:"type:varchar(10);not null"`
	CarryForwardLimit  calc.Days `gorm:"type:numeric(7,2);not null"`
	MaxConsecutiveDays *int      `gorm:"type:integer"`
	MinNoticeDays      int       `gorm:"type:integer;not null"`
	AllowHalfDay       bool      `gorm:"not null"`
	SandwichRule       bool      `gorm:"not null"`
	ApplicableGender   string    `gorm:"type:varchar(10);not null"`
	Status             string    `gorm:"type:varchar(20);not null;index:idx_leave_types_status"`
}

// TableName maps to leave_types (migration 000032).
func (LeaveType) TableName() string { return "leave_types" }

// BeforeCreate assigns a UUID v4 when unset.
func (l *LeaveType) BeforeCreate(tx *gorm.DB) error { return assignID(&l.ID) }

// Holiday is one tenant calendar date excluded from day counting unless optional (FR-HD001, D4-05).
type Holiday struct {
	database.BaseModel
	TenantID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_leave_holidays_tenant_date,priority:1,where:deleted_at IS NULL"`
	HolidayDate time.Time `gorm:"type:date;not null;uniqueIndex:uq_leave_holidays_tenant_date,priority:2"`
	Name        string    `gorm:"type:varchar(100);not null"`
	IsOptional  bool      `gorm:"not null"`
}

// TableName maps to leave_holidays (migration 000033).
func (Holiday) TableName() string { return "leave_holidays" }

// BeforeCreate assigns a UUID v4 when unset.
func (h *Holiday) BeforeCreate(tx *gorm.DB) error { return assignID(&h.ID) }

// Balance is the per (employee, type, year) projection of the ledger (D4-02, LV-013).
type Balance struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_leave_balances_employee_type_year,priority:1"`
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_leave_balances_employee_type_year,priority:2"`
	LeaveTypeID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_leave_balances_employee_type_year,priority:3"`
	Year              int       `gorm:"type:integer;not null;uniqueIndex:uq_leave_balances_employee_type_year,priority:4"`
	Opening           calc.Days `gorm:"type:numeric(7,2);not null"`
	Accrued           calc.Days `gorm:"type:numeric(7,2);not null"`
	Adjusted          calc.Days `gorm:"type:numeric(7,2);not null"`
	Used              calc.Days `gorm:"type:numeric(7,2);not null"`
	Reserved          calc.Days `gorm:"type:numeric(7,2);not null"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

// TableName maps to leave_balances (migration 000034).
func (Balance) TableName() string { return "leave_balances" }

// BeforeCreate assigns a UUID v4 when unset.
func (b *Balance) BeforeCreate(tx *gorm.DB) error { return assignID(&b.ID) }

// Available is opening + accrued + adjusted − used − reserved.
func (b *Balance) Available() calc.Days {
	return b.Opening + b.Accrued + b.Adjusted - b.Used - b.Reserved
}

// Request is a leave application; TotalDays is a snapshot at apply time (LV-012).
type Request struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null;index:idx_leave_requests_employee_start,priority:1;index:idx_leave_requests_tenant_status,priority:1"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;index:idx_leave_requests_employee_start,priority:2"`
	LeaveTypeID       uuid.UUID  `gorm:"type:uuid;not null"`
	StartDate         time.Time  `gorm:"type:date;not null;index:idx_leave_requests_employee_start,priority:3"`
	EndDate           time.Time  `gorm:"type:date;not null"`
	HalfDay           *string    `gorm:"type:varchar(12)"`
	TotalDays         calc.Days  `gorm:"type:numeric(7,2);not null"`
	Reason            string     `gorm:"type:text;not null"`
	Status            string     `gorm:"type:varchar(20);not null;index:idx_leave_requests_tenant_status,priority:2"`
	RequestedByUserID uuid.UUID  `gorm:"type:uuid;not null"`
	ReviewerUserID    *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt        *time.Time
	ReviewComment     *string `gorm:"type:varchar(500)"`
	CancelledAt       *time.Time
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

// TableName maps to leave_requests (migration 000035).
func (Request) TableName() string { return "leave_requests" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Request) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }

// LedgerEntry is one append-only balance movement (LV-013, LV-016).
type LedgerEntry struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null;index:idx_leave_ledger_balance,priority:1"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;index:idx_leave_ledger_balance,priority:2"`
	LeaveTypeID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_leave_ledger_balance,priority:3"`
	Year              int        `gorm:"type:integer;not null;index:idx_leave_ledger_balance,priority:4"`
	Kind              string     `gorm:"type:varchar(20);not null"`
	Days              calc.Days  `gorm:"type:numeric(7,2);not null"`
	LeaveRequestID    *uuid.UUID `gorm:"type:uuid"`
	ActorUserID       *uuid.UUID `gorm:"type:uuid"`
	Note              *string    `gorm:"type:varchar(500)"`
	CreatedAt         time.Time  `gorm:"not null"`
}

// TableName maps to leave_ledger (migration 000036).
func (LedgerEntry) TableName() string { return "leave_ledger" }

// BeforeCreate assigns a UUID v4 when unset.
func (e *LedgerEntry) BeforeCreate(tx *gorm.DB) error { return assignID(&e.ID) }

// OutboxEvent is a transactional-outbox row written with the business change (FR-EV002).
type OutboxEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_leave_outbox_tenant"`
	EventID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_leave_outbox_event_id"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null;index:idx_leave_outbox_published,where:published = false"`
	PublishedAt *time.Time
	Attempts    int       `gorm:"type:integer;not null"`
	LastError   *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to leave_events_outbox (migration 000037).
func (OutboxEvent) TableName() string { return "leave_events_outbox" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}
