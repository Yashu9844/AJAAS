package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthService handles identity validation use cases.
type AuthService interface {
	Login(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ipAddress, userAgent string, correlationID uuid.UUID) (*dto.LoginResponse, error)
	Logout(ctx context.Context, tx *gorm.DB, tenantID, userID, sessionID uuid.UUID, correlationID uuid.UUID) error
	RefreshToken(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, correlationID uuid.UUID) (*dto.RefreshTokenResponse, error)
	ForgotPassword(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, correlationID uuid.UUID) error
	ResetPassword(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, correlationID uuid.UUID) error
}

type authService struct {
	tenantRepo   repositories.TenantRepository
	userRepo     repositories.UserRepository
	userRoleRepo repositories.UserRoleRepository
	sessionRepo  repositories.SessionRepository
	tokenRepo    repositories.RefreshTokenRepository
	resetRepo    repositories.PasswordResetTokenRepository
	tokenSvc     TokenService
	sessionSvc   SessionService
	publisher    queue.EventPublisher
	auditSvc     AuditService
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	tenantRepo repositories.TenantRepository,
	userRepo repositories.UserRepository,
	userRoleRepo repositories.UserRoleRepository,
	sessionRepo repositories.SessionRepository,
	tokenRepo repositories.RefreshTokenRepository,
	resetRepo repositories.PasswordResetTokenRepository,
	tokenSvc TokenService,
	sessionSvc SessionService,
	publisher queue.EventPublisher,
	auditSvc AuditService,
) AuthService {
	return &authService{
		tenantRepo:   tenantRepo,
		userRepo:     userRepo,
		userRoleRepo: userRoleRepo,
		sessionRepo:  sessionRepo,
		tokenRepo:    tokenRepo,
		resetRepo:    resetRepo,
		tokenSvc:     tokenSvc,
		sessionSvc:   sessionSvc,
		publisher:    publisher,
		auditSvc:     auditSvc,
	}
}

func (s *authService) Login(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ipAddress, userAgent string, correlationID uuid.UUID) (*dto.LoginResponse, error) {
	// 1. Resolve Tenant by Slug
	tenant, err := s.tenantRepo.FindBySlug(ctx, tx, req.TenantSlug)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, sharedErrors.ErrInvalidCredentials
	}

	// AU-003: Check if Tenant Status is suspended
	if tenant.Status == "suspended" {
		return nil, &sharedErrors.AppError{
			Code:       "TENANT_SUSPENDED",
			Message:    "Tenant organization is suspended",
			StatusCode: 403,
		}
	}

	// 2. Resolve User by email within Tenant
	user, err := s.userRepo.FindByEmail(ctx, tx, tenant.ID, req.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, sharedErrors.ErrInvalidCredentials
	}

	// AU-004: Check if User Status is active
	if user.Status != "active" {
		return nil, &sharedErrors.AppError{
			Code:       "ACCOUNT_NOT_ACTIVE",
			Message:    "User account is not active",
			StatusCode: 403,
		}
	}

	// 3. Verify bcrypt password hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		// Log failed login attempt
		_ = s.auditSvc.Log(ctx, tx, tenant.ID.String(), user.ID.String(), "login.failed", "user", user.ID.String(), map[string]string{"email": req.Email, "reason": "invalid password"}, ipAddress, userAgent)
		return nil, sharedErrors.ErrInvalidCredentials
	}

	// 4. Create active Session
	session, err := s.sessionSvc.CreateSession(ctx, tx, user.ID, tenant.ID, ipAddress, userAgent)
	if err != nil {
		return nil, err
	}

	// 5. Create Refresh Token (rotated)
	rawRefreshToken, tokenHash, err := s.tokenSvc.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}

	rt := &models.RefreshToken{
		UserID:    user.ID,
		TenantID:  tenant.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7 days expiration
	}
	if err := s.tokenRepo.Create(ctx, tx, rt); err != nil {
		return nil, err
	}

	// Fetch roles for JWT encoding
	userRoles, err := s.userRoleRepo.FindByUserID(ctx, tx, tenant.ID, user.ID)
	if err != nil {
		return nil, err
	}

	roles := make([]string, len(userRoles))
	roleSummaries := make([]dto.RoleSummary, len(userRoles))
	for i, ur := range userRoles {
		if ur.Role != nil {
			roles[i] = ur.Role.Name
			roleSummaries[i] = dto.RoleSummary{
				ID:   ur.Role.ID.String(),
				Name: ur.Role.Name,
			}
		}
	}

	// 6. Generate access JWT token
	jwtToken, expiresAt, err := s.tokenSvc.GenerateAccessToken(
		user.ID.String(),
		tenant.ID.String(),
		user.Email,
		roles,
		session.ID.String(),
	)
	if err != nil {
		return nil, err
	}

	// Update last login
	now := time.Now()
	user.LastLoginAt = &now
	if err := s.userRepo.Update(ctx, tx, user); err != nil {
		return nil, err
	}

	// Publish Event & Audit Log
	eventPayload := events.UserCreatedPayload{ // mapping event user login
		UserID:    user.ID,
		TenantID:  tenant.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Status:    user.Status,
		CreatedAt: now,
	}
	evt := events.NewEvent("UserLoggedIn", "identity.user.login_success", tenant.ID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", "identity.user.login_success", evt)

	_ = s.auditSvc.Log(ctx, tx, tenant.ID.String(), user.ID.String(), "login.success", "user", user.ID.String(), map[string]string{"session_id": session.ID.String()}, ipAddress, userAgent)

	return &dto.LoginResponse{
		AccessToken:  jwtToken,
		RefreshToken: rawRefreshToken,
		ExpiresAt:    expiresAt,
		User: dto.UserLoginSummary{
			ID:        user.ID.String(),
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Status:    user.Status,
			Roles:     roleSummaries,
		},
	}, nil
}

func (s *authService) Logout(ctx context.Context, tx *gorm.DB, tenantID, userID, sessionID uuid.UUID, correlationID uuid.UUID) error {
	// Revoke Session
	if err := s.sessionSvc.RevokeSession(ctx, tx, sessionID); err != nil {
		return err
	}

	// Publish Event & Audit Log
	eventPayload := events.SessionRevokedPayload{
		SessionID: sessionID,
		UserID:    userID,
		TenantID:  tenantID,
		Reason:    "logout",
		RevokedAt: time.Now(),
	}
	evt := events.NewEvent(events.TypeSessionRevoked, events.RoutingKeySessionRevoked, tenantID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeySessionRevoked, evt)

	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), userID.String(), "logout", "user", userID.String(), map[string]string{"session_id": sessionID.String()}, "", "")

	return nil
}

func (s *authService) RefreshToken(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, correlationID uuid.UUID) (*dto.RefreshTokenResponse, error) {
	// 1. Hash incoming raw token
	tokenHash := s.tokenSvc.HashOpaqueToken(req.RefreshToken)

	// 2. Lookup RefreshToken by hash
	rt, err := s.tokenRepo.FindByTokenHash(ctx, tx, tokenHash)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, sharedErrors.ErrUnauthorized
	}

	// Check if expired
	if time.Now().After(rt.ExpiresAt) {
		return nil, sharedErrors.ErrUnauthorized
	}

	// RT-005: Reuse detection
	if rt.RevokedAt != nil {
		// Log security breach attempt
		_ = s.auditSvc.Log(ctx, tx, rt.TenantID.String(), rt.UserID.String(), "token.reuse_detected", "user", rt.UserID.String(), map[string]string{"token_id": rt.ID.String()}, "", "")

		// Force invalidate ALL sessions and tokens for this user
		_ = s.sessionSvc.RevokeAllForUser(ctx, tx, rt.TenantID, rt.UserID)
		_ = s.tokenRepo.RevokeAllByUserID(ctx, tx, rt.TenantID, rt.UserID)

		return nil, &sharedErrors.AppError{
			Code:       "TOKEN_REUSED",
			Message:    "Refresh token reuse detected. All sessions invalidated.",
			StatusCode: 401,
		}
	}

	// 3. Check User status in DB
	user, err := s.userRepo.FindByID(ctx, tx, rt.TenantID, rt.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Status != "active" {
		return nil, sharedErrors.ErrUnauthorized
	}

	// 4. Check Tenant status
	tenant, err := s.tenantRepo.FindByID(ctx, tx, rt.TenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil || tenant.Status == "suspended" {
		return nil, sharedErrors.ErrUnauthorized
	}

	// 5. Rotate tokens: Revoke old token
	if err := s.tokenRepo.RevokeByID(ctx, tx, rt.ID); err != nil {
		return nil, err
	}

	// Generate new access and refresh token pair
	newRawToken, newHash, err := s.tokenSvc.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}

	newRT := &models.RefreshToken{
		UserID:    rt.UserID,
		TenantID:  rt.TenantID,
		TokenHash: newHash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.tokenRepo.Create(ctx, tx, newRT); err != nil {
		return nil, err
	}

	// Generate new Session and Access Token
	session, err := s.sessionSvc.CreateSession(ctx, tx, rt.UserID, rt.TenantID, "", "")
	if err != nil {
		return nil, err
	}

	// Fetch roles
	userRoles, err := s.userRoleRepo.FindByUserID(ctx, tx, rt.TenantID, rt.UserID)
	if err != nil {
		return nil, err
	}

	roles := make([]string, len(userRoles))
	for i, ur := range userRoles {
		if ur.Role != nil {
			roles[i] = ur.Role.Name
		}
	}

	jwtToken, expiresAt, err := s.tokenSvc.GenerateAccessToken(
		rt.UserID.String(),
		rt.TenantID.String(),
		user.Email,
		roles,
		session.ID.String(),
	)
	if err != nil {
		return nil, err
	}

	return &dto.RefreshTokenResponse{
		AccessToken:  jwtToken,
		RefreshToken: newRawToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *authService) ForgotPassword(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, correlationID uuid.UUID) error {
	// Find Tenant
	tenant, err := s.tenantRepo.FindBySlug(ctx, tx, req.TenantSlug)
	if err != nil {
		return err
	}
	if tenant == nil {
		// Silent return to prevent email enumeration attacks
		return nil
	}

	// Find User
	user, err := s.userRepo.FindByEmail(ctx, tx, tenant.ID, req.Email)
	if err != nil {
		return err
	}
	if user == nil {
		// Silent return to prevent email enumeration
		return nil
	}

	// Generate token (SHA-256 hashed in DB, Raw sent to events consumer)
	rawToken, tokenHash, err := s.tokenSvc.GenerateOpaqueToken()
	if err != nil {
		return err
	}

	prt := &models.PasswordResetToken{
		UserID:    user.ID,
		TenantID:  tenant.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour), // 1 hour expiration
	}
	if err := s.resetRepo.Create(ctx, tx, prt); err != nil {
		return err
	}

	// Publish Event mapping payload containing raw reset token
	eventPayload := map[string]interface{}{
		"user_id":     user.ID.String(),
		"tenant_id":   tenant.ID.String(),
		"email":       user.Email,
		"reset_token": rawToken,
	}
	evt := events.NewEvent("PasswordResetRequested", "identity.password.reset_requested", tenant.ID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", "identity.password.reset_requested", evt)

	_ = s.auditSvc.Log(ctx, tx, tenant.ID.String(), user.ID.String(), "password.reset_requested", "user", user.ID.String(), nil, "", "")

	return nil
}

func (s *authService) ResetPassword(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, correlationID uuid.UUID) error {
	// 1. Hash incoming token
	tokenHash := s.tokenSvc.HashOpaqueToken(req.Token)

	// 2. Fetch PasswordResetToken
	prt, err := s.resetRepo.FindByTokenHash(ctx, tx, tokenHash)
	if err != nil {
		return err
	}
	if prt == nil {
		return &sharedErrors.AppError{
			Code:       "INVALID_TOKEN",
			Message:    "Invalid reset token",
			StatusCode: 400,
		}
	}

	// Check if already used
	if prt.UsedAt != nil {
		return &sharedErrors.AppError{
			Code:       "TOKEN_ALREADY_USED",
			Message:    "Reset token has already been consumed",
			StatusCode: 400,
		}
	}

	// Check if expired
	if time.Now().After(prt.ExpiresAt) {
		return &sharedErrors.AppError{
			Code:       "TOKEN_EXPIRED",
			Message:    "Reset token has expired",
			StatusCode: 400,
		}
	}

	// 3. Hash new password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		return err
	}

	// 4. Update user password
	user, err := s.userRepo.FindByID(ctx, tx, prt.TenantID, prt.UserID)
	if err != nil {
		return err
	}
	if user == nil {
		return sharedErrors.ErrNotFound
	}

	user.PasswordHash = string(hashed)
	if err := s.userRepo.Update(ctx, tx, user); err != nil {
		return err
	}

	// Mark token as used
	if err := s.resetRepo.MarkAsUsed(ctx, tx, prt.ID); err != nil {
		return err
	}

	// PW-009: Revoke all active sessions and refresh tokens on password reset
	_ = s.sessionSvc.RevokeAllForUser(ctx, tx, prt.TenantID, prt.UserID)
	_ = s.tokenRepo.RevokeAllByUserID(ctx, tx, prt.TenantID, prt.UserID)

	// Publish Event & Audit Log
	eventPayload := events.PasswordResetPayload{
		UserID:   user.ID,
		TenantID: prt.TenantID,
		ResetAt:  time.Now(),
	}
	evt := events.NewEvent(events.TypePasswordReset, events.RoutingKeyPasswordReset, prt.TenantID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyPasswordReset, evt)

	_ = s.auditSvc.Log(ctx, tx, prt.TenantID.String(), user.ID.String(), "password.reset_completed", "user", user.ID.String(), nil, "", "")

	return nil
}
