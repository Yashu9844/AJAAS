// Package api is what HR/Performance publishes to the other HR contexts. Other contexts depend on this package only
// (import-boundary test in internal/hr).
package api

import (
	"context"

	"github.com/google/uuid"
)

// Ratings reads final ratings (PM-020) for other contexts, e.g. compensation merit snapshots (CP-005).
type Ratings interface {
	// FinalBands returns the performance band (low|medium|high) of each rated worker in the cycle; unknown cycle → 404.
	FinalBands(ctx context.Context, tenantID, cycleID uuid.UUID) (map[uuid.UUID]string, error)
}
