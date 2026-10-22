// Package api is what HR/Onboarding publishes to the other HR contexts. Other contexts depend on this package only
// (import-boundary test in internal/hr).
package api

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Clearance tells payroll whether a leaver's exit clearance is complete (onboarding OB-07, payroll final settlement).
type Clearance interface {
	ClearanceCompleted(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (bool, error)
}
