package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"gorm.io/gorm"
)

// SetHooks installs the collaboration hooks (mentions in descriptions, automatic watchers). It is late-bound because the
// collaboration context depends on this one for task resolution.
func (s *Service) SetHooks(h api.TaskHooks) { s.hooks = h }

// textSaved reports a saved description so mentions are stored in the same transaction.
func (s *Service) textSaved(ctx context.Context, tx *gorm.DB, a api.Actor, t *Task, text string) error {
	if s.hooks == nil {
		return nil
	}
	return s.hooks.TaskTextSaved(ctx, tx, api.TaskText{TenantID: a.TenantID, ProjectID: t.ProjectID, TaskID: t.ID, ActorID: a.UserID, Text: text})
}

// watch makes users watch the task (reporter and assignees).
func (s *Service) watch(ctx context.Context, tx *gorm.DB, w api.WatchRequest) error {
	if s.hooks == nil || len(w.UserIDs) == 0 {
		return nil
	}
	return s.hooks.WatchTask(ctx, tx, w)
}

// afterCreate stores the mentions of the new description and watches the reporter and the assignees.
func (s *Service) afterCreate(u unit, t *Task, assignees []uuid.UUID) error {
	if err := s.textSaved(u.ctx, u.tx, u.a, t, t.Description); err != nil {
		return err
	}
	reporter := api.WatchRequest{TenantID: u.a.TenantID, TaskID: t.ID, Reason: "reporter", UserIDs: []uuid.UUID{u.a.UserID}}
	if err := s.watch(u.ctx, u.tx, reporter); err != nil {
		return err
	}
	return s.watch(u.ctx, u.tx, api.WatchRequest{TenantID: u.a.TenantID, TaskID: t.ID, Reason: "assignee", UserIDs: assignees})
}
