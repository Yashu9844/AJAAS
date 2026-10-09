// Package repositories is Module 3 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"gorm.io/gorm"
)

// ShiftRepository persists shifts (FR-SH).
type ShiftRepository interface {
	Create(ctx context.Context, tx *gorm.DB, s *models.Shift) error
	Update(ctx context.Context, tx *gorm.DB, s *models.Shift) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Shift, error)
	FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Shift, error)
	FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.Shift, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Shift, int64, error)
}

// AssignmentRepository persists shift assignments (FR-SA).
type AssignmentRepository interface {
	Create(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error
	Update(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error
	// Overlapping returns assignments intersecting [from, to]; to nil = open-ended.
	Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from time.Time, to *time.Time) ([]models.ShiftAssignment, error)
	ActiveFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.ShiftAssignment, error)
	CountCoveringFrom(ctx context.Context, db *gorm.DB, tenantID, shiftID uuid.UUID, date time.Time) (int64, error)
	ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.ShiftAssignment, int64, error)
}

// RecordFilter narrows admin record lists (A7).
type RecordFilter struct {
	EmployeeID *uuid.UUID
	From, To   time.Time
	Status     string
}

// StatusCounts is the per-date aggregate behind A9 (AT-020).
type StatusCounts struct {
	ByStatus map[string]int64
	Late     int64
}

// RecordRepository persists attendance records (FR-AR).
type RecordRepository interface {
	// LockEmployee serializes writes for one employee until the tx ends (AT-022).
	LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error
	// FindOrCreate returns the record for (employee, date), inserting `seed` if absent.
	FindOrCreate(ctx context.Context, tx *gorm.DB, seed *models.AttendanceRecord) (*models.AttendanceRecord, error)
	Update(ctx context.Context, tx *gorm.DB, r *models.AttendanceRecord) error
	// Delete removes a leave-only record (D3-17); records with punches are never deleted.
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.AttendanceRecord, error)
	FindByDate(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.AttendanceRecord, error)
	ListForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.AttendanceRecord, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RecordFilter, page dto.Page) ([]models.AttendanceRecord, int64, error)
	CountByStatus(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (StatusCounts, error)
}

// PunchRepository persists append-only punches (AT-019).
type PunchRepository interface {
	Create(ctx context.Context, tx *gorm.DB, p *models.AttendancePunch) error
	// LastForEmployee returns the newest non-superseded punch.
	LastForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.AttendancePunch, error)
	ActiveForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error)
	AllForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error)
	SupersedeForRecord(ctx context.Context, tx *gorm.DB, tenantID, recordID uuid.UUID) error
}

// RegularizationFilter narrows regularization lists (A5/A10).
type RegularizationFilter struct {
	EmployeeID *uuid.UUID
	Status     string
}

// RegularizationRepository persists correction requests (FR-RG).
type RegularizationRepository interface {
	Create(ctx context.Context, tx *gorm.DB, g *models.Regularization) error
	Update(ctx context.Context, tx *gorm.DB, g *models.Regularization) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Regularization, error)
	FindPending(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.Regularization, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RegularizationFilter, page dto.Page) ([]models.Regularization, int64, error)
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
