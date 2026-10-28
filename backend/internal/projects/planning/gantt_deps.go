package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

// attachDependencies fills the blocks edges and per-task dependency counts of a Gantt page (VW-04).
//
// The task context owns task relations (TK-07); this context only reads task_relations by SQL. An edge is returned
// when one end is on the page and both ends are live tasks in projects the caller can read, so a page never reveals a
// task of an unreadable project. DependencyCount is the number of readable tasks that block the task.
func (s *Service) attachDependencies(ctx context.Context, a api.Actor, v *GanttView) error {
	if len(v.Tasks) == 0 {
		return nil
	}
	page := make([]uuid.UUID, len(v.Tasks))
	for i, t := range v.Tasks {
		page[i] = t.ID
	}
	readable, err := s.taskProjects(ctx, a)
	if err != nil || len(readable) == 0 {
		return err
	}
	var edges []Dependency
	err = s.db.WithContext(ctx).Raw(`SELECT r.source_task_id, r.target_task_id, r.type FROM task_relations r
		JOIN tasks src ON src.id = r.source_task_id AND src.tenant_id = r.tenant_id AND src.deleted_at IS NULL AND src.project_id = ANY(?::uuid[])
		JOIN tasks dst ON dst.id = r.target_task_id AND dst.tenant_id = r.tenant_id AND dst.deleted_at IS NULL AND dst.project_id = ANY(?::uuid[])
		WHERE r.tenant_id = ? AND r.type = 'blocks' AND (r.source_task_id = ANY(?::uuid[]) OR r.target_task_id = ANY(?::uuid[]))
		ORDER BY r.source_task_id, r.target_task_id LIMIT 2000`,
		idArray(readable), idArray(readable), a.TenantID, idArray(page), idArray(page)).Scan(&edges).Error
	if err != nil {
		return err
	}
	blockers := map[uuid.UUID]int{}
	for _, e := range edges {
		blockers[e.TargetTaskID]++
	}
	for i := range v.Tasks {
		v.Tasks[i].DependencyCount = blockers[v.Tasks[i].ID]
	}
	if edges != nil {
		v.Dependencies = edges
	}
	return nil
}
