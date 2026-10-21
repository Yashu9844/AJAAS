// Package api is what HR/Core publishes to the other HR contexts: read-only facts about job assignments and grades.
// Other contexts depend on this package only (import-boundary test in internal/hr).
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Placement is the worker's assignment facts on a date (all nil when there is no assignment).
type Placement struct {
	GradeID           *uuid.UUID
	DepartmentID      *uuid.UUID
	ManagerEmployeeID *uuid.UUID
	JobProfileID      *uuid.UUID
	LocationID        *uuid.UUID
	WorkerType        string
}

// Directory reads core facts.
type Directory interface {
	// PlacementOn returns the worker's active assignment facts on the date, or nil.
	PlacementOn(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (*Placement, error)
	// GradeState reports whether a grade exists in the tenant and whether it is active.
	GradeState(ctx context.Context, tenantID, gradeID uuid.UUID) (exists, active bool, err error)
}

// ActionFact is an HR action that completed or was rescinded (onboarding OB-03).
type ActionFact struct {
	ActionID, EmployeeID uuid.UUID
	Type                 string // hire | rehire | terminate | promote | transfer | ...
	EffectiveDate        time.Time
	LastWorkingDay       *time.Time // terminate only
	Rescinded            bool
	// Placement is the worker's assignment on the effective date (the last working day for a termination).
	Placement *Placement
}

// ActionHook reacts to an action inside core's transaction; an error rolls the action back.
type ActionHook func(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, fact ActionFact) error
