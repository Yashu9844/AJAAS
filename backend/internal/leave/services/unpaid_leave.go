package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/models"
)

// UnpaidLeave is Module 4's read-only contract for Module 5 payroll (connections C3 of Module 5, D5-03):
// approved leave of unpaid types (is_paid = false), clipped to [from, to].
type UnpaidLeave interface {
	UnpaidDays(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (calc.Days, error)
}

type unpaidLeave struct{ base }

// NewUnpaidLeave builds the UnpaidLeave port.
func NewUnpaidLeave(d Deps) UnpaidLeave { return &unpaidLeave{base{d}} }

// UnpaidDays counts approved unpaid leave days inside the range with the request's day-counting rules
// (holidays, sandwich, half day = 0.50).
func (u *unpaidLeave) UnpaidDays(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (calc.Days, error) {
	db := u.Tx.DB()
	reqs, err := u.Repos.Requests.Overlapping(ctx, db, tenantID, employeeID, from, to)
	if err != nil {
		return 0, err
	}
	var total calc.Days
	for _, r := range reqs {
		if r.Status != models.StatusApproved {
			continue
		}
		isPaid, sandwich, err := u.typeFlags(ctx, tenantID, r.LeaveTypeID)
		if err != nil {
			return 0, err
		}
		if isPaid {
			continue
		}
		days, err := u.clippedDays(ctx, r, period{from, to}, sandwich)
		if err != nil {
			return 0, err
		}
		total += days
	}
	return total, nil
}

type period struct{ from, to time.Time }

// typeFlags returns (is_paid, sandwich_rule) for a type.
func (u *unpaidLeave) typeFlags(ctx context.Context, tenantID, typeID uuid.UUID) (bool, bool, error) {
	typ, err := u.Repos.Types.FindByID(ctx, u.Tx.DB(), tenantID, typeID)
	if err != nil || typ == nil {
		return true, false, err // unknown type counts as paid: never invent LOP
	}
	return typ.IsPaid, typ.SandwichRule, nil
}

func (u *unpaidLeave) clippedDays(ctx context.Context, r models.Request, p period, sandwich bool) (calc.Days, error) {
	start, end := r.StartDate, r.EndDate
	if start.Before(p.from) {
		start = p.from
	}
	if end.After(p.to) {
		end = p.to
	}
	holidays, err := u.blockingHolidays(ctx, u.Tx.DB(), r.TenantID, start, end)
	if err != nil {
		return 0, err
	}
	return calc.CountDays(calc.CountInput{Start: start, End: end, Half: r.HalfDay != nil, Sandwich: sandwich, Holidays: holidays}).Total, nil
}
