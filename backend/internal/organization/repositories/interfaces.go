package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

// DepartmentRepository handles CRUD for departments scoped by tenant.
type DepartmentRepository interface {
	Create(ctx context.Context, tx *gorm.DB, dept *models.Department) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Department, error)
	FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Department, error)
	FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Department, int64, error)
	FindChildren(ctx context.Context, db *gorm.DB, tenantID, parentID uuid.UUID) ([]models.Department, error)
	FindRoots(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Department, error)
	Update(ctx context.Context, tx *gorm.DB, dept *models.Department) error
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

// TeamRepository handles CRUD for teams scoped by tenant.
type TeamRepository interface {
	Create(ctx context.Context, tx *gorm.DB, team *models.Team) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Team, error)
	FindByName(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID, name string) (*models.Team, error)
	FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) ([]models.Team, int64, error)
	CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error)
	Update(ctx context.Context, tx *gorm.DB, team *models.Team) error
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

// DesignationRepository handles CRUD for designations scoped by tenant.
type DesignationRepository interface {
	Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error)
	FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error)
	FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error)
	Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error
	Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
}

// MappingRepository handles user→org mappings (append-only; deactivate, never hard delete).
type MappingRepository interface {
	Create(ctx context.Context, tx *gorm.DB, m *models.Mapping) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Mapping, error)
	FindPrimaryByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Mapping, error)
	FindByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) ([]models.Mapping, int64, error)
	FindReports(ctx context.Context, db *gorm.DB, tenantID, managerID uuid.UUID) ([]models.Mapping, error)
	FindByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) ([]models.Mapping, error)
	CountActiveByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error)
	CountActiveByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) (int64, error)
	CountActiveByDesignation(ctx context.Context, db *gorm.DB, tenantID, designationID uuid.UUID) (int64, error)
	Update(ctx context.Context, tx *gorm.DB, m *models.Mapping) error
}
