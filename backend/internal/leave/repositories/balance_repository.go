package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type balanceRepository struct{}

// NewBalanceRepository builds a BalanceRepository.
func NewBalanceRepository() BalanceRepository { return &balanceRepository{} }

// LockEmployee takes a tx-scoped advisory lock namespaced away from Module 3's attendance lock (LV-014).
func (r *balanceRepository) LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "leave:"+tenantID.String()+":"+employeeID.String()).Error
}

// FindOrCreate inserts a zero row unless the key exists; RowsAffected tells whether this call created it.
func (r *balanceRepository) FindOrCreate(ctx context.Context, tx *gorm.DB, key BalanceKey) (*models.Balance, bool, error) {
	seed := &models.Balance{TenantID: key.TenantID, EmployeeProfileID: key.EmployeeID, LeaveTypeID: key.LeaveTypeID, Year: key.Year}
	res := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "employee_profile_id"}, {Name: "leave_type_id"}, {Name: "year"}},
		DoNothing: true,
	}).Create(seed)
	if res.Error != nil {
		return nil, false, res.Error
	}
	b, err := r.Find(ctx, tx, key)
	if err == nil && b == nil {
		return nil, false, gorm.ErrRecordNotFound
	}
	return b, res.RowsAffected == 1, err
}

func (r *balanceRepository) Find(ctx context.Context, db *gorm.DB, key BalanceKey) (*models.Balance, error) {
	return first[models.Balance](db.WithContext(ctx).Where(
		"tenant_id = ? AND employee_profile_id = ? AND leave_type_id = ? AND year = ?",
		key.TenantID, key.EmployeeID, key.LeaveTypeID, key.Year))
}

func (r *balanceRepository) Update(ctx context.Context, tx *gorm.DB, b *models.Balance) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", b.TenantID).Save(b).Error
}

type ledgerRepository struct{}

// NewLedgerRepository builds a LedgerRepository.
func NewLedgerRepository() LedgerRepository { return &ledgerRepository{} }

func (r *ledgerRepository) Create(ctx context.Context, tx *gorm.DB, e *models.LedgerEntry) error {
	return tx.WithContext(ctx).Create(e).Error
}

func (r *ledgerRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f LedgerFilter, page dto.Page) ([]models.LedgerEntry, int64, error) {
	q := db.WithContext(ctx).Model(&models.LedgerEntry{}).Where("tenant_id = ? AND employee_profile_id = ?", tenantID, f.EmployeeID)
	if f.LeaveTypeID != nil {
		q = q.Where("leave_type_id = ?", *f.LeaveTypeID)
	}
	return paged[models.LedgerEntry](q, "created_at DESC, id DESC", page)
}
