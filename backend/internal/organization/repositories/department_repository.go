package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

type departmentRepository struct{}

// NewDepartmentRepository returns a DepartmentRepository.
func NewDepartmentRepository() DepartmentRepository {
	return &departmentRepository{}
}

func (r *departmentRepository) Create(ctx context.Context, tx *gorm.DB, dept *models.Department) error {
	return tx.WithContext(ctx).Create(dept).Error
}

func (r *departmentRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Department, error) {
	var dept models.Department
	err := db.WithContext(ctx).First(&dept, "id = ? AND tenant_id = ?", id, tenantID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &dept, nil
}

func (r *departmentRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Department, error) {
	var dept models.Department
	err := db.WithContext(ctx).First(&dept, "tenant_id = ? AND lower(name) = ?", tenantID, strings.ToLower(name)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &dept, nil
}

func (r *departmentRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Department, int64, error) {
	var depts []models.Department
	var total int64

	query := db.WithContext(ctx).Model(&models.Department{}).Where("tenant_id = ?", tenantID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("name ASC").Find(&depts).Error; err != nil {
		return nil, 0, err
	}
	return depts, total, nil
}

func (r *departmentRepository) FindChildren(ctx context.Context, db *gorm.DB, tenantID, parentID uuid.UUID) ([]models.Department, error) {
	var depts []models.Department
	err := db.WithContext(ctx).Where("tenant_id = ? AND parent_department_id = ?", tenantID, parentID).Order("name ASC").Find(&depts).Error
	return depts, err
}

func (r *departmentRepository) FindRoots(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Department, error) {
	var depts []models.Department
	err := db.WithContext(ctx).Where("tenant_id = ? AND parent_department_id IS NULL", tenantID).Order("name ASC").Find(&depts).Error
	return depts, err
}

func (r *departmentRepository) Update(ctx context.Context, tx *gorm.DB, dept *models.Department) error {
	return tx.WithContext(ctx).Save(dept).Error
}

func (r *departmentRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return tx.WithContext(ctx).Delete(&models.Department{}, "id = ? AND tenant_id = ?", id, tenantID).Error
}
