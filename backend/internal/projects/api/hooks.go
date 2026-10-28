package api

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskText is a task description that was just saved (on create or edit).
type TaskText struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	TaskID    uuid.UUID
	ActorID   uuid.UUID
	Text      string
}

// WatchRequest asks for users to watch a task for a reason (reporter, assignee).
type WatchRequest struct {
	TenantID uuid.UUID
	TaskID   uuid.UUID
	Reason   string
	UserIDs  []uuid.UUID
}

// TaskHooks lets the collaboration context react to task changes inside the task transaction: mentions in descriptions
// (CM-02) and automatic watchers (CM-03). The task context calls it when set and works without it.
type TaskHooks interface {
	// TaskTextSaved stores the readable mentions of the description and watches newly mentioned users.
	TaskTextSaved(ctx context.Context, tx *gorm.DB, t TaskText) error
	// WatchTask makes the users watchers of the task, keeping existing watches unchanged.
	WatchTask(ctx context.Context, tx *gorm.DB, w WatchRequest) error
}
