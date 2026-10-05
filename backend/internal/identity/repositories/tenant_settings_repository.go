package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenantSettingsRepository struct{}

// NewTenantSettingsRepository returns a TenantSettingsRepository.
func NewTenantSettingsRepository() TenantSettingsRepository {
	return &tenantSettingsRepository{}
}

func (r *tenantSettingsRepository) Upsert(ctx context.Context, tx *gorm.DB, setting *models.TenantSettings) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(setting).Error
}

func (r *tenantSettingsRepository) FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.TenantSettings, error) {
	var settings []models.TenantSettings
	err := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *tenantSettingsRepository) FindByKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.TenantSettings, error) {
	var setting models.TenantSettings
	err := db.WithContext(ctx).First(&setting, "tenant_id = ? AND key = ?", tenantID, key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &setting, nil
}
