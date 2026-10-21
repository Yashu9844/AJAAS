// Package api is what HR/Employee publishes to the other HR contexts: the employee directory. Other contexts depend
// on this package and on employee/dto only, never on employee services, repositories or models (import-boundary
// test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/employee/dto"
	"gorm.io/gorm"
)

// Directory reads and creates employee records.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error)
	CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error)
}

// StatusChange is a worker status set by HR/Core when an HR action takes effect (core DC-07).
type StatusChange struct {
	Status           string     // probation | active | terminated
	ConfirmationDate *time.Time // probation confirmed
	ExitDate         *time.Time // last working day
	ExitReason       string     // reason code (no free text)
	Note             string     // timeline note
}

// Lifecycle lets HR/Core project worker status changes. Unlike the manual status matrix it may leave a final status
// (rehire, rescinded termination). It runs inside the caller's transaction.
type Lifecycle interface {
	ApplyCoreStatus(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, change StatusChange) (previous string, err error)
}
