package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// MockEventPublisher stubs queue.EventPublisher
type MockEventPublisher struct {
	PublishFunc func(ctx context.Context, exchange string, routingKey string, event interface{}) error
}

func (m *MockEventPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error {
	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, exchange, routingKey, event)
	}
	return nil
}

// MockTenantRepository stubs repositories.TenantRepository
type MockTenantRepository struct {
	CreateFunc     func(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error
	FindByIDFunc   func(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error)
	FindBySlugFunc func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error)
	FindAllFunc    func(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error)
	UpdateFunc     func(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error
	DeleteFunc     func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
}

func (m *MockTenantRepository) Create(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, tenant)
	}
	return nil
}

func (m *MockTenantRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, db, id)
	}
	return nil, nil
}

func (m *MockTenantRepository) FindBySlug(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
	if m.FindBySlugFunc != nil {
		return m.FindBySlugFunc(ctx, db, slug)
	}
	return nil, nil
}

func (m *MockTenantRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error) {
	if m.FindAllFunc != nil {
		return m.FindAllFunc(ctx, db, page, perPage)
	}
	return nil, 0, nil
}

func (m *MockTenantRepository) Update(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, tx, tenant)
	}
	return nil
}

func (m *MockTenantRepository) Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, tx, id)
	}
	return nil
}

// MockUserRepository stubs repositories.UserRepository
type MockUserRepository struct {
	CreateFunc      func(ctx context.Context, tx *gorm.DB, user *models.User) error
	FindByIDFunc    func(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.User, error)
	FindByEmailFunc func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, email string) (*models.User, error)
	FindAllFunc     func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error)
	UpdateFunc      func(ctx context.Context, tx *gorm.DB, user *models.User) error
	DeleteFunc      func(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

func (m *MockUserRepository) Create(ctx context.Context, tx *gorm.DB, user *models.User) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, user)
	}
	return nil
}

func (m *MockUserRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.User, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, db, tenantID, id)
	}
	return nil, nil
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, email string) (*models.User, error) {
	if m.FindByEmailFunc != nil {
		return m.FindByEmailFunc(ctx, db, tenantID, email)
	}
	return nil, nil
}

func (m *MockUserRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error) {
	if m.FindAllFunc != nil {
		return m.FindAllFunc(ctx, db, tenantID, page, perPage)
	}
	return nil, 0, nil
}

func (m *MockUserRepository) Update(ctx context.Context, tx *gorm.DB, user *models.User) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, tx, user)
	}
	return nil
}

func (m *MockUserRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, tx, tenantID, id)
	}
	return nil
}

// MockRoleRepository stubs repositories.RoleRepository
type MockRoleRepository struct {
	CreateFunc     func(ctx context.Context, tx *gorm.DB, role *models.Role) error
	FindByIDFunc   func(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error)
	FindByNameFunc func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Role, error)
	FindAllFunc    func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Role, int64, error)
	UpdateFunc     func(ctx context.Context, tx *gorm.DB, role *models.Role) error
	DeleteFunc     func(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

func (m *MockRoleRepository) Create(ctx context.Context, tx *gorm.DB, role *models.Role) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, role)
	}
	return nil
}

func (m *MockRoleRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, db, tenantID, id)
	}
	return nil, nil
}

func (m *MockRoleRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Role, error) {
	if m.FindByNameFunc != nil {
		return m.FindByNameFunc(ctx, db, tenantID, name)
	}
	return nil, nil
}

func (m *MockRoleRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Role, int64, error) {
	if m.FindAllFunc != nil {
		return m.FindAllFunc(ctx, db, tenantID, page, perPage)
	}
	return nil, 0, nil
}

func (m *MockRoleRepository) Update(ctx context.Context, tx *gorm.DB, role *models.Role) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, tx, role)
	}
	return nil
}

func (m *MockRoleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, tx, tenantID, id)
	}
	return nil
}

// MockPermissionRepository stubs repositories.PermissionRepository
type MockPermissionRepository struct {
	CreateFunc    func(ctx context.Context, tx *gorm.DB, permission *models.Permission) error
	FindByIDFunc  func(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Permission, error)
	FindAllFunc   func(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error)
	FindByIDsFunc func(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error)
}

func (m *MockPermissionRepository) Create(ctx context.Context, tx *gorm.DB, permission *models.Permission) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, permission)
	}
	return nil
}

func (m *MockPermissionRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Permission, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, db, id)
	}
	return nil, nil
}

func (m *MockPermissionRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error) {
	if m.FindAllFunc != nil {
		return m.FindAllFunc(ctx, db, page, perPage)
	}
	return nil, 0, nil
}

func (m *MockPermissionRepository) FindByIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error) {
	if m.FindByIDsFunc != nil {
		return m.FindByIDsFunc(ctx, db, ids)
	}
	return nil, nil
}

// MockUserRoleRepository stubs repositories.UserRoleRepository
type MockUserRoleRepository struct {
	CreateFunc         func(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error
	FindByUserIDFunc   func(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error)
	DeleteFunc         func(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error
	DeleteByUserIDFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

func (m *MockUserRoleRepository) Create(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, ur)
	}
	return nil
}

func (m *MockUserRoleRepository) FindByUserID(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error) {
	if m.FindByUserIDFunc != nil {
		return m.FindByUserIDFunc(ctx, db, tenantID, userID)
	}
	return nil, nil
}

func (m *MockUserRoleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, tx, tenantID, userID, roleID)
	}
	return nil
}

func (m *MockUserRoleRepository) DeleteByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	if m.DeleteByUserIDFunc != nil {
		return m.DeleteByUserIDFunc(ctx, tx, tenantID, userID)
	}
	return nil
}

// MockRolePermissionRepository stubs repositories.RolePermissionRepository
type MockRolePermissionRepository struct {
	CreateFunc         func(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error
	FindByRoleIDFunc   func(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error)
	DeleteFunc         func(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error
	DeleteByRoleIDFunc func(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error
}

func (m *MockRolePermissionRepository) Create(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, rp)
	}
	return nil
}

func (m *MockRolePermissionRepository) FindByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error) {
	if m.FindByRoleIDFunc != nil {
		return m.FindByRoleIDFunc(ctx, db, tenantID, roleID)
	}
	return nil, nil
}

func (m *MockRolePermissionRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, tx, tenantID, roleID, permissionID)
	}
	return nil
}

func (m *MockRolePermissionRepository) DeleteByRoleID(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error {
	if m.DeleteByRoleIDFunc != nil {
		return m.DeleteByRoleIDFunc(ctx, tx, tenantID, roleID)
	}
	return nil
}

// MockSessionRepository stubs repositories.SessionRepository
type MockSessionRepository struct {
	CreateFunc            func(ctx context.Context, tx *gorm.DB, session *models.Session) error
	FindByIDFunc          func(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error)
	RevokeByIDFunc        func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
	RevokeAllByUserIDFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

func (m *MockSessionRepository) Create(ctx context.Context, tx *gorm.DB, session *models.Session) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, session)
	}
	return nil
}

func (m *MockSessionRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, db, id)
	}
	return nil, nil
}

func (m *MockSessionRepository) RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	if m.RevokeByIDFunc != nil {
		return m.RevokeByIDFunc(ctx, tx, id)
	}
	return nil
}

func (m *MockSessionRepository) RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	if m.RevokeAllByUserIDFunc != nil {
		return m.RevokeAllByUserIDFunc(ctx, tx, tenantID, userID)
	}
	return nil
}

// MockRefreshTokenRepository stubs repositories.RefreshTokenRepository
type MockRefreshTokenRepository struct {
	CreateFunc            func(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error
	FindByTokenHashFunc   func(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error)
	RevokeByIDFunc        func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
	RevokeAllByUserIDFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

func (m *MockRefreshTokenRepository) Create(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, rt)
	}
	return nil
}

func (m *MockRefreshTokenRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error) {
	if m.FindByTokenHashFunc != nil {
		return m.FindByTokenHashFunc(ctx, db, hash)
	}
	return nil, nil
}

func (m *MockRefreshTokenRepository) RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	if m.RevokeByIDFunc != nil {
		return m.RevokeByIDFunc(ctx, tx, id)
	}
	return nil
}

func (m *MockRefreshTokenRepository) RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	if m.RevokeAllByUserIDFunc != nil {
		return m.RevokeAllByUserIDFunc(ctx, tx, tenantID, userID)
	}
	return nil
}

// MockPasswordResetTokenRepository stubs repositories.PasswordResetTokenRepository
type MockPasswordResetTokenRepository struct {
	CreateFunc          func(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error
	FindByTokenHashFunc func(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error)
	MarkAsUsedFunc      func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error
}

func (m *MockPasswordResetTokenRepository) Create(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, prt)
	}
	return nil
}

func (m *MockPasswordResetTokenRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error) {
	if m.FindByTokenHashFunc != nil {
		return m.FindByTokenHashFunc(ctx, db, hash)
	}
	return nil, nil
}

func (m *MockPasswordResetTokenRepository) MarkAsUsed(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	if m.MarkAsUsedFunc != nil {
		return m.MarkAsUsedFunc(ctx, tx, id)
	}
	return nil
}

// MockMFAConfigRepository stubs repositories.MFAConfigRepository
type MockMFAConfigRepository struct {
	CreateFunc              func(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error
	FindByUserIDAndTypeFunc func(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, mfaType string) (*models.MFAConfig, error)
	UpdateFunc              func(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error
}

func (m *MockMFAConfigRepository) Create(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, mfa)
	}
	return nil
}

func (m *MockMFAConfigRepository) FindByUserIDAndType(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, mfaType string) (*models.MFAConfig, error) {
	if m.FindByUserIDAndTypeFunc != nil {
		return m.FindByUserIDAndTypeFunc(ctx, db, tenantID, userID, mfaType)
	}
	return nil, nil
}

func (m *MockMFAConfigRepository) Update(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, tx, mfa)
	}
	return nil
}

// MockTenantSettingsRepository stubs repositories.TenantSettingsRepository
type MockTenantSettingsRepository struct {
	UpsertFunc         func(ctx context.Context, tx *gorm.DB, setting *models.TenantSettings) error
	FindByTenantIDFunc func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.TenantSettings, error)
	FindByKeyFunc      func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.TenantSettings, error)
}

func (m *MockTenantSettingsRepository) Upsert(ctx context.Context, tx *gorm.DB, setting *models.TenantSettings) error {
	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, tx, setting)
	}
	return nil
}

func (m *MockTenantSettingsRepository) FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.TenantSettings, error) {
	if m.FindByTenantIDFunc != nil {
		return m.FindByTenantIDFunc(ctx, db, tenantID)
	}
	return nil, nil
}

func (m *MockTenantSettingsRepository) FindByKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.TenantSettings, error) {
	if m.FindByKeyFunc != nil {
		return m.FindByKeyFunc(ctx, db, tenantID, key)
	}
	return nil, nil
}

// MockAuditLogRepository stubs repositories.AuditLogRepository
type MockAuditLogRepository struct {
	CreateFunc         func(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error
	FindByTenantIDFunc func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error)
}

func (m *MockAuditLogRepository) Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, tx, log)
	}
	return nil
}

func (m *MockAuditLogRepository) FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error) {
	if m.FindByTenantIDFunc != nil {
		return m.FindByTenantIDFunc(ctx, db, tenantID, page, perPage)
	}
	return nil, 0, nil
}

// MockAuditService stubs AuditService
type MockAuditService struct {
	LogFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

func (m *MockAuditService) Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error {
	if m.LogFunc != nil {
		return m.LogFunc(ctx, tx, tenantID, userID, action, resource, resourceID, metadata, ipAddress, userAgent)
	}
	return nil
}

// MockTokenService stubs TokenService
type MockTokenService struct {
	GenerateAccessTokenFunc func(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error)
	ValidateAccessTokenFunc func(tokenStr string) (*UserClaims, error)
	GenerateOpaqueTokenFunc func() (string, string, error)
	HashOpaqueTokenFunc     func(token string) string
}

func (m *MockTokenService) GenerateAccessToken(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error) {
	if m.GenerateAccessTokenFunc != nil {
		return m.GenerateAccessTokenFunc(userID, tenantID, email, roles, sessionID)
	}
	return "", time.Time{}, nil
}

func (m *MockTokenService) ValidateAccessToken(tokenStr string) (*UserClaims, error) {
	if m.ValidateAccessTokenFunc != nil {
		return m.ValidateAccessTokenFunc(tokenStr)
	}
	return nil, nil
}

func (m *MockTokenService) GenerateOpaqueToken() (string, string, error) {
	if m.GenerateOpaqueTokenFunc != nil {
		return m.GenerateOpaqueTokenFunc()
	}
	return "", "", nil
}

func (m *MockTokenService) HashOpaqueToken(token string) string {
	if m.HashOpaqueTokenFunc != nil {
		return m.HashOpaqueTokenFunc(token)
	}
	return ""
}

// MockSessionService stubs SessionService
type MockSessionService struct {
	CreateSessionFunc   func(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error)
	ValidateSessionFunc func(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error)
	RevokeSessionFunc   func(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error
	RevokeAllForUserFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

func (m *MockSessionService) CreateSession(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error) {
	if m.CreateSessionFunc != nil {
		return m.CreateSessionFunc(ctx, tx, userID, tenantID, ipAddress, userAgent)
	}
	return nil, nil
}

func (m *MockSessionService) ValidateSession(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error) {
	if m.ValidateSessionFunc != nil {
		return m.ValidateSessionFunc(ctx, db, sessionID)
	}
	return false, nil
}

func (m *MockSessionService) RevokeSession(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error {
	if m.RevokeSessionFunc != nil {
		return m.RevokeSessionFunc(ctx, tx, sessionID)
	}
	return nil
}

func (m *MockSessionService) RevokeAllForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	if m.RevokeAllForUserFunc != nil {
		return m.RevokeAllForUserFunc(ctx, tx, tenantID, userID)
	}
	return nil
}


