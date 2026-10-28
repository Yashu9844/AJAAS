package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

// Backlog lists the tasks waiting to be planned (SP-03): in a kanban project the tasks in a backlog-category column, in
// a scrum project the unfinished tasks that belong to no sprint. Ordered by rank, keyset paged (task.read).
//
// Deviation from the literal specification text: in scrum mode finished (done or canceled) tasks without a sprint are
// not part of the backlog, because there is nothing left to plan.
func (s *Service) Backlog(ctx context.Context, a api.Actor, projectID uuid.UUID, cursor string, limit int) ([]Card, string, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, "", err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, "", err
	}
	where := `t.tenant_id = ? AND t.project_id = ? AND ` + liveTask
	if acc.Project.Mode == api.ModeScrum {
		where += ` AND t.sprint_id IS NULL AND ` + openCategories
	} else {
		where += ` AND c.category = 'backlog'`
	}
	return pageRank(ctx, s.db, cardQuery{where: where, args: []any{a.TenantID, projectID}, limit: limit}, cursor)
}
