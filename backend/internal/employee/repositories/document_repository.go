package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/models"
	"gorm.io/gorm"
)

type employeeDocumentRepository struct {
	db *gorm.DB
}

func NewEmployeeDocumentRepository(db *gorm.DB) EmployeeDocumentRepository {
	return &employeeDocumentRepository{db: db}
}

func (r *employeeDocumentRepository) Create(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Create(doc).Error
}

func (r *employeeDocumentRepository) GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*models.EmployeeDocument, error) {
	var doc models.EmployeeDocument
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, docID).
		First(&doc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

func (r *employeeDocumentRepository) ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeDocument, error) {
	var docs []models.EmployeeDocument
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, profileID).
		Order("created_at DESC").
		Find(&docs).Error
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func (r *employeeDocumentRepository) Update(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Save(doc).Error
}
