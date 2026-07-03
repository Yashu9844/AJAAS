package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

func TestRoleService_CreateRole(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	rolePermRepo := &MockRolePermissionRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewRoleService(roleRepo, nil, nil, rolePermRepo, nil, publisher, auditSvc)
	ctx := context.Background()
	tenantID := uuid.New()

	roleRepo.FindByNameFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, name string) (*models.Role, error) {
		if name == "duplicate" {
			return &models.Role{Name: name}, nil
		}
		return nil, nil
	}

	roleRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, role *models.Role) error {
		role.ID = uuid.New()
		return nil
	}

	rolePermRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error {
		return nil
	}

	// 1. Success case
	req := dto.CreateRoleRequest{
		Name:          "manager",
		Description:   "Manager role",
		PermissionIDs: []string{uuid.New().String()},
	}

	resp, err := svc.CreateRole(ctx, nil, tenantID, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "manager" || resp.IsSystem {
		t.Errorf("unexpected role created: %+v", resp)
	}

	// 2. Conflict
	req.Name = "duplicate"
	_, err = svc.CreateRole(ctx, nil, tenantID, req)
	if !errors.Is(err, sharedErrors.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestRoleService_GetRoleByID(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	svc := NewRoleService(roleRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()
	roleID := uuid.New()

	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		if rid == roleID {
			return &models.Role{Name: "admin"}, nil
		}
		return nil, nil
	}

	resp, err := svc.GetRoleByID(ctx, nil, tenantID, roleID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "admin" {
		t.Errorf("expected admin, got %s", resp.Name)
	}

	_, err = svc.GetRoleByID(ctx, nil, tenantID, uuid.New())
	if !errors.Is(err, sharedErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRoleService_ListRoles(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	svc := NewRoleService(roleRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()

	roleRepo.FindAllFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, page, perPage int) ([]models.Role, int64, error) {
		return []models.Role{{Name: "admin"}}, 1, nil
	}

	resp, err := svc.ListRoles(ctx, nil, tenantID, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 || resp.Meta.TotalItems != 1 {
		t.Errorf("unexpected list response: %+v", resp)
	}
}

func TestRoleService_UpdateRole(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	svc := NewRoleService(roleRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()
	roleID := uuid.New()

	// 1. Success case
	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		return &models.Role{Name: "Old", IsSystem: false}, nil
	}
	roleRepo.FindByNameFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, name string) (*models.Role, error) {
		return nil, nil
	}
	roleRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, role *models.Role) error {
		return nil
	}

	newName := "New"
	resp, err := svc.UpdateRole(ctx, nil, tenantID, roleID, dto.UpdateRoleRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "New" {
		t.Errorf("expected New, got %s", resp.Name)
	}

	// 2. Reject renaming system roles
	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		return &models.Role{Name: "admin", IsSystem: true}, nil
	}
	_, err = svc.UpdateRole(ctx, nil, tenantID, roleID, dto.UpdateRoleRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected system role rename to be forbidden, got nil")
	}
}

func TestRoleService_DeleteRole(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	svc := NewRoleService(roleRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()
	roleID := uuid.New()

	// 1. Success delete
	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		return &models.Role{Name: "custom", IsSystem: false}, nil
	}
	roleRepo.DeleteFunc = func(ctx context.Context, tx *gorm.DB, tid, rid uuid.UUID) error {
		return nil
	}

	err := svc.DeleteRole(ctx, nil, tenantID, roleID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2. Reject delete system role
	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		return &models.Role{Name: "admin", IsSystem: true}, nil
	}
	err = svc.DeleteRole(ctx, nil, tenantID, roleID)
	if err == nil {
		t.Fatal("expected system role delete to be forbidden, got nil")
	}
}

func TestRoleService_AssignPermissions(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	permRepo := &MockPermissionRepository{}
	rolePermRepo := &MockRolePermissionRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewRoleService(roleRepo, permRepo, nil, rolePermRepo, nil, publisher, auditSvc)
	ctx := context.Background()
	tenantID := uuid.New()
	roleID := uuid.New()
	permID := uuid.New()

	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		return &models.Role{Name: "admin"}, nil
	}

	permRepo.FindByIDsFunc = func(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error) {
		return []models.Permission{{}}, nil
	}

	rolePermRepo.FindByRoleIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) ([]models.RolePermission, error) {
		return nil, nil // none assigned yet
	}

	rolePermRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error {
		return nil
	}

	req := dto.AssignPermissionsRequest{
		PermissionIDs: []string{permID.String()},
	}

	err := svc.AssignPermissions(ctx, nil, tenantID, roleID, req, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoleService_AssignRolesToUser(t *testing.T) {
	roleRepo := &MockRoleRepository{}
	userRepo := &MockUserRepository{}
	userRoleRepo := &MockUserRoleRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewRoleService(roleRepo, nil, userRoleRepo, nil, userRepo, publisher, auditSvc)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()
	roleID := uuid.New()

	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		return &models.User{}, nil
	}

	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		return &models.Role{Name: "member"}, nil
	}

	userRoleRepo.FindByUserIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) ([]models.UserRole, error) {
		return nil, nil // no roles assigned yet
	}

	userRoleRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error {
		return nil
	}

	req := dto.AssignRoleRequest{
		RoleIDs: []string{roleID.String()},
	}

	err := svc.AssignRolesToUser(ctx, nil, tenantID, userID, req, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
