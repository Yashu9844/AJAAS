package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// TenantRepository handles CRUD for Tenants.
type TenantRepository interface {
	Create(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error
	FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error)
	FindBySlug(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error)
	FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error)
	Update(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error
	Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
}

// UserRepository handles CRUD for Users scoped by tenant.
type UserRepository interface {
	Create(ctx context.Context, tx *gorm.DB, user *models.User) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.User, error)
	FindByEmail(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, email string) (*models.User, error)
	FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error)
	Update(ctx context.Context, tx *gorm.DB, user *models.User) error
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

// RoleRepository handles CRUD for Roles.
type RoleRepository interface {
	Create(ctx context.Context, tx *gorm.DB, role *models.Role) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error)
	FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Role, error)
	FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Role, int64, error)
	Update(ctx context.Context, tx *gorm.DB, role *models.Role) error
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

// PermissionRepository handles CRUD for global Permissions.
type PermissionRepository interface {
	Create(ctx context.Context, tx *gorm.DB, permission *models.Permission) error
	FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Permission, error)
	FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error)
	FindByIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error)
}

// UserRoleRepository handles role assignments to users.
type UserRoleRepository interface {
	Create(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error
	FindByUserID(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error)
	Delete(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error
	DeleteByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

// RolePermissionRepository handles permission assignments to roles.
type RolePermissionRepository interface {
	Create(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error
	FindByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error)
	Delete(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error
	DeleteByRoleID(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error
}

// SessionRepository tracks login sessions.
type SessionRepository interface {
	Create(ctx context.Context, tx *gorm.DB, session *models.Session) error
	FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error)
	RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
	RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

// RefreshTokenRepository tracks refresh tokens.
type RefreshTokenRepository interface {
	Create(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error
	FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error)
	RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
	RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

// PasswordResetTokenRepository tracks password resets.
type PasswordResetTokenRepository interface {
	Create(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error
	FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error)
	MarkAsUsed(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
}

// MFAConfigRepository configures MFA.
type MFAConfigRepository interface {
	Create(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error
	FindByUserIDAndType(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, mfaType string) (*models.MFAConfig, error)
	Update(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error
}

// TenantSettingsRepository handles tenant config key-values.
type TenantSettingsRepository interface {
	Upsert(ctx context.Context, tx *gorm.DB, setting *models.TenantSettings) error
	FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.TenantSettings, error)
	FindByKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.TenantSettings, error)
}

// AuditLogRepository records security logs (append-only).
type AuditLogRepository interface {
	Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error
	FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error)
}
