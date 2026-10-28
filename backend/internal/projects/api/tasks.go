package api

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskRef is the part of a task other contexts need (comments, time, attachments, views).
type TaskRef struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Number    int
	Title     string
	ColumnID  uuid.UUID
	Version   int
	Archived  bool
}

// TaskResolver finds a task and the caller access to its project. The task context implements it; contexts that act on
// a task (collab, planning) depend on this interface instead of reading the task context. A task the caller cannot read
// is a 404, the same as an unknown id (PG4).
type TaskResolver interface {
	Task(ctx context.Context, db *gorm.DB, a Actor, taskID uuid.UUID) (TaskRef, Access, error)
}
