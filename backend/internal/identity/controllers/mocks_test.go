package controllers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"gorm.io/gorm"
)

// MockTenantService stubs services.TenantService
type MockTenantService struct {
	CreateTenantFunc    func(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, correlationID uuid.UUID) (*dto.TenantResponse, error)
	GetTenantByIDFunc   func(ctx context.Context, db *gorm.DB, id uuid.UUID) (*dto.TenantResponse, error)
	GetTenantBySlugFunc func(ctx context.Context, db *gorm.DB, slug string) (*dto.TenantResponse, error)
	ListTenantsFunc     func(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error)
	UpdateTenantFunc    func(ctx context.Context, tx *gorm.DB, id uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error)
	ActivateTenantFunc  func(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)
	SuspendTenantFunc   func(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)
}

func (m *MockTenantService) CreateTenant(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, correlationID uuid.UUID) (*dto.TenantResponse, error) {
	if m.CreateTenantFunc != nil {
		return m.CreateTenantFunc(ctx, tx, req, correlationID)
	}
	return nil, nil
}

func (m *MockTenantService) GetTenantByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*dto.TenantResponse, error) {
	if m.GetTenantByIDFunc != nil {
		return m.GetTenantByIDFunc(ctx, db, id)
	}
	return nil, nil
}

func (m *MockTenantService) GetTenantBySlug(ctx context.Context, db *gorm.DB, slug string) (*dto.TenantResponse, error) {
	if m.GetTenantBySlugFunc != nil {
		return m.GetTenantBySlugFunc(ctx, db, slug)
	}
	return nil, nil
}

func (m *MockTenantService) ListTenants(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error) {
	if m.ListTenantsFunc != nil {
		return m.ListTenantsFunc(ctx, db, page, perPage)
	}
	return nil, nil
}

func (m *MockTenantService) UpdateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error) {
	if m.UpdateTenantFunc != nil {
		return m.UpdateTenantFunc(ctx, tx, id, req)
	}
	return nil, nil
}

func (m *MockTenantService) ActivateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error) {
	if m.ActivateTenantFunc != nil {
		return m.ActivateTenantFunc(ctx, tx, id, correlationID)
	}
	return nil, nil
}

func (m *MockTenantService) SuspendTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error) {
	if m.SuspendTenantFunc != nil {
		return m.SuspendTenantFunc(ctx, tx, id, correlationID)
	}
	return nil, nil
}

// MockAuthService stubs services.AuthService
type MockAuthService struct {
	LoginFunc          func(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ipAddress, userAgent string, correlationID uuid.UUID) (*dto.LoginResponse, error)
	LogoutFunc         func(ctx context.Context, tx *gorm.DB, tenantID, userID, sessionID uuid.UUID, correlationID uuid.UUID) error
	RefreshTokenFunc   func(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, correlationID uuid.UUID) (*dto.RefreshTokenResponse, error)
	ForgotPasswordFunc func(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, correlationID uuid.UUID) error
	ResetPasswordFunc  func(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, correlationID uuid.UUID) error
}

func (m *MockAuthService) Login(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ipAddress, userAgent string, correlationID uuid.UUID) (*dto.LoginResponse, error) {
	if m.LoginFunc != nil {
		return m.LoginFunc(ctx, tx, req, ipAddress, userAgent, correlationID)
	}
	return nil, nil
}

func (m *MockAuthService) Logout(ctx context.Context, tx *gorm.DB, tenantID, userID, sessionID uuid.UUID, correlationID uuid.UUID) error {
	if m.LogoutFunc != nil {
		return m.LogoutFunc(ctx, tx, tenantID, userID, sessionID, correlationID)
	}
	return nil
}

func (m *MockAuthService) RefreshToken(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, correlationID uuid.UUID) (*dto.RefreshTokenResponse, error) {
	if m.RefreshTokenFunc != nil {
		return m.RefreshTokenFunc(ctx, tx, req, correlationID)
	}
	return nil, nil
}

func (m *MockAuthService) ForgotPassword(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, correlationID uuid.UUID) error {
	if m.ForgotPasswordFunc != nil {
		return m.ForgotPasswordFunc(ctx, tx, req, correlationID)
	}
	return nil
}

func (m *MockAuthService) ResetPassword(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, correlationID uuid.UUID) error {
	if m.ResetPasswordFunc != nil {
		return m.ResetPasswordFunc(ctx, tx, req, correlationID)
	}
	return nil
}

// MockUserService stubs services.UserService
type MockUserService struct {
	CreateUserFunc     func(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)
	InviteUserFunc     func(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.InviteUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)
	GetByIDFunc        func(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)
	ListUsersFunc      func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error)
	UpdateUserFunc     func(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error)
	DeactivateUserFunc func(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error
}

func (m *MockUserService) CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, tx, tenantID, req, correlationID)
	}
	return nil, nil
}

func (m *MockUserService) InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.InviteUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error) {
	if m.InviteUserFunc != nil {
		return m.InviteUserFunc(ctx, tx, tenantID, req, correlationID)
	}
	return nil, nil
}

func (m *MockUserService) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, db, tenantID, id)
	}
	return nil, nil
}

func (m *MockUserService) ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error) {
	if m.ListUsersFunc != nil {
		return m.ListUsersFunc(ctx, db, tenantID, page, perPage)
	}
	return nil, nil
}

func (m *MockUserService) UpdateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error) {
	if m.UpdateUserFunc != nil {
		return m.UpdateUserFunc(ctx, tx, tenantID, id, req)
	}
	return nil, nil
}

func (m *MockUserService) DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error {
	if m.DeactivateUserFunc != nil {
		return m.DeactivateUserFunc(ctx, tx, tenantID, id, correlationID)
	}
	return nil
}

// MockRoleService stubs services.RoleService
type MockRoleService struct {
	CreateRoleFunc        func(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error)
	GetRoleByIDFunc       func(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error)
	ListRolesFunc         func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error)
	UpdateRoleFunc        func(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error)
	DeleteRoleFunc        func(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	AssignPermissionsFunc func(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error
	AssignRolesToUserFunc func(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error
}

func (m *MockRoleService) CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error) {
	if m.CreateRoleFunc != nil {
		return m.CreateRoleFunc(ctx, tx, tenantID, req)
	}
	return nil, nil
}

func (m *MockRoleService) GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error) {
	if m.GetRoleByIDFunc != nil {
		return m.GetRoleByIDFunc(ctx, db, tenantID, id)
	}
	return nil, nil
}

func (m *MockRoleService) ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error) {
	if m.ListRolesFunc != nil {
		return m.ListRolesFunc(ctx, db, tenantID, page, perPage)
	}
	return nil, nil
}

func (m *MockRoleService) UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error) {
	if m.UpdateRoleFunc != nil {
		return m.UpdateRoleFunc(ctx, tx, tenantID, id, req)
	}
	return nil, nil
}

func (m *MockRoleService) DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	if m.DeleteRoleFunc != nil {
		return m.DeleteRoleFunc(ctx, tx, tenantID, id)
	}
	return nil
}

func (m *MockRoleService) AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error {
	if m.AssignPermissionsFunc != nil {
		return m.AssignPermissionsFunc(ctx, tx, tenantID, roleID, req, correlationID)
	}
	return nil
}

func (m *MockRoleService) AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error {
	if m.AssignRolesToUserFunc != nil {
		return m.AssignRolesToUserFunc(ctx, tx, tenantID, userID, req, correlationID)
	}
	return nil
}

// MockPermissionService stubs services.PermissionService
type MockPermissionService struct {
	ListPermissionsFunc func(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error)
}

func (m *MockPermissionService) ListPermissions(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error) {
	if m.ListPermissionsFunc != nil {
		return m.ListPermissionsFunc(ctx, db, page, perPage)
	}
	return nil, nil
}

var _ services.TenantService = (*MockTenantService)(nil)
var _ services.AuthService = (*MockAuthService)(nil)
var _ services.UserService = (*MockUserService)(nil)
var _ services.RoleService = (*MockRoleService)(nil)
var _ services.PermissionService = (*MockPermissionService)(nil)
