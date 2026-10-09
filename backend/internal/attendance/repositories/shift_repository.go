package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"gorm.io/gorm"
)

type shiftRepository struct{}

// NewShiftRepository returns a ShiftRepository.
func NewShiftRepository() ShiftRepository { return &shiftRepository{} }

var _ ShiftRepository = (*shiftRepository)(nil)

func (r *shiftRepository) Create(ctx context.Context, tx *gorm.DB, s *models.Shift) error {
	return tx.WithContext(ctx).Create(s).Error
}

func (r *shiftRepository) Update(ctx context.Context, tx *gorm.DB, s *models.Shift) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", s.TenantID).Save(s).Error
}

func (r *shiftRepository) first(ctx context.Context, db *gorm.DB, query string, args ...interface{}) (*models.Shift, error) {
	var s models.Shift
	if err := db.WithContext(ctx).Where(query, args...).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *shiftRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Shift, error) {
	return r.first(ctx, db, "tenant_id = ? AND id = ?", tenantID, id)
}

func (r *shiftRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Shift, error) {
	return r.first(ctx, db, "tenant_id = ? AND lower(name) = ?", tenantID, strings.ToLower(strings.TrimSpace(name)))
}

func (r *shiftRepository) FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.Shift, error) {
	return r.first(ctx, db, "tenant_id = ? AND code = ?", tenantID, code)
}

func (r *shiftRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Shift, int64, error) {
	q := db.WithContext(ctx).Model(&models.Shift{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.Shift
	err := q.Order("name ASC").Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}

type assignmentRepository struct{}

// NewAssignmentRepository returns an AssignmentRepository.
func NewAssignmentRepository() AssignmentRepository { return &assignmentRepository{} }

var _ AssignmentRepository = (*assignmentRepository)(nil)

func (r *assignmentRepository) Create(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error {
	return tx.WithContext(ctx).Create(a).Error
}

func (r *assignmentRepository) Update(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", a.TenantID).Save(a).Error
}

func (r *assignmentRepository) Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from time.Time, to *time.Time) ([]models.ShiftAssignment, error) {
	q := db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID).
		Where("(effective_to IS NULL OR effective_to >= ?::date)", day(from))
	if to != nil {
		q = q.Where("effective_from <= ?::date", day(*to))
	}
	var out []models.ShiftAssignment
	err := q.Order("effective_from ASC").Find(&out).Error
	return out, err
}

func (r *assignmentRepository) ActiveFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.ShiftAssignment, error) {
	var a models.ShiftAssignment
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID).
		Where("effective_from <= ?::date AND (effective_to IS NULL OR effective_to >= ?::date)", day(date), day(date)).
		Order("effective_from DESC").First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (r *assignmentRepository) CountCoveringFrom(ctx context.Context, db *gorm.DB, tenantID, shiftID uuid.UUID, date time.Time) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.ShiftAssignment{}).
		Where("tenant_id = ? AND shift_id = ?", tenantID, shiftID).
		Where("(effective_to IS NULL OR effective_to >= ?::date)", day(date)).
		Count(&n).Error
	return n, err
}

func (r *assignmentRepository) ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.ShiftAssignment, int64, error) {
	q := db.WithContext(ctx).Model(&models.ShiftAssignment{}).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.ShiftAssignment
	err := q.Order("effective_from DESC").Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}
