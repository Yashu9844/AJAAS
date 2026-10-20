package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/organization/dto"
	"github.com/jaas/jaas/internal/hr/organization/models"
	"gorm.io/gorm"
)

type locationRepository struct{}

// NewLocationRepository returns a LocationRepository.
func NewLocationRepository() LocationRepository {
	return &locationRepository{}
}

func (r *locationRepository) Create(ctx context.Context, tx *gorm.DB, l *models.Location) error {
	return tx.WithContext(ctx).Create(l).Error
}

func (r *locationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Location, error) {
	return firstLocation(db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID))
}

func (r *locationRepository) FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.Location, error) {
	return firstLocation(db.WithContext(ctx).Where("tenant_id = ? AND code = ?", tenantID, code))
}

func (r *locationRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Location, error) {
	return firstLocation(db.WithContext(ctx).Where("tenant_id = ? AND lower(name) = ?", tenantID, strings.ToLower(name)))
}

func (r *locationRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f dto.LocationFilter, page, perPage int) ([]models.Location, int64, error) {
	var out []models.Location
	var total int64
	query := db.WithContext(ctx).Model(&models.Location{}).Where("tenant_id = ?", tenantID)
	if f.Status != "" {
		query = query.Where("status = ?", f.Status)
	}
	if f.StateCode != "" {
		query = query.Where("state_code = ?", f.StateCode)
	}
	if f.Search != "" {
		like := "%" + strings.ToLower(f.Search) + "%"
		query = query.Where("lower(name) LIKE ? OR lower(code) LIKE ? OR lower(city) LIKE ?", like, like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * perPage
	err := query.Order("name ASC, id").Offset(offset).Limit(perPage).Find(&out).Error
	return out, total, err
}

func (r *locationRepository) Update(ctx context.Context, tx *gorm.DB, l *models.Location) error {
	return tx.WithContext(ctx).Save(l).Error
}

func firstLocation(q *gorm.DB) (*models.Location, error) {
	var l models.Location
	if err := q.First(&l).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}
