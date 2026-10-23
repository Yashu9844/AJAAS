package plus

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/api"
	"gorm.io/gorm"
)

// ErrNotPayable is returned when an encashment cannot be marked paid (unknown, unapproved or already paid).
var ErrNotPayable = errors.New("plus: encashment is not an approved, unpaid request")

// settlementPort implements leave/api.Settlement over the parity store.
type settlementPort struct{ store Store }

// SettlementPort builds the port payroll final settlement uses.
func SettlementPort(store Store) api.Settlement { return settlementPort{store: store} }

func (p settlementPort) EncashableBalances(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]api.EncashableBalance, error) {
	rows, err := p.store.LatestBalances(ctx, db, tenantID, employeeID)
	out := make([]api.EncashableBalance, 0, len(rows))
	for _, r := range rows {
		if r.Days > 0 {
			out = append(out, api.EncashableBalance{LeaveTypeID: r.LeaveTypeID, Days: r.Days})
		}
	}
	return out, err
}

func (p settlementPort) UnpaidEncashments(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]api.UnpaidEncashment, error) {
	rows, err := p.store.UnpaidEncashments(ctx, db, tenantID, employeeID)
	out := make([]api.UnpaidEncashment, 0, len(rows))
	for _, r := range rows {
		out = append(out, api.UnpaidEncashment{ID: r.ID, LeaveTypeID: r.LeaveTypeID, Days: r.Days})
	}
	return out, err
}

func (p settlementPort) MarkEncashmentsPaid(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, ids []uuid.UUID, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := p.store.MarkPaid(ctx, tx, tenantID, ids, at)
	if err != nil {
		return err
	}
	if n != int64(len(ids)) {
		return ErrNotPayable
	}
	return nil
}
