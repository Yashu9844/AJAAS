package repositories

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
)

type typeRepository struct{}

// NewTypeRepository builds a TypeRepository.
func NewTypeRepository() TypeRepository { return &typeRepository{} }

func (r *typeRepository) Create(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error {
	return tx.WithContext(ctx).Create(t).Error
}

func (r *typeRepository) Update(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", t.TenantID).Save(t).Error
}

func (r *typeRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.LeaveType, error) {
	return first[models.LeaveType](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *typeRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.LeaveType, error) {
	return first[models.LeaveType](db.WithContext(ctx).Where("tenant_id = ? AND lower(name) = ?", tenantID, strings.ToLower(name)))
}

func (r *typeRepository) FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.LeaveType, error) {
	return first[models.LeaveType](db.WithContext(ctx).Where("tenant_id = ? AND code = ?", tenantID, code))
}

func (r *typeRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.LeaveType, int64, error) {
	q := db.WithContext(ctx).Model(&models.LeaveType{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paged[models.LeaveType](q, "code ASC", page)
}

type holidayRepository struct{}

// NewHolidayRepository builds a HolidayRepository.
func NewHolidayRepository() HolidayRepository { return &holidayRepository{} }

func (r *holidayRepository) Create(ctx context.Context, tx *gorm.DB, h *models.Holiday) error {
	return tx.WithContext(ctx).Create(h).Error
}

// Delete soft-deletes (FR-HD003); existing requests keep their snapshot (LV-012).
func (r *holidayRepository) Delete(ctx context.Context, tx *gorm.DB, h *models.Holiday) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", h.TenantID).Delete(h).Error
}

func (r *holidayRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Holiday, error) {
	return first[models.Holiday](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *holidayRepository) FindByDate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (*models.Holiday, error) {
	return first[models.Holiday](db.WithContext(ctx).Where("tenant_id = ? AND holiday_date = ?::date", tenantID, day(date)))
}

func (r *holidayRepository) ListRange(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]models.Holiday, error) {
	var out []models.Holiday
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND holiday_date BETWEEN ?::date AND ?::date", tenantID, day(from), day(to)).
		Order("holiday_date ASC").Find(&out).Error
	return out, err
}
