package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AccessRepository persists the access-model rows beyond plain role/permission CRUD (docs/ARCHITECTURE.md §17).
type AccessRepository interface {
	AccessVersion(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int64, error)
	TenantRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Role, error)
	RoleByTemplateKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.Role, error)
	SetRolePermissionScope(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID, scope string) error
	UserGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionGrant, error)
	UpsertUserGrant(ctx context.Context, tx *gorm.DB, g *models.UserPermissionGrant) error
	DeleteUserGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error)
	UserBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionBlock, error)
	UpsertUserBlock(ctx context.Context, tx *gorm.DB, b *models.UserPermissionBlock) error
	DeleteUserBlock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error)
	AutomaticAssignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.UserRole, error)
	// UpsertPermission inserts a catalogue entry or refreshes its description, module and sensitivity.
	UpsertPermission(ctx context.Context, db *gorm.DB, p *models.Permission) error
	AllPermissions(ctx context.Context, db *gorm.DB) ([]models.Permission, error)
	TenantIDs(ctx context.Context, db *gorm.DB, activeOnly bool) ([]uuid.UUID, error)
}

type accessRepository struct{}

// NewAccessRepository returns an AccessRepository.
func NewAccessRepository() AccessRepository { return &accessRepository{} }

// bumpAccessVersion invalidates cached effective access for the tenant; every access write calls it in its tx.
func bumpAccessVersion(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("UPDATE tenants SET access_version = access_version + 1 WHERE id = ?", tenantID).Error
}

func (r *accessRepository) AccessVersion(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int64, error) {
	var v int64
	err := db.WithContext(ctx).Raw("SELECT access_version FROM tenants WHERE id = ?", tenantID).Scan(&v).Error
	return v, err
}

func (r *accessRepository) TenantRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Role, error) {
	var roles []models.Role
	err := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("name").Find(&roles).Error
	return roles, err
}

func (r *accessRepository) RoleByTemplateKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.Role, error) {
	var role models.Role
	err := db.WithContext(ctx).First(&role, "tenant_id = ? AND template_key = ?", tenantID, key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *accessRepository) SetRolePermissionScope(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID, scope string) error {
	err := tx.WithContext(ctx).Model(&models.RolePermission{}).
		Where("tenant_id = ? AND role_id = ? AND permission_id = ?", tenantID, roleID, permissionID).
		Updates(map[string]interface{}{"scope": scope, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
	if err != nil {
		return err
	}
	return bumpAccessVersion(ctx, tx, tenantID)
}

func (r *accessRepository) UserGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionGrant, error) {
	var rows []models.UserPermissionGrant
	err := db.WithContext(ctx).Preload("Permission").Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&rows).Error
	return rows, err
}

func (r *accessRepository) UpsertUserGrant(ctx context.Context, tx *gorm.DB, g *models.UserPermissionGrant) error {
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "permission_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope", "expires_at", "reason", "granted_by", "updated_at"}),
	}).Create(g).Error
	if err != nil {
		return err
	}
	return bumpAccessVersion(ctx, tx, g.TenantID)
}

func (r *accessRepository) DeleteUserGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error) {
	return deleteAccessRow(ctx, tx, &models.UserPermissionGrant{}, tenantID, userID, permissionID)
}

func (r *accessRepository) UserBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionBlock, error) {
	var rows []models.UserPermissionBlock
	err := db.WithContext(ctx).Preload("Permission").Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&rows).Error
	return rows, err
}

func (r *accessRepository) UpsertUserBlock(ctx context.Context, tx *gorm.DB, b *models.UserPermissionBlock) error {
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "permission_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"reason", "created_by", "updated_at"}),
	}).Create(b).Error
	if err != nil {
		return err
	}
	return bumpAccessVersion(ctx, tx, b.TenantID)
}

func (r *accessRepository) DeleteUserBlock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error) {
	return deleteAccessRow(ctx, tx, &models.UserPermissionBlock{}, tenantID, userID, permissionID)
}

func (r *accessRepository) AutomaticAssignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.UserRole, error) {
	var rows []models.UserRole
	err := db.WithContext(ctx).Where("tenant_id = ? AND source = ?", tenantID, "automatic").Find(&rows).Error
	return rows, err
}

func (r *accessRepository) UpsertPermission(ctx context.Context, db *gorm.DB, p *models.Permission) error {
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "resource"}, {Name: "action"}},
		DoUpdates: clause.AssignmentColumns([]string{"description", "module", "is_sensitive", "updated_at"}),
	}).Create(p).Error
}

func (r *accessRepository) AllPermissions(ctx context.Context, db *gorm.DB) ([]models.Permission, error) {
	var perms []models.Permission
	err := db.WithContext(ctx).Order("resource, action").Find(&perms).Error
	return perms, err
}

func (r *accessRepository) TenantIDs(ctx context.Context, db *gorm.DB, activeOnly bool) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	q := db.WithContext(ctx).Model(&models.Tenant{})
	if activeOnly {
		q = q.Where("status = ?", "active")
	}
	err := q.Pluck("id", &ids).Error
	return ids, err
}

func deleteAccessRow(ctx context.Context, tx *gorm.DB, model interface{}, tenantID, userID, permissionID uuid.UUID) (bool, error) {
	res := tx.WithContext(ctx).Delete(model, "tenant_id = ? AND user_id = ? AND permission_id = ?", tenantID, userID, permissionID)
	if res.Error != nil || res.RowsAffected == 0 {
		return false, res.Error
	}
	return true, bumpAccessVersion(ctx, tx, tenantID)
}
