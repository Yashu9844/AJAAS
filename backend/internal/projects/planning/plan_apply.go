package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// planBatch is the set of tasks that really change sprint in one planning move.
type planBatch struct {
	project uuid.UUID
	to      *uuid.UUID
	tasks   []planTask
}

// applyPlan writes the sprint change for the locked tasks with one UPDATE of tasks.sprint_id, then records one activity
// entry and one event per task. Data and payloads carry ids and versions only (PG12).
func (s *Service) applyPlan(ctx context.Context, tx *gorm.DB, a api.Actor, b planBatch) error {
	if len(b.tasks) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(b.tasks))
	for i, t := range b.tasks {
		ids[i] = t.ID
	}
	err := tx.Exec(`UPDATE tasks SET sprint_id = ?, version = version + 1, updated_at = ?
		WHERE tenant_id = ? AND project_id = ? AND id = ANY(?::uuid[])`, b.to, s.now().UTC(), a.TenantID, b.project, idArray(ids)).Error
	if err != nil {
		return err
	}
	for _, t := range b.tasks {
		taskID := t.ID
		err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: b.project, TaskID: &taskID, ActorID: &a.UserID,
			Type: "sprint_changed", Data: map[string]any{"from_sprint_id": t.SprintID, "to_sprint_id": b.to}})
		if err != nil {
			return err
		}
		err = kernel.Emit(ctx, tx, a.TenantID, "task.updated", map[string]any{"project_id": b.project, "task_id": taskID,
			"version": t.Version + 1, "sprint_id": b.to})
		if err != nil {
			return err
		}
	}
	s.audit(ctx, tx, a, "project.plan_moved", ref{"project", b.project})
	return nil
}
