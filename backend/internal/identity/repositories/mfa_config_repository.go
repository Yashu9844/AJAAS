package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type mfaConfigRepository struct{}

// NewMFAConfigRepository returns a MFAConfigRepository.
func NewMFAConfigRepository() MFAConfigRepository {
	return &mfaConfigRepository{}
}

func (r *mfaConfigRepository) Create(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error {
	return tx.WithContext(ctx).Create(mfa).Error
}

func (r *mfaConfigRepository) FindByUserIDAndType(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, mfaType string) (*models.MFAConfig, error) {
	var mfa models.MFAConfig
	err := db.WithContext(ctx).
		First(&mfa, "tenant_id = ? AND user_id = ? AND type = ?", tenantID, userID, mfaType).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &mfa, nil
}

func (r *mfaConfigRepository) Update(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error {
	return tx.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", mfa.TenantID, mfa.ID).
		Save(mfa).Error
}
