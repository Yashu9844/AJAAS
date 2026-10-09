// Package services holds Module 6 business rules (specification.md RC-001..RC-011).
package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is Module 6's view of Module 2 (connections C5).
type EmployeeDirectory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	CreateEmployee(ctx context.Context, tenantID uuid.UUID, req employeeDTO.CreateEmployeeRequest) (*employeeDTO.EmployeeResponse, error)
}

// UserInviter is Module 6's view of Module 0 UserService.InviteUser (connections C2, D6-02).
type UserInviter interface {
	InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req identityDTO.InviteUserRequest, correlationID uuid.UUID) (*identityDTO.UserResponse, error)
}

// OrgDirectory answers whether Module 1 references exist (connections C4, D6-03).
type OrgDirectory interface {
	DepartmentExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error)
	DesignationExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error)
}

// AuditLogger is Module 6's view of the Module 0 AuditService.
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
	Jobs       repositories.JobRepository
	Candidates repositories.CandidateRepository
	Interviews repositories.InterviewRepository
	Offers     repositories.OfferRepository
	Outbox     repositories.OutboxRepository
}

// Deps is everything a Module 6 service needs.
type Deps struct {
	Tx        TxRunner
	Repos     Repos
	Employees EmployeeDirectory
	Users     UserInviter
	Org       OrgDirectory
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
