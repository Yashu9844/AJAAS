// Package services holds Module 4 business rules (specification.md LV-001..LV-017).
package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/leave/repositories"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is Module 4's view of Module 2 (connections C2).
type EmployeeDirectory interface {
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// AuditLogger is Module 4's view of the Module 0 AuditService (connections C3).
type AuditLogger interface {
	Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

// AttendanceSync is Module 4's view of Module 3 services.LeaveSync (connections C8, D4-08).
type AttendanceSync interface {
	MarkLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error
	ClearLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error
}

// CalendarSource is the leave parity package's contribution to pricing and the leave year (LP-02..LP-05). A nil source
// keeps the tenant calendar, Saturday/Sunday weekly off and calendar-year leave year.
type CalendarSource interface {
	// YearStartMonth returns the month (1–12) in which the tenant's leave year starts.
	YearStartMonth(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int, error)
	// For returns the extra blocked dates of the employee's location in [from, to] and its weekly-off test (nil = default).
	For(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) (holidays []time.Time, weekOff func(time.Time) bool, err error)
}

// TxRunner opens database transactions; controllers never see *gorm.DB.
type TxRunner interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
}

// Clock returns the current instant; injected so goldens run on fixed dates.
type Clock func() time.Time

// Actor is the authenticated caller, from Module 0 middleware context only (NFR-SEC002).
type Actor struct {
	TenantID      uuid.UUID
	UserID        uuid.UUID
	IP            string
	UserAgent     string
	CorrelationID uuid.UUID
}

// Repos bundles the module repositories.
type Repos struct {
	Types    repositories.TypeRepository
	Holidays repositories.HolidayRepository
	Balances repositories.BalanceRepository
	Requests repositories.RequestRepository
	Ledger   repositories.LedgerRepository
	Outbox   repositories.OutboxRepository
}

// Deps is everything a Module 4 service needs; built once in module.go.
type Deps struct {
	Tx         TxRunner
	Repos      Repos
	Employees  EmployeeDirectory
	Audit      AuditLogger
	Attendance AttendanceSync
	Calendar   CalendarSource
	Publisher  queue.EventPublisher
	Now        Clock
}

type gormRunner struct{ db *gorm.DB }

// NewTxRunner wraps a GORM handle as a TxRunner.
func NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }

func (g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return g.db.WithContext(ctx).Transaction(fn)
}

func (g gormRunner) DB() *gorm.DB { return g.db }
