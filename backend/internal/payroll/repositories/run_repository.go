package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type runRepository struct{}

// NewRunRepository builds a RunRepository.
func NewRunRepository() RunRepository { return &runRepository{} }

func (r *runRepository) Create(ctx context.Context, tx *gorm.DB, run *models.Run) error {
	return tx.WithContext(ctx).Create(run).Error
}

func (r *runRepository) Update(ctx context.Context, tx *gorm.DB, run *models.Run) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", run.TenantID).Save(run).Error
}

func (r *runRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error) {
	return first[models.Run](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *runRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error) {
	return first[models.Run](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *runRepository) FindByPeriod(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, year, month int) (*models.Run, error) {
	return first[models.Run](db.WithContext(ctx).Where("tenant_id = ? AND year = ? AND month = ?", tenantID, year, month))
}

func (r *runRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Run, int64, error) {
	q := db.WithContext(ctx).Model(&models.Run{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paged[models.Run](q, "year DESC, month DESC", page)
}

type payslipRepository struct{}

// NewPayslipRepository builds a PayslipRepository.
func NewPayslipRepository() PayslipRepository { return &payslipRepository{} }

// DeleteByRun removes a run's payslips; lines cascade (PY-011 recalculation).
func (r *payslipRepository) DeleteByRun(ctx context.Context, tx *gorm.DB, tenantID, runID uuid.UUID) error {
	return tx.WithContext(ctx).Where("tenant_id = ? AND run_id = ?", tenantID, runID).Delete(&models.Payslip{}).Error
}

// CreateBatch inserts payslips and their lines.
func (r *payslipRepository) CreateBatch(ctx context.Context, tx *gorm.DB, slips []models.Payslip) error {
	if len(slips) == 0 {
		return nil
	}
	return tx.WithContext(ctx).CreateInBatches(&slips, 200).Error
}

func (r *payslipRepository) withLines(db *gorm.DB) *gorm.DB {
	return db.Preload("Lines", func(q *gorm.DB) *gorm.DB { return q.Order("position ASC") })
}

func (r *payslipRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Payslip, error) {
	return first[models.Payslip](r.withLines(db.WithContext(ctx)).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *payslipRepository) ListByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error) {
	q := r.withLines(db.WithContext(ctx).Model(&models.Payslip{})).Where("tenant_id = ? AND run_id = ?", tenantID, runID)
	return paged[models.Payslip](q, "employee_code ASC", page)
}

func (r *payslipRepository) AllByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID) ([]models.Payslip, error) {
	var out []models.Payslip
	err := db.WithContext(ctx).Where("tenant_id = ? AND run_id = ?", tenantID, runID).Order("employee_code ASC").Find(&out).Error
	return out, err
}

func (r *payslipRepository) ListFinalizedForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error) {
	q := r.withLines(db.WithContext(ctx).Model(&models.Payslip{})).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID).
		Where("run_id IN (?)", db.Model(&models.Run{}).Select("id").Where("tenant_id = ? AND status = ?", tenantID, models.RunFinalized))
	return paged[models.Payslip](q, "created_at DESC", page)
}

type outboxRepository struct{}

// NewOutboxRepository builds an OutboxRepository.
func NewOutboxRepository() OutboxRepository { return &outboxRepository{} }

func (r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error {
	return tx.WithContext(ctx).Create(e).Error
}

func (r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).Updates(map[string]interface{}{"published": true, "published_at": at}).Error
}

func (r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "last_error": reason}).Error
}

func (r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error) {
	var out []models.OutboxEvent
	err := db.WithContext(ctx).Where("published = ? AND attempts < ?", false, maxAttempts).Order("created_at ASC").Limit(limit).Find(&out).Error
	return out, err
}
