// Package api is what HR/Recruitment publishes to the other HR contexts. Other contexts depend on this package only
// (import-boundary test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HireFact is a completed recruitment hire (onboarding OB-03).
type HireFact struct {
	CandidateID, EmployeeID uuid.UUID
	JoiningDate             time.Time
}

// HireHook reacts to a hire inside recruitment's transaction; an error rolls the hire step back.
type HireHook func(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, fact HireFact) error
