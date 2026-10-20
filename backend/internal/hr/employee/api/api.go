// Package api is what HR/Employee publishes to the other HR contexts: the employee directory. Other contexts depend
// on this package and on employee/dto only, never on employee services, repositories or models (import-boundary
// test in internal/hr).
package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/employee/dto"
)

// Directory reads and creates employee records.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error)
	CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error)
}
