package automation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// runAction performs one action through the task use cases as the rule creator. The context carries the automation agent
// label, so the activity entry and the event of the change say who acted and the engine ignores the event.
func (s *Service) runAction(ctx context.Context, a api.Actor, taskID uuid.UUID, act Action) error {
	switch act.Type {
	case ActionMove:
		if act.ColumnID != nil {
			return s.tasks.Move(ctx, a, taskID, *act.ColumnID)
		}
	case ActionPriority:
		return s.tasks.SetPriority(ctx, a, taskID, act.Priority)
	case ActionLabel:
		if act.LabelID != nil {
			return s.tasks.AddLabel(ctx, a, taskID, *act.LabelID)
		}
	case ActionAssign:
		if act.UserID != nil {
			return s.tasks.Assign(ctx, a, taskID, *act.UserID)
		}
	case ActionComment:
		return s.tasks.Comment(ctx, a, taskID, act.Text)
	case ActionDue:
		if act.Days != nil {
			return s.tasks.SetDue(ctx, a, taskID, s.dueIn(*act.Days))
		}
	}
	return kernel.Invalid("action", "is incomplete")
}

// dueIn is midnight UTC of today plus the given number of days.
func (s *Service) dueIn(days int) time.Time {
	y, m, d := s.now().UTC().Date()
	return time.Date(y, m, d+days, 0, 0, 0, 0, time.UTC)
}
