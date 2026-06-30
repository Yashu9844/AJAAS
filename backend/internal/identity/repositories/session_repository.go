package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type sessionRepository struct{}

// NewSessionRepository returns a SessionRepository.
func NewSessionRepository() SessionRepository {
	return &sessionRepository{}
}

func (r *sessionRepository) Create(ctx context.Context, tx *gorm.DB, session *models.Session) error {
	return tx.WithContext(ctx).Create(session).Error
}

func (r *sessionRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error) {
	var session models.Session
	err := db.WithContext(ctx).First(&session, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *sessionRepository) RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	now := time.Now()
	return tx.WithContext(ctx).
		Model(&models.Session{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", &now).Error
}

func (r *sessionRepository) RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	now := time.Now()
	return tx.WithContext(ctx).
		Model(&models.Session{}).
		Where("tenant_id = ? AND user_id = ? AND revoked_at IS NULL", tenantID, userID).
		Update("revoked_at", &now).Error
}

type refreshTokenRepository struct{}

// NewRefreshTokenRepository returns a RefreshTokenRepository.
func NewRefreshTokenRepository() RefreshTokenRepository {
	return &refreshTokenRepository{}
}

func (r *refreshTokenRepository) Create(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error {
	return tx.WithContext(ctx).Create(rt).Error
}

func (r *refreshTokenRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := db.WithContext(ctx).First(&rt, "token_hash = ?", hash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rt, nil
}

func (r *refreshTokenRepository) RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	now := time.Now()
	return tx.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", &now).Error
}

func (r *refreshTokenRepository) RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	now := time.Now()
	return tx.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("tenant_id = ? AND user_id = ? AND revoked_at IS NULL", tenantID, userID).
		Update("revoked_at", &now).Error
}
