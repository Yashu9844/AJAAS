package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"gorm.io/gorm"
)

// PastPayslip is a finalized payslip with its run month and the ARREARS earning it already carried (PP-04).
type PastPayslip struct {
	models.Payslip
	RunYear     int        `gorm:"column:run_year"`
	RunMonth    int        `gorm:"column:run_month"`
	ArrearsLine calc.Money `gorm:"column:arrears_line"`
}

// ArrearsRepository persists settled arrears (PP-04).
type ArrearsRepository interface {
	// PaidFor sums, per source payslip, what runs other than exceptRun already settled.
	PaidFor(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, sourceIDs []uuid.UUID, exceptRun uuid.UUID) (map[uuid.UUID]calc.Money, error)
	// ReplaceForRun replaces a run's own arrears rows (recalculation, PY-011).
	ReplaceForRun(ctx context.Context, tx *gorm.DB, tenantID, runID uuid.UUID, rows []models.Arrear) error
}

type arrearsRepository struct{}

// NewArrearsRepository builds an ArrearsRepository.
func NewArrearsRepository() ArrearsRepository { return &arrearsRepository{} }

func (r *arrearsRepository) PaidFor(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, ids []uuid.UUID, exceptRun uuid.UUID) (map[uuid.UUID]calc.Money, error) {
	out := map[uuid.UUID]calc.Money{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		SourcePayslipID uuid.UUID
		Total           calc.Money
	}
	err := db.WithContext(ctx).Model(&models.Arrear{}).Select("source_payslip_id, SUM(amount) AS total").
		Where("tenant_id = ? AND source_payslip_id IN ? AND run_id <> ?", tenantID, ids, exceptRun).Group("source_payslip_id").Scan(&rows).Error
	for _, row := range rows {
		out[row.SourcePayslipID] = row.Total
	}
	return out, err
}

func (r *arrearsRepository) ReplaceForRun(ctx context.Context, tx *gorm.DB, tenantID, runID uuid.UUID, rows []models.Arrear) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND run_id = ?", tenantID, runID).Delete(&models.Arrear{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&rows).Error
}

// PeriodAssignments returns every assignment overlapping the period, ordered by employee then start (PP-03).
func (r *assignmentRepository) PeriodAssignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, p Period) ([]models.Assignment, error) {
	var out []models.Assignment
	err := db.WithContext(ctx).Where("tenant_id = ? AND effective_from <= ?::date AND (effective_to IS NULL OR effective_to >= ?::date)", tenantID, day(p.To), day(p.From)).
		Order("employee_profile_id, effective_from").Find(&out).Error
	return out, err
}

// ForEmployee returns one employee assignments overlapping the period, ordered by start.
func (r *assignmentRepository) ForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, p Period) ([]models.Assignment, error) {
	var out []models.Assignment
	err := db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND effective_from <= ?::date AND (effective_to IS NULL OR effective_to >= ?::date)",
		tenantID, employeeID, day(p.To), day(p.From)).Order("effective_from").Find(&out).Error
	return out, err
}

// FinalizedBefore returns the employee payslips of finalized runs in the `months` months before (year, month) (PP-04).
func (r *payslipRepository) FinalizedBefore(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, year, month, months int) ([]PastPayslip, error) {
	var out []PastPayslip
	current := year*12 + month
	err := db.WithContext(ctx).Table("payroll_payslips AS p").
		Select(`p.*, r.year AS run_year, r.month AS run_month,
			COALESCE((SELECT SUM(l.amount) FROM payroll_payslip_lines l WHERE l.payslip_id = p.id AND l.code = 'ARREARS'), 0) AS arrears_line`).
		Joins("JOIN payroll_runs r ON r.id = p.run_id").
		Where("p.tenant_id = ? AND p.employee_profile_id = ? AND r.status = ? AND r.year * 12 + r.month < ? AND r.year * 12 + r.month >= ?",
			tenantID, employeeID, models.RunFinalized, current, current-months).
		Order("r.year, r.month").Scan(&out).Error
	return out, err
}
