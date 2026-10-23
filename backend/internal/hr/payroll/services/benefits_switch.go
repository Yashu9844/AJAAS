package services

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BenefitsSwitch is a BenefitDeductions whose source is installed after construction (HR/Benefits is built after
// payroll). Without a source it yields no deductions (PP-05).
type BenefitsSwitch struct {
	mu  sync.RWMutex
	src BenefitDeductions
}

// Set installs the source.
func (b *BenefitsSwitch) Set(s BenefitDeductions) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.src = s
}

// Deductions implements BenefitDeductions.
func (b *BenefitsSwitch) Deductions(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) ([]BenefitLine, error) {
	b.mu.RLock()
	src := b.src
	b.mu.RUnlock()
	if src == nil {
		return nil, nil
	}
	return src.Deductions(ctx, tenantID, employeeID, from, to)
}
