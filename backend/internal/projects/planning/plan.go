package planning

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// PlanInput moves tasks between the backlog (SprintID nil) and a sprint. Versions optionally maps a task id to the
// version the caller last read; a stale entry yields a per-task conflict instead of failing the whole request.
type PlanInput struct {
	TaskIDs  []uuid.UUID    `json:"task_ids"`
	SprintID *uuid.UUID     `json:"sprint_id"`
	Versions map[string]int `json:"versions"`
}

// Per-task outcomes of a planning move.
const (
	PlanMoved     = "moved"
	PlanUnchanged = "unchanged"
	PlanConflict  = "conflict"
	PlanNotFound  = "not_found"
	PlanArchived  = "archived"
)

// PlanResult is the outcome for one task; Version is the task version after the move, or the current one on conflict.
type PlanResult struct {
	TaskID  uuid.UUID `json:"task_id"`
	Status  string    `json:"status"`
	Version int       `json:"version,omitempty"`
}

type planTask struct {
	ID       uuid.UUID
	Version  int
	SprintID *uuid.UUID
	Archived bool
}

func (in PlanInput) validate() error {
	if len(in.TaskIDs) == 0 || len(in.TaskIDs) > maxPlanTasks {
		return kernel.Invalid("task_ids", "must list between 1 and 100 tasks")
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range in.TaskIDs {
		if seen[id] {
			return kernel.Invalid("task_ids", "must not repeat a task")
		}
		seen[id] = true
	}
	return nil
}

// PlanMove moves tasks between the backlog and sprints in bulk (SP-03, task.update). It writes only tasks.sprint_id
// (plus version+1 and updated_at) and checks each task version under a row lock, so a concurrent edit makes that task
// a per-task conflict while the others still move.
func (s *Service) PlanMove(ctx context.Context, a api.Actor, projectID uuid.UUID, in PlanInput) ([]PlanResult, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskUpdate); err != nil {
		return nil, err
	}
	if err := requireScrum(acc); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	var out []PlanResult
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if err := s.checkTargetSprint(tx, a, projectID, in.SprintID); err != nil {
			return err
		}
		tasks, err := lockPlanTasks(tx, a.TenantID, projectID, in.TaskIDs)
		if err != nil {
			return err
		}
		var move []planTask
		out, move = decidePlan(in, tasks)
		return s.applyPlan(ctx, tx, a, planBatch{project: projectID, to: in.SprintID, tasks: move})
	})
	return out, err
}

// checkTargetSprint requires the target to be a planned or active sprint of the project (404 otherwise).
func (s *Service) checkTargetSprint(tx *gorm.DB, a api.Actor, projectID uuid.UUID, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	var status string
	err := tx.Raw(`SELECT status FROM sprints WHERE id = ? AND tenant_id = ? AND project_id = ? FOR SHARE`, *id, a.TenantID, projectID).Row().Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return kernel.NotFound("sprint")
		}
		return err
	}
	if status == SprintCompleted {
		return kernel.Conflict("SPRINT_STATE", "tasks cannot be planned into a completed sprint")
	}
	return nil
}

// lockPlanTasks reads and row-locks the live tasks of the project among ids, in id order so concurrent plans cannot deadlock.
func lockPlanTasks(tx *gorm.DB, tenantID, projectID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]planTask, error) {
	var rows []struct {
		ID       uuid.UUID
		Version  int
		SprintID *uuid.UUID
		Archived bool
	}
	err := tx.Raw(`SELECT id, version, sprint_id, (archived_at IS NOT NULL) AS archived FROM tasks
		WHERE tenant_id = ? AND project_id = ? AND id = ANY(?::uuid[]) AND deleted_at IS NULL ORDER BY id FOR UPDATE`,
		tenantID, projectID, idArray(ids)).Scan(&rows).Error
	out := make(map[uuid.UUID]planTask, len(rows))
	for _, r := range rows {
		out[r.ID] = planTask{ID: r.ID, Version: r.Version, SprintID: r.SprintID, Archived: r.Archived}
	}
	return out, err
}

// decidePlan classifies every requested task and returns the ones that actually move.
func decidePlan(in PlanInput, tasks map[uuid.UUID]planTask) ([]PlanResult, []planTask) {
	results := make([]PlanResult, 0, len(in.TaskIDs))
	var move []planTask
	for _, id := range in.TaskIDs {
		t, ok := tasks[id]
		want, hasWant := in.Versions[id.String()]
		switch {
		case !ok:
			results = append(results, PlanResult{TaskID: id, Status: PlanNotFound})
		case t.Archived:
			results = append(results, PlanResult{TaskID: id, Status: PlanArchived, Version: t.Version})
		case hasWant && want != t.Version:
			results = append(results, PlanResult{TaskID: id, Status: PlanConflict, Version: t.Version})
		case sameSprint(t.SprintID, in.SprintID):
			results = append(results, PlanResult{TaskID: id, Status: PlanUnchanged, Version: t.Version})
		default:
			results = append(results, PlanResult{TaskID: id, Status: PlanMoved, Version: t.Version + 1})
			move = append(move, t)
		}
	}
	return results, move
}

func sameSprint(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
