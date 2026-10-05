package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"gorm.io/gorm"
)

type EmployeeProfileRepository interface {
	Create(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.EmployeeProfile, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*models.EmployeeProfile, error)
	GetByCode(ctx context.Context, tenantID uuid.UUID, code string) (*models.EmployeeProfile, error)
	List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]models.EmployeeProfile, int64, error)
	Update(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error
	UpdateEmploymentDetail(ctx context.Context, tx *gorm.DB, detail *models.EmploymentDetail) error
	UpdateContact(ctx context.Context, tx *gorm.DB, contact *models.EmployeeContact) error
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	WithTx(tx *gorm.DB) EmployeeProfileRepository
}

type EmployeeStatutoryRepository interface {
	GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) (*models.EmployeeStatutory, error)
	Upsert(ctx context.Context, tx *gorm.DB, statutory *models.EmployeeStatutory) error
}

type EmployeeDocumentRepository interface {
	Create(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error
	GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*models.EmployeeDocument, error)
	ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeDocument, error)
	Update(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error
}

type EmployeeTimelineRepository interface {
	Create(ctx context.Context, tx *gorm.DB, timeline *models.EmployeeTimeline) error
	ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeTimeline, error)
}
