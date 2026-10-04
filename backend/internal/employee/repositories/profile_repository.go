package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"gorm.io/gorm"
)

type employeeProfileRepository struct {
	db *gorm.DB
}

func NewEmployeeProfileRepository(db *gorm.DB) EmployeeProfileRepository {
	return &employeeProfileRepository{db: db}
}

func (r *employeeProfileRepository) WithTx(tx *gorm.DB) EmployeeProfileRepository {
	if tx == nil {
		return r
	}
	return &employeeProfileRepository{db: tx}
}

func (r *employeeProfileRepository) Create(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Create(profile).Error
}

func (r *employeeProfileRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.EmployeeProfile, error) {
	var profile models.EmployeeProfile
	err := r.db.WithContext(ctx).
		Preload("EmploymentDetail").
		Preload("Contact").
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *employeeProfileRepository) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*models.EmployeeProfile, error) {
	var profile models.EmployeeProfile
	err := r.db.WithContext(ctx).
		Preload("EmploymentDetail").
		Preload("Contact").
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *employeeProfileRepository) GetByCode(ctx context.Context, tenantID uuid.UUID, code string) (*models.EmployeeProfile, error) {
	var profile models.EmployeeProfile
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND LOWER(employee_code) = LOWER(?)", tenantID, strings.TrimSpace(code)).
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *employeeProfileRepository) List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]models.EmployeeProfile, int64, error) {
	var profiles []models.EmployeeProfile
	var total int64

	q := r.db.WithContext(ctx).Model(&models.EmployeeProfile{}).
		Preload("EmploymentDetail").
		Preload("Contact").
		Where("tenant_id = ?", tenantID)

	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}

	if filter.Search != "" {
		s := "%" + strings.ToLower(filter.Search) + "%"
		q = q.Where("(LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ? OR LOWER(employee_code) LIKE ?)", s, s, s)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	perPage := filter.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&profiles).Error; err != nil {
		return nil, 0, err
	}

	return profiles, total, nil
}

func (r *employeeProfileRepository) Update(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Save(profile).Error
}

func (r *employeeProfileRepository) UpdateEmploymentDetail(ctx context.Context, tx *gorm.DB, detail *models.EmploymentDetail) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Save(detail).Error
}

func (r *employeeProfileRepository) UpdateContact(ctx context.Context, tx *gorm.DB, contact *models.EmployeeContact) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Save(contact).Error
}

func (r *employeeProfileRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.EmployeeProfile{}).Error
}
