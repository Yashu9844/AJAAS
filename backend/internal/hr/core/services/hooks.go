package services

import (
	"context"
	"sync"

	"github.com/jaas/jaas/internal/hr/core/api"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	"gorm.io/gorm"
)

// Hooks holds the action hooks other contexts register (core/api.ActionHook).
type Hooks struct {
	mu    sync.RWMutex
	hooks []api.ActionHook
}

// Add registers a hook.
func (h *Hooks) Add(fn api.ActionHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, fn)
}

func (h *Hooks) list() []api.ActionHook {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]api.ActionHook(nil), h.hooks...)
}

// runHooks tells registered contexts about a completed or rescinded action, inside the same transaction.
func (b base) runHooks(ctx context.Context, tx *gorm.DB, act *models.Action, rescinded bool) error {
	hooks := b.Hooks.list()
	if len(hooks) == 0 {
		return nil
	}
	fact := api.ActionFact{ActionID: act.ID, EmployeeID: act.EmployeeID, Type: act.Type, EffectiveDate: act.EffectiveDate, Rescinded: rescinded}
	on := act.EffectiveDate
	if act.Type == lifecycle.Terminate && act.Changes.LastWorkingDay != nil {
		if lwd, err := parseDate("last_working_day", *act.Changes.LastWorkingDay); err == nil {
			fact.LastWorkingDay, on = &lwd, lwd
		}
	}
	rows, err := b.Repo.WorkerAssignments(ctx, tx, act.TenantID, act.EmployeeID)
	if err != nil {
		return err
	}
	for i := range rows {
		if a := rows[i]; a.CoversDate(on) || (act.Type == lifecycle.Terminate && a.Status == models.AssignmentActive &&
			!a.EffectiveFrom.After(on) && a.EffectiveTo != nil && !a.EffectiveTo.Before(on)) {
			fact.Placement = &api.Placement{GradeID: a.GradeID, DepartmentID: a.DepartmentID, ManagerEmployeeID: a.ManagerEmployeeID,
				JobProfileID: a.JobProfileID, LocationID: a.LocationID, WorkerType: a.WorkerType}
		}
	}
	for _, h := range hooks {
		if err := h(ctx, tx, act.TenantID, fact); err != nil {
			return err
		}
	}
	return nil
}
