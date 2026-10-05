package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type userRoleRepository struct{}

// NewUserRoleRepository returns a UserRoleRepository.
func NewUserRoleRepository() UserRoleRepository {
	return &userRoleRepository{}
}

func (r *userRoleRepository) Create(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error {
	return tx.WithContext(ctx).Create(ur).Error
}

func (r *userRoleRepository) FindByUserID(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error) {
	var userRoles []models.UserRole
	err := db.WithContext(ctx).
		Preload("Role").
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Find(&userRoles).Error
	if err != nil {
		return nil, err
	}
	return userRoles, nil
}

func (r *userRoleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error {
	return tx.WithContext(ctx).
		Delete(&models.UserRole{}, "tenant_id = ? AND user_id = ? AND role_id = ?", tenantID, userID, roleID).
		Error
}

func (r *userRoleRepository) DeleteByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	return tx.WithContext(ctx).
		Delete(&models.UserRole{}, "tenant_id = ? AND user_id = ?", tenantID, userID).
		Error
}

type rolePermissionRepository struct{}

// NewRolePermissionRepository returns a RolePermissionRepository.
func NewRolePermissionRepository() RolePermissionRepository {
	return &rolePermissionRepository{}
}

func (r *rolePermissionRepository) Create(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error {
	return tx.WithContext(ctx).Create(rp).Error
}

func (r *rolePermissionRepository) FindByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error) {
	var rolePerms []models.RolePermission
	err := db.WithContext(ctx).
		Preload("Permission").
		Where("tenant_id = ? AND role_id = ?", tenantID, roleID).
		Find(&rolePerms).Error
	if err != nil {
		return nil, err
	}
	return rolePerms, nil
}

func (r *rolePermissionRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error {
	return tx.WithContext(ctx).
		Delete(&models.RolePermission{}, "tenant_id = ? AND role_id = ? AND permission_id = ?", tenantID, roleID, permissionID).
		Error
}

func (r *rolePermissionRepository) DeleteByRoleID(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error {
	return tx.WithContext(ctx).
		Delete(&models.RolePermission{}, "tenant_id = ? AND role_id = ?", tenantID, roleID).
		Error
}
