package repositories

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"gorm.io/gorm"
)

type structureRepository struct{}

// NewStructureRepository builds a StructureRepository.
func NewStructureRepository() StructureRepository { return &structureRepository{} }

func (r *structureRepository) Create(ctx context.Context, tx *gorm.DB, s *models.Structure, comps []models.Component) error {
	if err := tx.WithContext(ctx).Create(s).Error; err != nil {
		return err
	}
	for i := range comps {
		comps[i].TenantID, comps[i].StructureID, comps[i].Position = s.TenantID, s.ID, i
	}
	return tx.WithContext(ctx).Create(&comps).Error
}

func (r *structureRepository) Update(ctx context.Context, tx *gorm.DB, s *models.Structure) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", s.TenantID).Save(s).Error
}

func (r *structureRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Structure, error) {
	return first[models.Structure](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *structureRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Structure, error) {
	return first[models.Structure](db.WithContext(ctx).Where("tenant_id = ? AND lower(name) = ?", tenantID, strings.ToLower(name)))
}

func (r *structureRepository) Components(ctx context.Context, db *gorm.DB, tenantID, structureID uuid.UUID) ([]models.Component, error) {
	var out []models.Component
	err := db.WithContext(ctx).Where("tenant_id = ? AND structure_id = ?", tenantID, structureID).Order("position ASC").Find(&out).Error
	return out, err
}

func (r *structureRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Structure, int64, error) {
	q := db.WithContext(ctx).Model(&models.Structure{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paged[models.Structure](q, "name ASC", page)
}

type assignmentRepository struct{}

// NewAssignmentRepository builds an AssignmentRepository.
func NewAssignmentRepository() AssignmentRepository { return &assignmentRepository{} }

// LockEmployee takes a tx-scoped advisory lock namespaced for payroll.
func (r *assignmentRepository) LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "payroll:"+tenantID.String()+":"+employeeID.String()).Error
}

func (r *assignmentRepository) Create(ctx context.Context, tx *gorm.DB, a *models.Assignment) error {
	return tx.WithContext(ctx).Create(a).Error
}

func (r *assignmentRepository) Update(ctx context.Context, tx *gorm.DB, a *models.Assignment) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", a.TenantID).Save(a).Error
}

func (r *assignmentRepository) Current(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.Assignment, error) {
	return first[models.Assignment](db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ? AND effective_to IS NULL", tenantID, employeeID).
		Order("effective_from DESC"))
}

func (r *assignmentRepository) ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Assignment, int64, error) {
	q := db.WithContext(ctx).Model(&models.Assignment{}).Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID)
	return paged[models.Assignment](q, "effective_from DESC", page)
}

func (r *assignmentRepository) EligibleFor(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, p Period) ([]models.Assignment, error) {
	var out []models.Assignment
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (employee_profile_id) * FROM payroll_assignments
		WHERE tenant_id = ? AND effective_from <= ?::date AND (effective_to IS NULL OR effective_to >= ?::date)
		ORDER BY employee_profile_id, effective_from DESC`, tenantID, day(p.To), day(p.From)).Scan(&out).Error
	return out, err
}
