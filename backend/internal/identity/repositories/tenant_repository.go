package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type tenantRepository struct{}

// NewTenantRepository returns a TenantRepository.
func NewTenantRepository() TenantRepository {
	return &tenantRepository{}
}

func (r *tenantRepository) Create(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
	return tx.WithContext(ctx).Create(tenant).Error
}

func (r *tenantRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error) {
	var tenant models.Tenant
	err := db.WithContext(ctx).First(&tenant, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tenant, nil
}

func (r *tenantRepository) FindBySlug(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
	var tenant models.Tenant
	err := db.WithContext(ctx).First(&tenant, "slug = ?", slug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tenant, nil
}

func (r *tenantRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error) {
	var tenants []models.Tenant
	var total int64

	query := db.WithContext(ctx).Model(&models.Tenant{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	err := query.Offset(offset).Limit(perPage).Find(&tenants).Error
	if err != nil {
		return nil, 0, err
	}

	return tenants, total, nil
}

func (r *tenantRepository) Update(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
	return tx.WithContext(ctx).Save(tenant).Error
}

func (r *tenantRepository) Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	return tx.WithContext(ctx).Delete(&models.Tenant{}, "id = ?", id).Error
}
