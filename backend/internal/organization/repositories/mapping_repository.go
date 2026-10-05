package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

type mappingRepository struct{}

// NewMappingRepository returns a MappingRepository.
func NewMappingRepository() MappingRepository {
	return &mappingRepository{}
}

func (r *mappingRepository) Create(ctx context.Context, tx *gorm.DB, m *models.Mapping) error {
	return tx.WithContext(ctx).Create(m).Error
}

func (r *mappingRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Mapping, error) {
	var m models.Mapping
	err := db.WithContext(ctx).First(&m, "id = ? AND tenant_id = ?", id, tenantID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *mappingRepository) FindPrimaryByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Mapping, error) {
	var m models.Mapping
	err := db.WithContext(ctx).First(&m, "tenant_id = ? AND user_id = ? AND is_primary = TRUE AND status = 'active'", tenantID, userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *mappingRepository) FindByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) ([]models.Mapping, int64, error) {
	var out []models.Mapping
	var total int64

	query := db.WithContext(ctx).Model(&models.Mapping{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("is_primary DESC, created_at ASC").Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *mappingRepository) FindReports(ctx context.Context, db *gorm.DB, tenantID, managerID uuid.UUID) ([]models.Mapping, error) {
	var out []models.Mapping
	err := db.WithContext(ctx).Where("tenant_id = ? AND manager_user_id = ? AND status = 'active'", tenantID, managerID).Find(&out).Error
	return out, err
}

func (r *mappingRepository) FindByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) ([]models.Mapping, error) {
	var out []models.Mapping
	err := db.WithContext(ctx).Where("tenant_id = ? AND team_id = ? AND status = 'active'", tenantID, teamID).Find(&out).Error
	return out, err
}

func (r *mappingRepository) CountActiveByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error) {
	var total int64
	err := db.WithContext(ctx).Model(&models.Mapping{}).Where("tenant_id = ? AND department_id = ? AND status = 'active'", tenantID, departmentID).Count(&total).Error
	return total, err
}

func (r *mappingRepository) CountActiveByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) (int64, error) {
	var total int64
	err := db.WithContext(ctx).Model(&models.Mapping{}).Where("tenant_id = ? AND team_id = ? AND status = 'active'", tenantID, teamID).Count(&total).Error
	return total, err
}

func (r *mappingRepository) CountActiveByDesignation(ctx context.Context, db *gorm.DB, tenantID, designationID uuid.UUID) (int64, error) {
	var total int64
	err := db.WithContext(ctx).Model(&models.Mapping{}).Where("tenant_id = ? AND designation_id = ? AND status = 'active'", tenantID, designationID).Count(&total).Error
	return total, err
}

func (r *mappingRepository) Update(ctx context.Context, tx *gorm.DB, m *models.Mapping) error {
	return tx.WithContext(ctx).Save(m).Error
}
