// Package services holds Module 5 business rules (specification.md PY-001..PY-015).
package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	leavecalc "github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/payroll/repositories"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is Module 5's view of Module 2 (connections C2).
type EmployeeDirectory interface {
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// LeaveLOP is Module 5's view of Module 4 `services.UnpaidLeave` (connections C3, D5-03).
type LeaveLOP interface {
	UnpaidDays(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (leavecalc.Days, error)
}

// AuditLogger is Module 5's view of the Module 0 AuditService.
type AuditLogger interface {
	Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

// TxRunner opens database transactions.
type TxRunner interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
}

// Clock returns the current instant.
type Clock func() time.Time

// Actor is the authenticated caller from Module 0 middleware context only.
type Actor struct {
	TenantID, UserID, CorrelationID uuid.UUID
	IP, UserAgent                   string
}

// Repos bundles the module repositories.
type Repos struct {
	Structures  repositories.StructureRepository
	Assignments repositories.AssignmentRepository
	Runs        repositories.RunRepository
	Payslips    repositories.PayslipRepository
	Outbox      repositories.OutboxRepository
}

// Deps is everything a Module 5 service needs.
type Deps struct {
	Tx        TxRunner
	Repos     Repos
	Employees EmployeeDirectory
	Leave     LeaveLOP
	Audit     AuditLogger
	Publisher queue.EventPublisher
	Now       Clock
}

type gormRunner struct{ db *gorm.DB }

// NewTxRunner wraps a GORM handle as a TxRunner.
func NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }

func (g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return g.db.WithContext(ctx).Transaction(fn)
}

func (g gormRunner) DB() *gorm.DB { return g.db }
