package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type employeeStatutoryRepository struct {
	db *gorm.DB
}

func NewEmployeeStatutoryRepository(db *gorm.DB) EmployeeStatutoryRepository {
	return &employeeStatutoryRepository{db: db}
}

func (r *employeeStatutoryRepository) GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) (*models.EmployeeStatutory, error) {
	var stat models.EmployeeStatutory
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, profileID).
		First(&stat).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &stat, nil
}

func (r *employeeStatutoryRepository) Upsert(ctx context.Context, tx *gorm.DB, statutory *models.EmployeeStatutory) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "employee_profile_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tax_id", "national_id", "bank_name", "bank_account_number", "bank_routing_swift", "updated_at",
		}),
	}).Create(statutory).Error
}
