// Package services holds HR/Performance business rules (specification.md PM-001..PM-017).
package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is HR/Performance's view of HR/Employee (connection C4); employee/api.Directory satisfies it.
type EmployeeDirectory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// ManagerDirectory resolves a user's direct manager from HR/Organization (connection C3, D7-03): the manager_user_id
// of the user's primary active mapping, or nil.
type ManagerDirectory interface {
	DirectManager(ctx context.Context, tenantID, userID uuid.UUID) (*uuid.UUID, error)
}

// AuditLogger is HR/Performance's view of the M0 audit trail.
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

// Actor is the authenticated caller from M0 middleware context only (NFR-SEC002). CanRead and CanManage are the
// performance:read / performance:manage flags resolved by the route (PermissionFlag).
type Actor struct {
	TenantID, UserID, CorrelationID uuid.UUID
	IP, UserAgent                   string
	CanRead, CanManage              bool
}

// Repos bundles the context repositories.
type Repos struct {
	Cycles  repositories.CycleRepository
	Goals   repositories.GoalRepository
	Reviews repositories.ReviewRepository
	Outbox  repositories.OutboxRepository
}

// Deps is everything an HR/Performance service needs.
type Deps struct {
	Tx        TxRunner
	Repos     Repos
	Employees EmployeeDirectory
	Managers  ManagerDirectory
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
