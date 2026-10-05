package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

type designationRepository struct{}

// NewDesignationRepository returns a DesignationRepository.
func NewDesignationRepository() DesignationRepository {
	return &designationRepository{}
}

func (r *designationRepository) Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	return tx.WithContext(ctx).Create(d).Error
}

func (r *designationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error) {
	var d models.Designation
	err := db.WithContext(ctx).First(&d, "id = ? AND tenant_id = ?", id, tenantID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *designationRepository) FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error) {
	var d models.Designation
	err := db.WithContext(ctx).First(&d, "tenant_id = ? AND lower(title) = ?", tenantID, strings.ToLower(title)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *designationRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error) {
	var out []models.Designation
	var total int64

	query := db.WithContext(ctx).Model(&models.Designation{}).Where("tenant_id = ?", tenantID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("level ASC NULLS LAST, title ASC").Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *designationRepository) Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	return tx.WithContext(ctx).Save(d).Error
}

func (r *designationRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return tx.WithContext(ctx).Delete(&models.Designation{}, "id = ? AND tenant_id = ?", id, tenantID).Error
}
