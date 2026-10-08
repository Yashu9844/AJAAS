// Package services holds Module 3 business rules (specification.md AT-001..AT-022).
package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/repositories"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is Module 3's view of Module 2 (connections C2).
type EmployeeDirectory interface {
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	// CountWorking counts employees whose status allows work (AT-002 statuses).
	CountWorking(ctx context.Context, tenantID uuid.UUID) (int64, error)
}

// AuditLogger is Module 3's view of the Module 0 AuditService (connections C3).
type AuditLogger interface {
	Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

// TxRunner opens database transactions; services never touch *gorm.DB transactions directly (D3-05).
type TxRunner interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
}

// Clock returns the current instant; injected so goldens run on fixed time (AT-001).
type Clock func() time.Time

// Actor is the authenticated caller, taken from Module 0 middleware context only (NFR-SEC002).
type Actor struct {
	TenantID      uuid.UUID
	UserID        uuid.UUID
	IP            string
	UserAgent     string
	CorrelationID uuid.UUID
}

// Repos bundles the module repositories.
type Repos struct {
	Shifts          repositories.ShiftRepository
	Assignments     repositories.AssignmentRepository
	Records         repositories.RecordRepository
	Punches         repositories.PunchRepository
	Regularizations repositories.RegularizationRepository
	Outbox          repositories.OutboxRepository
}

// Deps is everything a Module 3 service needs; built once in module.go.
type Deps struct {
	Tx        TxRunner
	Repos     Repos
	Employees EmployeeDirectory
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
