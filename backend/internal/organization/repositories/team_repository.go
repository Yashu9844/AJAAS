package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

type teamRepository struct{}

// NewTeamRepository returns a TeamRepository.
func NewTeamRepository() TeamRepository {
	return &teamRepository{}
}

func (r *teamRepository) Create(ctx context.Context, tx *gorm.DB, team *models.Team) error {
	return tx.WithContext(ctx).Create(team).Error
}

func (r *teamRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Team, error) {
	var team models.Team
	err := db.WithContext(ctx).First(&team, "id = ? AND tenant_id = ?", id, tenantID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &team, nil
}

func (r *teamRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) ([]models.Team, int64, error) {
	var teams []models.Team
	var total int64

	query := db.WithContext(ctx).Model(&models.Team{}).Where("tenant_id = ?", tenantID)
	if departmentID != nil {
		query = query.Where("department_id = ?", *departmentID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("name ASC").Find(&teams).Error; err != nil {
		return nil, 0, err
	}
	return teams, total, nil
}

func (r *teamRepository) CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error) {
	var total int64
	err := db.WithContext(ctx).Model(&models.Team{}).Where("tenant_id = ? AND department_id = ?", tenantID, departmentID).Count(&total).Error
	return total, err
}

func (r *teamRepository) Update(ctx context.Context, tx *gorm.DB, team *models.Team) error {
	return tx.WithContext(ctx).Save(team).Error
}

func (r *teamRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return tx.WithContext(ctx).Delete(&models.Team{}, "id = ? AND tenant_id = ?", id, tenantID).Error
}
