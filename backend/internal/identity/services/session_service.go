package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/jaas/jaas/internal/shared/cache"
	"gorm.io/gorm"
)

// SessionService manages user HTTP session mappings.
type SessionService interface {
	CreateSession(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error)
	ValidateSession(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error)
	RevokeSession(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error
}

type sessionService struct {
	sessionRepo repositories.SessionRepository
	redisClient *cache.RedisClient
}

// NewSessionService creates a SessionService instance.
func NewSessionService(sessionRepo repositories.SessionRepository, redisClient *cache.RedisClient) SessionService {
	return &sessionService{
		sessionRepo: sessionRepo,
		redisClient: redisClient,
	}
}

func (s *sessionService) CreateSession(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error) {
	// Sessions expire in 15 minutes by default (aligning with JWT TTL)
	expiresAt := time.Now().Add(15 * time.Minute)

	session := &models.Session{
		UserID:    userID,
		TenantID:  tenantID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		ExpiresAt: expiresAt,
	}

	if err := s.sessionRepo.Create(ctx, tx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

func (s *sessionService) ValidateSession(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error) {
	// 1. Check Redis blacklist cache
	blacklistKey := fmt.Sprintf("blacklist:session:%s", sessionID.String())
	exists, err := s.redisClient.Exists(ctx, blacklistKey)
	if err == nil && exists {
		return false, nil // Session is explicitly blacklisted
	}

	// 2. Fall back to GORM database lookup
	session, err := s.sessionRepo.FindByID(ctx, db, sessionID)
	if err != nil {
		return false, err
	}
	if session == nil {
		return false, nil
	}

	// 3. Confirm revocation and expiration columns
	if session.RevokedAt != nil {
		return false, nil
	}
	if time.Now().After(session.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

func (s *sessionService) RevokeSession(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error {
	// Revoke in DB
	if err := s.sessionRepo.RevokeByID(ctx, tx, sessionID); err != nil {
		return err
	}

	// Push key to Redis blacklist (TTL matches max possible remaining session duration, e.g. 15 minutes)
	blacklistKey := fmt.Sprintf("blacklist:session:%s", sessionID.String())
	_ = s.redisClient.Set(ctx, blacklistKey, "revoked", 15*time.Minute)

	return nil
}

func (s *sessionService) RevokeAllForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	if err := s.sessionRepo.RevokeAllByUserID(ctx, tx, tenantID, userID); err != nil {
		return err
	}
	return nil
}
