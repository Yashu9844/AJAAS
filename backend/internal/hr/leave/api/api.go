// Package api is what HR/Leave publishes to the other HR contexts. Other contexts depend on this package only
// (import-boundary test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// Holidays reads the tenant holiday calendar (attendance AP-07).
type Holidays interface {
	// IsHoliday reports whether the date is a (non-optional) holiday for the tenant.
	IsHoliday(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (bool, error)
	// IsHolidayFor is IsHoliday for one worker: tenant or their location's holiday, plus the location's weekly-off
	// verdict when it has a pattern (nil otherwise, so callers keep their own default).
	IsHolidayFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (holiday bool, weekOff *bool, err error)
}

// EncashableBalance is the days a worker still holds in a leave type that has an encashment rule.
type EncashableBalance struct {
	LeaveTypeID uuid.UUID
	Days        kernel.Days
}

// UnpaidEncashment is an approved encashment that payroll has not paid yet.
type UnpaidEncashment struct {
	ID, LeaveTypeID uuid.UUID
	Days            kernel.Days
}

// Settlement is what payroll final settlement reads from HR/Leave (payroll PP-06).
type Settlement interface {
	// EncashableBalances returns the latest available days per encashable leave type (read only, no accrual top-up).
	EncashableBalances(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]EncashableBalance, error)
	// UnpaidEncashments returns the worker approved encashments with no payout yet.
	UnpaidEncashments(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]UnpaidEncashment, error)
	// MarkEncashmentsPaid stamps the encashments paid on the caller transaction; an id that is not an approved, unpaid
	// encashment of the tenant is an error and nothing is stamped.
	MarkEncashmentsPaid(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, ids []uuid.UUID, at time.Time) error
}
