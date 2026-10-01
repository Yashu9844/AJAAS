package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type permissionRepository struct{}

// NewPermissionRepository returns a PermissionRepository.
func NewPermissionRepository() PermissionRepository {
	return &permissionRepository{}
}

func (r *permissionRepository) Create(ctx context.Context, tx *gorm.DB, permission *models.Permission) error {
	return tx.WithContext(ctx).Create(permission).Error
}

func (r *permissionRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Permission, error) {
	var perm models.Permission
	err := db.WithContext(ctx).First(&perm, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &perm, nil
}

func (r *permissionRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error) {
	var perms []models.Permission
	var total int64

	query := db.WithContext(ctx).Model(&models.Permission{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	err := query.Offset(offset).Limit(perPage).Find(&perms).Error
	if err != nil {
		return nil, 0, err
	}

	return perms, total, nil
}

func (r *permissionRepository) FindByIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error) {
	var perms []models.Permission
	err := db.WithContext(ctx).Find(&perms, "id IN ?", ids).Error
	if err != nil {
		return nil, err
	}
	return perms, nil
}
