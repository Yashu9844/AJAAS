package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/models"
	"gorm.io/gorm"
)

type employeeTimelineRepository struct {
	db *gorm.DB
}

func NewEmployeeTimelineRepository(db *gorm.DB) EmployeeTimelineRepository {
	return &employeeTimelineRepository{db: db}
}

func (r *employeeTimelineRepository) Create(ctx context.Context, tx *gorm.DB, timeline *models.EmployeeTimeline) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Create(timeline).Error
}

func (r *employeeTimelineRepository) ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeTimeline, error) {
	var timelines []models.EmployeeTimeline
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, profileID).
		Order("effective_date DESC, created_at DESC").
		Find(&timelines).Error
	if err != nil {
		return nil, err
	}
	return timelines, nil
}
