package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type roleRepository struct{}

// NewRoleRepository returns a RoleRepository.
func NewRoleRepository() RoleRepository {
	return &roleRepository{}
}

func (r *roleRepository) Create(ctx context.Context, tx *gorm.DB, role *models.Role) error {
	return tx.WithContext(ctx).Create(role).Error
}

func (r *roleRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error) {
	var role models.Role
	err := db.WithContext(ctx).First(&role, "tenant_id = ? AND id = ?", tenantID, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Role, error) {
	var role models.Role
	err := db.WithContext(ctx).First(&role, "tenant_id = ? AND name = ?", tenantID, name).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Role, int64, error) {
	var roles []models.Role
	var total int64

	query := db.WithContext(ctx).Model(&models.Role{}).Where("tenant_id = ?", tenantID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	err := query.Offset(offset).Limit(perPage).Find(&roles).Error
	if err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

func (r *roleRepository) Update(ctx context.Context, tx *gorm.DB, role *models.Role) error {
	return tx.WithContext(ctx).Where("tenant_id = ? AND id = ?", role.TenantID, role.ID).Save(role).Error
}

func (r *roleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return tx.WithContext(ctx).Delete(&models.Role{}, "tenant_id = ? AND id = ?", tenantID, id).Error
}
