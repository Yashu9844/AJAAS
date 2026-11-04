// Package tasksport is M3 Meetings' only view of M2 Projects & Tasks (docs/modules/m3-meetings/decisions.md DMT-05):
// action items are M2 tasks created and read as the calling user, so M2 decides every permission and stays the only
// owner of task data. The in-process adapter lives in internal/app; meetings packages never import projects packages.
package tasksport

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Actor is the user on whose behalf M3 calls M2.
type Actor struct {
	TenantID, UserID uuid.UUID
}

// NewTask is a task to create in a project.
type NewTask struct {
	ProjectID   uuid.UUID
	Title       string
	Description string
	AssigneeIDs []uuid.UUID
	DueAt       *time.Time
}

// Task is what M3 shows of a task.
type Task struct {
	ID          uuid.UUID   `json:"id"`
	ProjectID   uuid.UUID   `json:"project_id"`
	Key         string      `json:"key"`
	Title       string      `json:"title"`
	Status      string      `json:"status"` // column category: backlog, todo, in_progress, done, canceled
	DueAt       *time.Time  `json:"due_at"`
	AssigneeIDs []uuid.UUID `json:"assignee_ids"`
}

// Tasks creates and reads M2 tasks as the actor. Errors are M2's own (404 for an unreadable project or task, 403 for a
// missing permission) and are passed to the caller unchanged.
type Tasks interface {
	Create(ctx context.Context, a Actor, in NewTask) (Task, error)
	// Get returns the tasks the actor can read among ids; unreadable or deleted tasks are absent.
	Get(ctx context.Context, a Actor, ids []uuid.UUID) (map[uuid.UUID]Task, error)
}
