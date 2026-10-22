// Package api is what HR/Attendance publishes to the other HR contexts. Other contexts depend on this package only
// (import-boundary test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Presence reads worked time (leave comp-off LP-08).
type Presence interface {
	// WorkedMinutes returns the work minutes recorded for the worker on the date (0 when no record).
	WorkedMinutes(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (int, error)
}
