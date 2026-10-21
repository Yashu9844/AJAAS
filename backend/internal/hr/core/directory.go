package core

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/api"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
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
