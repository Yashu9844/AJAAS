package core

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/api"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"github.com/jaas/jaas/internal/hr/core/services"
	"gorm.io/gorm"
)

// directory implements core/api.Directory over the core repository.
type directory struct {
	db   *gorm.DB
	repo repositories.Repository
}

func (d directory) PlacementOn(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (*api.Placement, error) {
	rows, err := d.repo.WorkerAssignments(ctx, d.db, tenantID, employeeID)
	if err != nil {
		return nil, err
	}
	day := time.Date(on.Year(), on.Month(), on.Day(), 0, 0, 0, 0, time.UTC)
	for i := range rows {
		if rows[i].CoversDate(day) {
			a := rows[i]
			return &api.Placement{GradeID: a.GradeID, DepartmentID: a.DepartmentID, ManagerEmployeeID: a.ManagerEmployeeID,
				JobProfileID: a.JobProfileID, LocationID: a.LocationID, WorkerType: a.WorkerType}, nil
		}
	}
	return nil, nil
}

func (d directory) GradeState(ctx context.Context, tenantID, gradeID uuid.UUID) (bool, bool, error) {
	g, err := d.repo.Grade(ctx, d.db, tenantID, gradeID)
	if err != nil || g == nil {
		return false, false, err
	}
	return true, g.Status == models.StatusActive, nil
}

// Directory is the read port the other HR contexts use (core/api).
func (m *Module) Directory() api.Directory { return m.dir }

// OnActionCompleted registers a hook run when an HR action completes or is rescinded (onboarding OB-03).
func (m *Module) OnActionCompleted(h api.ActionHook) { m.hooks.Add(h) }

type positionsPort struct{ svc *services.PositionService }

// PositionInfo reports a position and its open whole seats today (requisition RP-02).
func (p positionsPort) PositionInfo(ctx context.Context, tenantID, positionID uuid.UUID) (api.PositionInfo, error) {
	r, err := p.svc.Get(ctx, tenantID, positionID, "")
	if errors.Is(err, services.ErrNotFound) {
		return api.PositionInfo{}, nil
	}
	if err != nil {
		return api.PositionInfo{}, err
	}
	return api.PositionInfo{Exists: true, Active: r.Status == models.PositionActive, Title: r.Title, DepartmentID: r.DepartmentID, OpenSeats: int(r.OpenFTE / 100)}, nil
}

// Positions is the position read port other HR contexts use (core/api).
func (m *Module) Positions() api.Positions { return positionsPort{svc: m.positions} }
