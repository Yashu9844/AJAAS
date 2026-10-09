// Package repositories is Module 5 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"gorm.io/gorm"
)

// StructureRepository persists structures and their immutable components (FR-ST).
type StructureRepository interface {
	Create(ctx context.Context, tx *gorm.DB, s *models.Structure, comps []models.Component) error
	Update(ctx context.Context, tx *gorm.DB, s *models.Structure) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Structure, error)
	FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Structure, error)
	Components(ctx context.Context, db *gorm.DB, tenantID, structureID uuid.UUID) ([]models.Component, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Structure, int64, error)
}

// Period is an inclusive date range.
type Period struct{ From, To time.Time }

// AssignmentRepository persists CTC assignments (FR-AS, PY-014).
type AssignmentRepository interface {
	LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error
	Create(ctx context.Context, tx *gorm.DB, a *models.Assignment) error
	Update(ctx context.Context, tx *gorm.DB, a *models.Assignment) error
	// Current returns the open (effective_to IS NULL) assignment, or nil.
	Current(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.Assignment, error)
	ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Assignment, int64, error)
	// EligibleFor returns, per employee, the latest assignment overlapping the period (D5-10).
	EligibleFor(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, p Period) ([]models.Assignment, error)
}

// RunRepository persists payroll runs (FR-RN).
type RunRepository interface {
	Create(ctx context.Context, tx *gorm.DB, r *models.Run) error
	Update(ctx context.Context, tx *gorm.DB, r *models.Run) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error)
	// LockByID loads the run with SELECT … FOR UPDATE (transitions are serialized).
	LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error)
	FindByPeriod(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, year, month int) (*models.Run, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Run, int64, error)
}

// PayslipRepository persists payslips and their lines (FR-PS, PY-011).
type PayslipRepository interface {
	DeleteByRun(ctx context.Context, tx *gorm.DB, tenantID, runID uuid.UUID) error
	CreateBatch(ctx context.Context, tx *gorm.DB, slips []models.Payslip) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Payslip, error)
	ListByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error)
	AllByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID) ([]models.Payslip, error)
	// ListFinalizedForEmployee returns the employee's payslips of finalized runs only (PY-013).
	ListFinalizedForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error)
}

// OutboxRepository persists outbox rows; relay methods are cross-tenant by design.
type OutboxRepository interface {
	Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error
	MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error
	RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error
	FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)
}

func day(t time.Time) string { return t.Format("2006-01-02") }

func first[T any](q *gorm.DB) (*T, error) {
	var out T
	err := q.Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error) {
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []T
	err := q.Order(order).Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}
