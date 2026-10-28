package collab

import (
	"context"

	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// hooks adapts the service to api.TaskHooks.
type hooks struct{ s *Service }

// TaskHooks returns the hooks the task context calls for descriptions and automatic watchers.
func (s *Service) TaskHooks() api.TaskHooks { return hooks{s: s} }

// TaskTextSaved stores the readable mentions of a description, watches the newly mentioned users and emits
// task.mentioned with ids only (CM-02, PG11).
func (h hooks) TaskTextSaved(ctx context.Context, tx *gorm.DB, t api.TaskText) error {
	readable, err := h.s.ReadableMentions(ctx, t.TenantID, t.ProjectID, ParseMentions(t.Text))
	if err != nil {
		return err
	}
	added, err := ReplaceMentions(ctx, tx, MentionSet{TenantID: t.TenantID, Kind: "task", SourceID: t.TaskID, TaskID: t.TaskID, Users: readable})
	if err != nil || len(added) == 0 {
		return err
	}
	if err := watchMany(ctx, tx, Watcher{TaskID: t.TaskID, TenantID: t.TenantID, Reason: ReasonMentioned}, added); err != nil {
		return err
	}
	return kernel.Emit(ctx, tx, t.TenantID, "task.mentioned", map[string]any{"task_id": t.TaskID, "project_id": t.ProjectID,
		"source": "task", "source_id": t.TaskID, "user_ids": added, "actor_user_id": t.ActorID})
}

// WatchTask makes the users watchers of the task.
func (h hooks) WatchTask(ctx context.Context, tx *gorm.DB, w api.WatchRequest) error {
	return watchMany(ctx, tx, Watcher{TaskID: w.TaskID, TenantID: w.TenantID, Reason: w.Reason}, w.UserIDs)
}
