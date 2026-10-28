package collab

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Watch reasons (CM-03).
const (
	ReasonExplicit  = "explicit"
	ReasonReporter  = "reporter"
	ReasonAssignee  = "assignee"
	ReasonCommenter = "commenter"
	ReasonMentioned = "mentioned"
)

// AddWatcher makes a user watch a task in the caller transaction and keeps an existing watch unchanged. The task
// context uses it for reporters and assignees; this context for commenters and mentioned users.
func AddWatcher(ctx context.Context, tx *gorm.DB, w Watcher) error {
	return tx.WithContext(ctx).Exec(`INSERT INTO task_watchers (task_id, user_id, tenant_id, reason) VALUES (?, ?, ?, ?)
		ON CONFLICT (task_id, user_id) DO NOTHING`, w.TaskID, w.UserID, w.TenantID, w.Reason).Error
}

// WatchState says whether the caller watches a task and why.
type WatchState struct {
	Watching bool   `json:"watching"`
	Reason   string `json:"reason"`
}

// Watch makes the caller an explicit watcher; watching twice is a no-op (CM-03).
func (s *Service) Watch(ctx context.Context, a api.Actor, taskID uuid.UUID) (WatchState, error) {
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return WatchState{}, err
	}
	err := s.db.WithContext(ctx).Exec(`INSERT INTO task_watchers (task_id, user_id, tenant_id, reason) VALUES (?, ?, ?, 'explicit')
		ON CONFLICT (task_id, user_id) DO UPDATE SET reason = 'explicit'`, taskID, a.UserID, a.TenantID).Error
	return WatchState{Watching: err == nil, Reason: ReasonExplicit}, err
}

// Unwatch stops the caller watching a task; it succeeds when the caller was not watching.
func (s *Service) Unwatch(ctx context.Context, a api.Actor, taskID uuid.UUID) error {
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(`DELETE FROM task_watchers WHERE tenant_id = ? AND task_id = ? AND user_id = ?`,
		a.TenantID, taskID, a.UserID).Error
}

// WatchStatus reports whether the caller watches a task.
func (s *Service) WatchStatus(ctx context.Context, a api.Actor, taskID uuid.UUID) (WatchState, error) {
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return WatchState{}, err
	}
	var w Watcher
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ? AND user_id = ?", a.TenantID, taskID, a.UserID).Take(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return WatchState{}, nil
	}
	return WatchState{Watching: err == nil, Reason: w.Reason}, err
}

type watcherCursor struct {
	User uuid.UUID `json:"u"`
}

// Watchers lists a task's watchers by user id, cursor-paged.
func (s *Service) Watchers(ctx context.Context, a api.Actor, taskID uuid.UUID, pr PageRequest) ([]Watcher, string, error) {
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return nil, "", err
	}
	var cur watcherCursor
	if err := kernel.DecodeCursor(pr.Cursor, &cur); err != nil {
		return nil, "", err
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", a.TenantID, taskID)
	if cur.User != uuid.Nil {
		q = q.Where("user_id > ?", cur.User)
	}
	var rows []Watcher
	if err := q.Order("user_id").Limit(pr.limit() + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, next := page(rows, pr.limit(), func(w Watcher) any { return watcherCursor{User: w.UserID} })
	return rows, next, nil
}

// watchMany adds auto-watchers (commenter, mentioned users) in the caller transaction.
func watchMany(ctx context.Context, tx *gorm.DB, w Watcher, users []uuid.UUID) error {
	for _, id := range users {
		w.UserID = id
		if err := AddWatcher(ctx, tx, w); err != nil {
			return err
		}
	}
	return nil
}
