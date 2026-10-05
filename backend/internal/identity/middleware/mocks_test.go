package middleware

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/services"
	"gorm.io/gorm"
)

// MockTenantRepository mocks repositories.TenantRepository
type MockTenantRepository struct {
	FindBySlugFunc func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error)
}

func (m *MockTenantRepository) Create(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
	return nil
}
func (m *MockTenantRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error) {
	return nil, nil
}
func (m *MockTenantRepository) FindBySlug(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
	if m.FindBySlugFunc != nil {
		return m.FindBySlugFunc(ctx, db, slug)
	}
	return nil, nil
}
func (m *MockTenantRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error) {
	return nil, 0, nil
}
func (m *MockTenantRepository) Update(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
	return nil
}
func (m *MockTenantRepository) Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	return nil
}

// MockTokenService mocks services.TokenService
type MockTokenService struct {
	ValidateAccessTokenFunc func(tokenStr string) (*services.UserClaims, error)
}

func (m *MockTokenService) GenerateAccessToken(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error) {
	return "", time.Time{}, nil
}
func (m *MockTokenService) ValidateAccessToken(tokenStr string) (*services.UserClaims, error) {
	if m.ValidateAccessTokenFunc != nil {
		return m.ValidateAccessTokenFunc(tokenStr)
	}
	return nil, nil
}
func (m *MockTokenService) GenerateOpaqueToken() (string, string, error) {
	return "", "", nil
}
func (m *MockTokenService) HashOpaqueToken(token string) string {
	return ""
}

// MockSessionService mocks services.SessionService
type MockSessionService struct {
	ValidateSessionFunc func(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error)
}

func (m *MockSessionService) CreateSession(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error) {
	return nil, nil
}
func (m *MockSessionService) ValidateSession(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error) {
	if m.ValidateSessionFunc != nil {
		return m.ValidateSessionFunc(ctx, db, sessionID)
	}
	return false, nil
}
func (m *MockSessionService) RevokeSession(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error {
	return nil
}
func (m *MockSessionService) RevokeAllForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	return nil
}

// MockUserRoleRepository mocks repositories.UserRoleRepository
type MockUserRoleRepository struct {
	FindByUserIDFunc func(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error)
}

func (m *MockUserRoleRepository) Create(ctx context.Context, tx *gorm.DB, userRole *models.UserRole) error {
	return nil
}
func (m *MockUserRoleRepository) FindByUserID(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error) {
	if m.FindByUserIDFunc != nil {
		return m.FindByUserIDFunc(ctx, db, tenantID, userID)
	}
	return nil, nil
}
func (m *MockUserRoleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error {
	return nil
}
func (m *MockUserRoleRepository) DeleteByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	return nil
}

// MockRolePermissionRepository mocks repositories.RolePermissionRepository
type MockRolePermissionRepository struct {
	FindByRoleIDFunc func(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error)
}

func (m *MockRolePermissionRepository) Create(ctx context.Context, tx *gorm.DB, rolePermission *models.RolePermission) error {
	return nil
}
func (m *MockRolePermissionRepository) FindByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error) {
	if m.FindByRoleIDFunc != nil {
		return m.FindByRoleIDFunc(ctx, db, tenantID, roleID)
	}
	return nil, nil
}
func (m *MockRolePermissionRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error {
	return nil
}
func (m *MockRolePermissionRepository) DeleteByRoleID(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error {
	return nil
}

// MockAuditService mocks services.AuditService
type MockAuditService struct {
	LogFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

func (m *MockAuditService) Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error {
	if m.LogFunc != nil {
		return m.LogFunc(ctx, tx, tenantID, userID, action, resource, resourceID, metadata, ipAddress, userAgent)
	}
	return nil
}
