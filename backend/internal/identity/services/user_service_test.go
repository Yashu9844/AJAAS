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

func TestUserService_CreateUser(t *testing.T) {
	userRepo := &MockUserRepository{}
	roleRepo := &MockRoleRepository{}
	userRoleRepo := &MockUserRoleRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewUserService(userRepo, roleRepo, userRoleRepo, nil, nil, publisher, auditSvc)
	ctx := context.Background()
	tenantID := uuid.New()

	userRepo.FindByEmailFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, email string) (*models.User, error) {
		if email == "duplicate@example.com" {
			return &models.User{Email: email}, nil
		}
		return nil, nil
	}

	userRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, user *models.User) error {
		user.ID = uuid.New()
		return nil
	}

	roleID := uuid.New()
	roleRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*models.Role, error) {
		if rid == roleID {
			role := &models.Role{Name: "admin"}
			role.ID = rid
			return role, nil
		}
		return nil, nil
	}

	userRoleRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error {
		return nil
	}

	// 1. Success case
	req := dto.CreateUserRequest{
		Email:     "new@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
		RoleIDs:   []string{roleID.String()},
	}

	resp, err := svc.CreateUser(ctx, nil, tenantID, req, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Email != "new@example.com" || resp.Status != "active" {
		t.Errorf("unexpected response: %+v", resp)
	}

	// 2. Duplicate user email
	req.Email = "duplicate@example.com"
	_, err = svc.CreateUser(ctx, nil, tenantID, req, uuid.New())
	if !errors.Is(err, sharedErrors.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}

	// 3. Role not found
	req.Email = "new2@example.com"
	req.RoleIDs = []string{uuid.New().String()}
	_, err = svc.CreateUser(ctx, nil, tenantID, req, uuid.New())
	if err == nil {
		t.Fatal("expected role not found error, got nil")
	}
}

func TestUserService_InviteUser(t *testing.T) {
	userRepo := &MockUserRepository{}
	roleRepo := &MockRoleRepository{}
	userRoleRepo := &MockUserRoleRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewUserService(userRepo, roleRepo, userRoleRepo, nil, nil, publisher, auditSvc)
	ctx := context.Background()
	tenantID := uuid.New()

	userRepo.FindByEmailFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, email string) (*models.User, error) {
		return nil, nil
	}
	userRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, user *models.User) error {
		user.ID = uuid.New()
		return nil
	}

	req := dto.InviteUserRequest{
		Email:     "invitee@example.com",
		FirstName: "Jane",
		LastName:  "Smith",
	}

	resp, err := svc.InviteUser(ctx, nil, tenantID, req, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "invited" {
		t.Errorf("expected status 'invited', got %s", resp.Status)
	}
}

func TestUserService_GetByID(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := NewUserService(userRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		if uid == userID {
			return &models.User{Email: "test@example.com"}, nil
		}
		return nil, nil
	}

	resp, err := svc.GetByID(ctx, nil, tenantID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Email != "test@example.com" {
		t.Errorf("expected test@example.com, got %s", resp.Email)
	}

	_, err = svc.GetByID(ctx, nil, tenantID, uuid.New())
	if !errors.Is(err, sharedErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUserService_ListUsers(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := NewUserService(userRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()

	userRepo.FindAllFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, page, perPage int) ([]models.User, int64, error) {
		return []models.User{{Email: "test@example.com"}}, 1, nil
	}

	resp, err := svc.ListUsers(ctx, nil, tenantID, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 || resp.Meta.TotalItems != 1 {
		t.Errorf("unexpected list response: %+v", resp)
	}
}

func TestUserService_UpdateUser(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := NewUserService(userRepo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		return &models.User{FirstName: "Old"}, nil
	}
	userRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, user *models.User) error {
		return nil
	}

	newFirstName := "New"
	resp, err := svc.UpdateUser(ctx, nil, tenantID, userID, dto.UpdateUserRequest{FirstName: &newFirstName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FirstName != "New" {
		t.Errorf("expected first name New, got %s", resp.FirstName)
	}
}

func TestUserService_DeactivateUser(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	tokenRepo := &MockRefreshTokenRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewUserService(userRepo, nil, nil, sessionRepo, tokenRepo, publisher, auditSvc)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		return &models.User{Status: "active"}, nil
	}
	userRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, user *models.User) error {
		return nil
	}
	sessionRepo.RevokeAllByUserIDFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}
	tokenRepo.RevokeAllByUserIDFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}

	err := svc.DeactivateUser(ctx, nil, tenantID, userID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deactivate already inactive
	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		return &models.User{Status: "inactive"}, nil
	}
	err = svc.DeactivateUser(ctx, nil, tenantID, userID, uuid.New())
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}
