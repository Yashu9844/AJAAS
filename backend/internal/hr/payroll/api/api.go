// Package api is what HR/Payroll publishes to the other HR contexts. Other contexts depend on this package only,
// never on payroll services, repositories or models (import-boundary test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// CTC lets HR/Compensation set a worker's annual CTC from a date (compensation CO-C11). The worker keeps their
// current salary structure; without a payroll assignment nothing changes and synced is false.
type CTC interface {
	SetCTC(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, annual kernel.Money, from time.Time, actorUserID uuid.UUID) (synced bool, err error)
}

// BenefitLine is one benefit deduction for a worker and period (payroll PP-05).
type BenefitLine struct {
	Code   string       `json:"code"`
	Name   string       `json:"name"`
	Amount kernel.Money `json:"amount"`
}

// BenefitDeductions supplies benefit deductions per worker and period; HR/Benefits implements it (benefits BN-C07).
type BenefitDeductions interface {
	Deductions(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) ([]BenefitLine, error)
}
