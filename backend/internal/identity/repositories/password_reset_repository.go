package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type passwordResetTokenRepository struct{}

// NewPasswordResetTokenRepository returns a PasswordResetTokenRepository.
func NewPasswordResetTokenRepository() PasswordResetTokenRepository {
	return &passwordResetTokenRepository{}
}

func (r *passwordResetTokenRepository) Create(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error {
	return tx.WithContext(ctx).Create(prt).Error
}

func (r *passwordResetTokenRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error) {
	var prt models.PasswordResetToken
	err := db.WithContext(ctx).First(&prt, "token_hash = ?", hash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &prt, nil
}

func (r *passwordResetTokenRepository) MarkAsUsed(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	now := time.Now()
	return tx.WithContext(ctx).
		Model(&models.PasswordResetToken{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", &now).Error
}
