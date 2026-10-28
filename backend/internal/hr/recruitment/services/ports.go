// Package services holds Module 6 business rules (specification.md RC-001..RC-011).
package services

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	"github.com/jaas/jaas/internal/hr/recruitment/models"
	"github.com/jaas/jaas/internal/hr/recruitment/repositories"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EmployeeDirectory is Module 6's view of Module 2 (connections C5).
type EmployeeDirectory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	CreateEmployee(ctx context.Context, tenantID uuid.UUID, req employeeDTO.CreateEmployeeRequest) (*employeeDTO.EmployeeResponse, error)
}

// UserInviter is HR/Recruitment's view of M0 user invitation (connections C2, D6-02); identityport.Users satisfies it.
type UserInviter interface {
	InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in identityport.Invite, correlationID uuid.UUID) (*identityport.User, error)
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
	Hooks     *Hooks
	OfferGate OfferGate
}

// OfferGate decides an offer's starting status (RP-06); nil keeps every offer `offered`.
type OfferGate interface {
	StartingStatus(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, ctc kernel.Money) (string, error)
}

// OfferGateSwitch is an OfferGate installed after construction.
type OfferGateSwitch struct {
	mu   sync.RWMutex
	gate OfferGate
}

// Set installs the gate.
func (g *OfferGateSwitch) Set(x OfferGate) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gate = x
}

// StartingStatus implements OfferGate.
func (g *OfferGateSwitch) StartingStatus(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, ctc kernel.Money) (string, error) {
	g.mu.RLock()
	x := g.gate
	g.mu.RUnlock()
	if x == nil {
		return models.OfferOffered, nil
	}
	return x.StartingStatus(ctx, tx, tenantID, ctc)
}

type gormRunner struct{ db *gorm.DB }

// NewTxRunner wraps a GORM handle as a TxRunner.
func NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }

func (g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return g.db.WithContext(ctx).Transaction(fn)
}

func (g gormRunner) DB() *gorm.DB { return g.db }
