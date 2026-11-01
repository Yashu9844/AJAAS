package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResourceRoleRepository stores role assignments on resources (docs/modules/m0-identity-access/resource-roles.md).
type ResourceRoleRepository interface {
	// Assign sets the user role on the resource, replacing any earlier one, and invalidates cached access.
	Assign(ctx context.Context, tx *gorm.DB, a *models.ResourceRoleAssignment) error
	// Revoke removes the user role on the resource and reports whether one existed.
	Revoke(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (bool, error)
	// Find returns the user assignment on the resource (nil when none), with its role.
	Find(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (*models.ResourceRoleAssignment, error)
	// ListByResource returns every assignment on the resource, with roles.
	ListByResource(ctx context.Context, db *gorm.DB, ref access.ResourceRef) ([]models.ResourceRoleAssignment, error)
	// ListForUser returns the user assignments on the given resource ids, with roles.
	ListForUser(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID, ids []uuid.UUID) ([]models.ResourceRoleAssignment, error)
	// CountWithTemplate counts assignments on the resource whose role came from the template.
	CountWithTemplate(ctx context.Context, db *gorm.DB, ref access.ResourceRef, templateKey string) (int64, error)
	// RoleInUse counts assignments of a role anywhere (a role in use cannot be deleted).
	RoleInUse(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) (int64, error)
}

type resourceRoleRepository struct{}

// NewResourceRoleRepository returns a ResourceRoleRepository.
func NewResourceRoleRepository() ResourceRoleRepository { return &resourceRoleRepository{} }

func (r *resourceRoleRepository) Assign(ctx context.Context, tx *gorm.DB, a *models.ResourceRoleAssignment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "resource_type"}, {Name: "resource_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role_id", "source", "assigned_by", "updated_at"}),
	}).Create(a).Error
	if err != nil {
		return err
	}
	return bumpAccessVersion(ctx, tx, a.TenantID)
}

func (r *resourceRoleRepository) Revoke(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (bool, error) {
	res := tx.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND resource_type = ? AND resource_id = ?", ref.TenantID, userID, ref.Type, ref.ID).
		Delete(&models.ResourceRoleAssignment{})
	if res.Error != nil || res.RowsAffected == 0 {
		return false, res.Error
	}
	return true, bumpAccessVersion(ctx, tx, ref.TenantID)
}

func (r *resourceRoleRepository) Find(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (*models.ResourceRoleAssignment, error) {
	var rows []models.ResourceRoleAssignment
	err := db.WithContext(ctx).Preload("Role").
		Where("tenant_id = ? AND user_id = ? AND resource_type = ? AND resource_id = ?", ref.TenantID, userID, ref.Type, ref.ID).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (r *resourceRoleRepository) ListByResource(ctx context.Context, db *gorm.DB, ref access.ResourceRef) ([]models.ResourceRoleAssignment, error) {
	var rows []models.ResourceRoleAssignment
	err := db.WithContext(ctx).Preload("Role").
		Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", ref.TenantID, ref.Type, ref.ID).Find(&rows).Error
	return rows, err
}

func (r *resourceRoleRepository) ListForUser(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID, ids []uuid.UUID) ([]models.ResourceRoleAssignment, error) {
	var rows []models.ResourceRoleAssignment
	if len(ids) == 0 {
		return rows, nil
	}
	err := db.WithContext(ctx).Preload("Role").
		Where("tenant_id = ? AND user_id = ? AND resource_type = ? AND resource_id IN ?", ref.TenantID, userID, ref.Type, ids).Find(&rows).Error
	return rows, err
}

func (r *resourceRoleRepository) CountWithTemplate(ctx context.Context, db *gorm.DB, ref access.ResourceRef, templateKey string) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Table("resource_role_assignments a").Joins("JOIN roles r ON r.id = a.role_id").
		Where("a.tenant_id = ? AND a.resource_type = ? AND a.resource_id = ? AND r.template_key = ?", ref.TenantID, ref.Type, ref.ID, templateKey).Count(&n).Error
	return n, err
}

func (r *resourceRoleRepository) RoleInUse(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.ResourceRoleAssignment{}).Where("tenant_id = ? AND role_id = ?", tenantID, roleID).Count(&n).Error
	return n, err
}
