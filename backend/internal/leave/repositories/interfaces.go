// Package repositories is Module 4 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
)

// TypeRepository persists leave types (FR-LT).
type TypeRepository interface {
	Create(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error
	Update(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.LeaveType, error)
	FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.LeaveType, error)
	FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.LeaveType, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.LeaveType, int64, error)
}

// HolidayRepository persists the tenant holiday calendar (FR-HD).
type HolidayRepository interface {
	Create(ctx context.Context, tx *gorm.DB, h *models.Holiday) error
	Delete(ctx context.Context, tx *gorm.DB, h *models.Holiday) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Holiday, error)
	FindByDate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (*models.Holiday, error)
	// ListRange returns holidays in [from, to], optional ones included, ordered by date.
	ListRange(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]models.Holiday, error)
}

// BalanceKey identifies one balance row.
type BalanceKey struct {
	TenantID, EmployeeID, LeaveTypeID uuid.UUID
	Year                              int
}

// BalanceRepository persists balance projections (D4-02).
type BalanceRepository interface {
	// LockEmployee serializes leave writes for one employee until the tx ends (LV-014).
	LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error
	// FindOrCreate returns the row for key, inserting a zero row if absent; created reports the insert.
	FindOrCreate(ctx context.Context, tx *gorm.DB, key BalanceKey) (b *models.Balance, created bool, err error)
	Find(ctx context.Context, db *gorm.DB, key BalanceKey) (*models.Balance, error)
	Update(ctx context.Context, tx *gorm.DB, b *models.Balance) error
}

// RequestFilter narrows request lists (R3/R5).
type RequestFilter struct {
	EmployeeID *uuid.UUID
	Status     string
	From, To   *time.Time
}

// RequestRepository persists leave requests (FR-LR).
type RequestRepository interface {
	Create(ctx context.Context, tx *gorm.DB, r *models.Request) error
	Update(ctx context.Context, tx *gorm.DB, r *models.Request) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Request, error)
	// Overlapping returns the employee's pending/approved requests intersecting [from, to] (LV-008).
	Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.Request, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.Request, int64, error)
}

// LedgerFilter narrows ledger lists (B4).
type LedgerFilter struct {
	EmployeeID  uuid.UUID
	LeaveTypeID *uuid.UUID
}

// LedgerRepository appends ledger rows (LV-013, LV-016) — there is no update or delete.
type LedgerRepository interface {
	Create(ctx context.Context, tx *gorm.DB, e *models.LedgerEntry) error
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f LedgerFilter, page dto.Page) ([]models.LedgerEntry, int64, error)
}

// OutboxRepository persists outbox rows. Relay methods are cross-tenant by design (system worker).
type OutboxRepository interface {
	Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error
	MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error
	RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error
	FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)
}

// day renders a calendar date for `?::date` comparisons (session-timezone independent).
func day(t time.Time) string { return t.Format("2006-01-02") }

// first runs a single-row query and maps "not found" to (nil, nil).
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

// paged counts then fetches one page of q.
func paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error) {
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []T
	err := q.Order(order).Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}
