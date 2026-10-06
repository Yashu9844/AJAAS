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

func (r *userRoleRepository) FindByUserIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, userIDs []uuid.UUID) ([]models.UserRole, error) {
	var userRoles []models.UserRole
	if len(userIDs) == 0 {
		return userRoles, nil
	}
	err := db.WithContext(ctx).
		Preload("Role").
		Where("tenant_id = ? AND user_id IN ?", tenantID, userIDs).
		Find(&userRoles).Error
	return userRoles, err
}

func (r *userRoleRepository) CountByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) (int64, error) {
	var total int64
	err := db.WithContext(ctx).Model(&models.UserRole{}).Where("tenant_id = ? AND role_id = ?", tenantID, roleID).Count(&total).Error
	return total, err
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

func (r *rolePermissionRepository) FindByRoleIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) ([]models.RolePermission, error) {
	var rolePerms []models.RolePermission
	if len(roleIDs) == 0 {
		return rolePerms, nil
	}
	err := db.WithContext(ctx).
		Preload("Permission").
		Where("tenant_id = ? AND role_id IN ?", tenantID, roleIDs).
		Find(&rolePerms).Error
	return rolePerms, err
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
