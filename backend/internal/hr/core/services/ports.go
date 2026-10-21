// Package services holds HR/Core business rules (specification.md CO-001..CO-051).
package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	employeeAPI "github.com/jaas/jaas/internal/hr/employee/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is core's view of HR/Employee (connection C2); employee/api.Directory satisfies it.
type EmployeeDirectory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error)
}

// OrgDirectory is core's view of HR/Organization (connection C3); organization.Module satisfies it.
type OrgDirectory interface {
	DepartmentState(ctx context.Context, tenantID, id uuid.UUID) (exists, active bool, err error)
	LocationState(ctx context.Context, tenantID, id uuid.UUID) (exists, active bool, err error)
	PrimaryPlacement(ctx context.Context, tenantID, userID uuid.UUID) (dept, location, managerUser *uuid.UUID, err error)
	ProjectPrimaryMapping(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, dept, location, managerUser *uuid.UUID) error
}

// Logins deactivates and reactivates M0 logins (connection C4); identityport.Users satisfies it.
type Logins interface {
	DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id, correlationID uuid.UUID) error
	ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

// AuditLogger is core's view of the M0 audit trail.
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

// Actor is the authenticated caller from M0 middleware context only.
type Actor struct {
	TenantID, UserID, CorrelationID uuid.UUID
	IP, UserAgent                   string
}

// Deps is everything an HR/Core service needs.
type Deps struct {
	Tx        TxRunner
	Repo      repositories.Repository
	Employees EmployeeDirectory
	Lifecycle employeeAPI.Lifecycle
	Org       OrgDirectory
	Logins    Logins
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
