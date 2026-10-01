package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestAuthService_Login(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	userRepo := &MockUserRepository{}
	userRoleRepo := &MockUserRoleRepository{}
	sessionRepo := &MockSessionRepository{}
	tokenRepo := &MockRefreshTokenRepository{}
	resetRepo := &MockPasswordResetTokenRepository{}
	tokenSvc := &MockTokenService{}
	sessionSvc := &MockSessionService{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewAuthService(tenantRepo, userRepo, userRoleRepo, sessionRepo, tokenRepo, resetRepo, tokenSvc, sessionSvc, publisher, auditSvc)
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()
	pwd := "password123"
	pwdHash, _ := bcrypt.GenerateFromPassword([]byte(pwd), 12)

	tenantRepo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		if slug == "acme" {
			tenant := &models.Tenant{
				Slug:   "acme",
				Status: "active",
			}
			tenant.ID = tenantID
			return tenant, nil
		}
		if slug == "suspended" {
			tenant := &models.Tenant{
				Slug:   "suspended",
				Status: "suspended",
			}
			tenant.ID = tenantID
			return tenant, nil
		}
		return nil, nil
	}

	userRepo.FindByEmailFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, email string) (*models.User, error) {
		if email == "active@example.com" {
			user := &models.User{
				TenantID:     tid,
				Email:        email,
				PasswordHash: string(pwdHash),
				Status:       "active",
			}
			user.ID = userID
			return user, nil
		}
		if email == "inactive@example.com" {
			user := &models.User{
				TenantID:     tid,
				Email:        email,
				PasswordHash: string(pwdHash),
				Status:       "inactive",
			}
			user.ID = userID
			return user, nil
		}
		return nil, nil
	}

	userRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, user *models.User) error {
		return nil
	}

	sessionSvc.CreateSessionFunc = func(ctx context.Context, tx *gorm.DB, uid, tid uuid.UUID, ip, ua string) (*models.Session, error) {
		session := &models.Session{
			UserID:   uid,
			TenantID: tid,
		}
		session.ID = uuid.New()
		return session, nil
	}

	tokenSvc.GenerateOpaqueTokenFunc = func() (string, string, error) {
		return "raw-rt", "hash-rt", nil
	}

	tokenRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error {
		return nil
	}

	userRoleRepo.FindByUserIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) ([]models.UserRole, error) {
		return []models.UserRole{
			{
				Role: &models.Role{
					Name: "member",
				},
			},
		}, nil
	}

	tokenSvc.GenerateAccessTokenFunc = func(uid, tid, email string, roles []string, sid string) (string, time.Time, error) {
		return "signed-jwt", time.Now().Add(15 * time.Minute), nil
	}

	// 1. Success Login
	req := dto.LoginRequest{
		TenantSlug: "acme",
		Email:      "active@example.com",
		Password:   pwd,
	}
	resp, err := svc.Login(ctx, nil, req, "127.0.0.1", "Chrome", uuid.New())
	if err != nil {
		t.Fatalf("unexpected login failure: %v", err)
	}
	if resp.AccessToken != "signed-jwt" || resp.RefreshToken != "raw-rt" {
		t.Errorf("unexpected login response: %+v", resp)
	}

	// 2. Suspended Tenant
	req.TenantSlug = "suspended"
	_, err = svc.Login(ctx, nil, req, "127.0.0.1", "Chrome", uuid.New())
	if err == nil {
		t.Fatal("expected error for suspended tenant, got nil")
	}

	// 3. Inactive User
	req.TenantSlug = "acme"
	req.Email = "inactive@example.com"
	_, err = svc.Login(ctx, nil, req, "127.0.0.1", "Chrome", uuid.New())
	if err == nil {
		t.Fatal("expected error for inactive user, got nil")
	}

	// 4. Invalid Password
	req.Email = "active@example.com"
	req.Password = "wrongpassword"
	_, err = svc.Login(ctx, nil, req, "127.0.0.1", "Chrome", uuid.New())
	if !errors.Is(err, sharedErrors.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthService_Logout(t *testing.T) {
	sessionSvc := &MockSessionService{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}
	svc := NewAuthService(nil, nil, nil, nil, nil, nil, nil, sessionSvc, publisher, auditSvc)

	sessionSvc.RevokeSessionFunc = func(ctx context.Context, tx *gorm.DB, sid uuid.UUID) error {
		return nil
	}

	err := svc.Logout(context.Background(), nil, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected logout failure: %v", err)
	}
}

func TestAuthService_RefreshToken(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	userRepo := &MockUserRepository{}
	userRoleRepo := &MockUserRoleRepository{}
	sessionRepo := &MockSessionRepository{}
	tokenRepo := &MockRefreshTokenRepository{}
	tokenSvc := &MockTokenService{}
	sessionSvc := &MockSessionService{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewAuthService(tenantRepo, userRepo, userRoleRepo, sessionRepo, tokenRepo, nil, tokenSvc, sessionSvc, publisher, auditSvc)
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	tokenSvc.HashOpaqueTokenFunc = func(token string) string {
		return "hashed-" + token
	}

	// Token not found
	tokenRepo.FindByTokenHashFunc = func(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error) {
		return nil, nil
	}
	_, err := svc.RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "invalid"}, uuid.New())
	if !errors.Is(err, sharedErrors.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}

	// Success case
	tokenRepo.FindByTokenHashFunc = func(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error) {
		rt := &models.RefreshToken{
			UserID:    userID,
			TenantID:  tenantID,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		rt.ID = uuid.New()
		return rt, nil
	}
	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		return &models.User{Status: "active"}, nil
	}
	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID) (*models.Tenant, error) {
		return &models.Tenant{Status: "active"}, nil
	}
	tokenRepo.RevokeByIDFunc = func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
		return nil
	}
	tokenSvc.GenerateOpaqueTokenFunc = func() (string, string, error) {
		return "new-raw", "new-hash", nil
	}
	tokenRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error {
		return nil
	}
	sessionSvc.CreateSessionFunc = func(ctx context.Context, tx *gorm.DB, uid, tid uuid.UUID, ip, ua string) (*models.Session, error) {
		session := &models.Session{}
		session.ID = uuid.New()
		return session, nil
	}
	userRoleRepo.FindByUserIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) ([]models.UserRole, error) {
		return nil, nil
	}
	tokenSvc.GenerateAccessTokenFunc = func(uid, tid, email string, roles []string, sid string) (string, time.Time, error) {
		return "new-access", time.Now().Add(15 * time.Minute), nil
	}

	resp, err := svc.RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "valid"}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected refresh error: %v", err)
	}
	if resp.AccessToken != "new-access" || resp.RefreshToken != "new-raw" {
		t.Errorf("unexpected response: %+v", resp)
	}

	// Reuse detection
	tokenRepo.FindByTokenHashFunc = func(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error) {
		now := time.Now()
		rt := &models.RefreshToken{
			UserID:    userID,
			TenantID:  tenantID,
			ExpiresAt: time.Now().Add(1 * time.Hour),
			RevokedAt: &now, // already revoked
		}
		rt.ID = uuid.New()
		return rt, nil
	}
	sessionSvc.RevokeAllForUserFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}
	tokenRepo.RevokeAllByUserIDFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}
	_, err = svc.RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "reused"}, uuid.New())
	if err == nil {
		t.Fatal("expected reuse detection error, got nil")
	}
}

func TestAuthService_ForgotPassword(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	userRepo := &MockUserRepository{}
	resetRepo := &MockPasswordResetTokenRepository{}
	tokenSvc := &MockTokenService{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewAuthService(tenantRepo, userRepo, nil, nil, nil, resetRepo, tokenSvc, nil, publisher, auditSvc)
	ctx := context.Background()

	tenantRepo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		tenant := &models.Tenant{}
		tenant.ID = uuid.New()
		return tenant, nil
	}
	userRepo.FindByEmailFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, email string) (*models.User, error) {
		user := &models.User{Email: email}
		user.ID = uuid.New()
		return user, nil
	}
	tokenSvc.GenerateOpaqueTokenFunc = func() (string, string, error) {
		return "raw-prt", "hash-prt", nil
	}
	resetRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error {
		return nil
	}

	// Should not leak status info (returns nil for non-existent and existent users alike)
	err := svc.ForgotPassword(ctx, nil, dto.ForgotPasswordRequest{TenantSlug: "acme", Email: "someone@example.com"}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected forgot password failure: %v", err)
	}
}

func TestAuthService_ResetPassword(t *testing.T) {
	userRepo := &MockUserRepository{}
	resetRepo := &MockPasswordResetTokenRepository{}
	tokenSvc := &MockTokenService{}
	sessionSvc := &MockSessionService{}
	tokenRepo := &MockRefreshTokenRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewAuthService(nil, userRepo, nil, nil, tokenRepo, resetRepo, tokenSvc, sessionSvc, publisher, auditSvc)
	ctx := context.Background()

	tokenSvc.HashOpaqueTokenFunc = func(token string) string {
		return "hash-" + token
	}

	// Reset - Invalid token
	resetRepo.FindByTokenHashFunc = func(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error) {
		return nil, nil
	}
	err := svc.ResetPassword(ctx, nil, dto.ResetPasswordRequest{Token: "invalid"}, uuid.New())
	if err == nil {
		t.Fatal("expected invalid token error, got nil")
	}

	// Reset - Success
	resetRepo.FindByTokenHashFunc = func(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error) {
		prt := &models.PasswordResetToken{
			UserID:    uuid.New(),
			TenantID:  uuid.New(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		prt.ID = uuid.New()
		return prt, nil
	}
	userRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*models.User, error) {
		return &models.User{}, nil
	}
	userRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, user *models.User) error {
		return nil
	}
	resetRepo.MarkAsUsedFunc = func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
		return nil
	}
	sessionSvc.RevokeAllForUserFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}
	tokenRepo.RevokeAllByUserIDFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}

	err = svc.ResetPassword(ctx, nil, dto.ResetPasswordRequest{Token: "valid", NewPassword: "newpassword123"}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected reset password error: %v", err)
	}
}
