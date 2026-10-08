package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/events"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
)

// movement is one balance change; it always becomes exactly one ledger row (LV-013).
type movement struct {
	Kind      string
	Days      calc.Days
	RequestID *uuid.UUID
	Actor     *uuid.UUID
	Note      *string
}

// move applies a movement to the projection column for its kind and appends the ledger row (architecture §4).
func (b base) move(ctx context.Context, tx *gorm.DB, bal *models.Balance, m movement) error {
	switch m.Kind {
	case models.KindCarryForward:
		bal.Opening += m.Days
	case models.KindAccrual:
		bal.Accrued += m.Days
	case models.KindAdjustment:
		bal.Adjusted += m.Days
	case models.KindReserve, models.KindRelease:
		bal.Reserved += m.Days
	case models.KindConsume, models.KindReversal:
		bal.Used += m.Days
	}
	if err := b.Repos.Balances.Update(ctx, tx, bal); err != nil {
		return err
	}
	return b.Repos.Ledger.Create(ctx, tx, &models.LedgerEntry{TenantID: bal.TenantID, EmployeeProfileID: bal.EmployeeProfileID,
		LeaveTypeID: bal.LeaveTypeID, Year: bal.Year, Kind: m.Kind, Days: m.Days, LeaveRequestID: m.RequestID,
		ActorUserID: m.Actor, Note: m.Note})
}

// entitlement is the employee-side input to balance materialization.
type entitlement struct {
	EmployeeID uuid.UUID
	Joining    *time.Time
	Year       int
}

// ensure materializes the balance lazily (FR-BL001..003, D4-06): carry-forward once on creation, then accrual
// top-up to the policy target as of today. Caller must hold the employee lock (LV-014).
func (b base) ensure(ctx context.Context, tx *gorm.DB, a Actor, typ *models.LeaveType, e entitlement) (*models.Balance, error) {
	bal, created, err := b.Repos.Balances.FindOrCreate(ctx, tx, keyFor(a.TenantID, e.EmployeeID, typ, e.Year))
	if err != nil {
		return nil, err
	}
	if created {
		if err := b.carryForward(ctx, tx, bal, typ); err != nil {
			return nil, err
		}
	}
	target := calc.Accrued(calc.Policy{Allowance: typ.AnnualAllowance, Monthly: typ.Accrual == models.AccrualMonthly}, e.Joining, e.Year, b.Now())
	delta := target - bal.Accrued
	if delta <= 0 {
		return bal, nil
	}
	if err := b.move(ctx, tx, bal, movement{Kind: models.KindAccrual, Days: delta}); err != nil {
		return nil, err
	}
	_, err = b.writeOutbox(ctx, tx, a, events.Accrued, events.AccruedPayload{EmployeeID: e.EmployeeID,
		LeaveTypeID: typ.ID, Year: e.Year, Days: delta, AccruedTotal: bal.Accrued})
	return bal, err
}

func (b base) carryForward(ctx context.Context, tx *gorm.DB, bal *models.Balance, typ *models.LeaveType) error {
	prevKey := keyFor(bal.TenantID, bal.EmployeeProfileID, typ, bal.Year-1)
	prev, err := b.Repos.Balances.Find(ctx, tx, prevKey)
	if err != nil || prev == nil {
		return err
	}
	cf := calc.CarryForward(prev.Available(), typ.CarryForwardLimit)
	if cf == 0 {
		return nil
	}
	return b.move(ctx, tx, bal, movement{Kind: models.KindCarryForward, Days: cf})
}
