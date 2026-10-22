package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	"github.com/jaas/jaas/internal/hr/leave/models"
	"gorm.io/gorm"
)

// BalanceMover is the transactional balance port the leave parity package uses (comp-off credit and expiry, encashment):
// every method runs on the caller's transaction under the per-employee lock (LV-014) and writes ledger rows (LV-013).
type BalanceMover interface {
	// AvailableTx returns the days available now in the current leave year, after the lazy accrual top-up.
	AvailableTx(ctx context.Context, tx *gorm.DB, a Actor, employeeID, leaveTypeID uuid.UUID) (calc.Days, error)
	// AdjustTx appends one adjustment row (days may be negative) and returns the available days afterwards.
	AdjustTx(ctx context.Context, tx *gorm.DB, a Actor, employeeID, leaveTypeID uuid.UUID, days calc.Days, note string) (calc.Days, error)
}

type balanceMover struct{ base }

// NewBalanceMover builds the BalanceMover port.
func NewBalanceMover(d Deps) BalanceMover { return &balanceMover{base{d}} }

func (m *balanceMover) balance(ctx context.Context, tx *gorm.DB, a Actor, employeeID, leaveTypeID uuid.UUID) (*models.Balance, error) {
	if err := m.Repos.Balances.LockEmployee(ctx, tx, a.TenantID, employeeID); err != nil {
		return nil, err
	}
	emp, err := m.Employees.GetByID(ctx, a.TenantID, employeeID)
	emp, err = checkEmployee(emp, err, false)
	if err != nil {
		return nil, err
	}
	typ, err := m.Repos.Types.FindByID(ctx, tx, a.TenantID, leaveTypeID)
	if err != nil || typ == nil {
		return nil, orNotFound(err)
	}
	year, err := m.leaveYear(ctx, tx, a.TenantID, m.today())
	if err != nil {
		return nil, err
	}
	return m.ensure(ctx, tx, a, typ, entitlement{EmployeeID: employeeID, Joining: joiningOf(emp), Year: year})
}

func (m *balanceMover) AvailableTx(ctx context.Context, tx *gorm.DB, a Actor, employeeID, leaveTypeID uuid.UUID) (calc.Days, error) {
	bal, err := m.balance(ctx, tx, a, employeeID, leaveTypeID)
	if err != nil {
		return 0, err
	}
	return bal.Available(), nil
}

func (m *balanceMover) AdjustTx(ctx context.Context, tx *gorm.DB, a Actor, employeeID, leaveTypeID uuid.UUID, days calc.Days, note string) (calc.Days, error) {
	bal, err := m.balance(ctx, tx, a, employeeID, leaveTypeID)
	if err != nil {
		return 0, err
	}
	actor := a.UserID
	if err := m.move(ctx, tx, bal, movement{Kind: models.KindAdjustment, Days: days, Actor: &actor, Note: &note}); err != nil {
		return 0, err
	}
	return bal.Available(), nil
}
