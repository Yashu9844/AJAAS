package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

type auditLogRepository struct{}

// NewAuditLogRepository returns a AuditLogRepository.
func NewAuditLogRepository() AuditLogRepository {
	return &auditLogRepository{}
}

func (r *auditLogRepository) Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error {
	return tx.WithContext(ctx).Create(log).Error
}

func (r *auditLogRepository) FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64

	query := db.WithContext(ctx).Model(&models.AuditLog{}).Where("tenant_id = ?", tenantID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	// Order by created_at desc so latest audit logs appear first
	err := query.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
