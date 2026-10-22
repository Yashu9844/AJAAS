// Package api is what HR/Leave publishes to the other HR contexts. Other contexts depend on this package only
// (import-boundary test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Holidays reads the tenant holiday calendar (attendance AP-07).
type Holidays interface {
	// IsHoliday reports whether the date is a (non-optional) holiday for the tenant.
	IsHoliday(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (bool, error)
}
