// Package bridge is composition code: it binds the real task and collaboration services to the small interfaces the
// automation and recurrence contexts define, so those contexts never import the task context. Every call goes through the
// same use cases a person would run, so permissions, WIP limits, done rules and numbering apply unchanged.
package bridge

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/automation"
	"github.com/jaas/jaas/internal/projects/collab"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/recurrence"
	"github.com/jaas/jaas/internal/projects/task"
)

// retries is how often a change is repeated after a concurrent edit moved the task version.
const retries = 3

// Tasks implements automation.Tasks and recurrence.Tasks on the task and collaboration services.
type Tasks struct {
	task   *task.Service
	collab *collab.Service
}

// New binds the services.
func New(t *task.Service, c *collab.Service) *Tasks { return &Tasks{task: t, collab: c} }

var (
	_ automation.Tasks = (*Tasks)(nil)
	_ recurrence.Tasks = (*Tasks)(nil)
)

// State reads the task as the actor.
func (b *Tasks) State(ctx context.Context, a api.Actor, id uuid.UUID) (automation.TaskState, error) {
	v, err := b.task.Get(ctx, a, id)
	if err != nil {
		return automation.TaskState{}, err
	}
	return automation.TaskState{ID: v.ID, ProjectID: v.ProjectID, ColumnID: v.ColumnID, ColumnCategory: v.ColumnCategory, Priority: v.Priority,
		LabelIDs: v.LabelIDs, AssigneeIDs: v.AssigneeIDs}, nil
}

// Move puts the task in a column through the move use case (done rules and WIP limits apply).
func (b *Tasks) Move(ctx context.Context, a api.Actor, id, columnID uuid.UUID) error {
	return b.versioned(ctx, a, id, func(v int) error {
		_, err := b.task.Move(ctx, a, id, task.MoveInput{ColumnID: &columnID, Version: &v})
		return err
	})
}

// SetPriority edits the priority.
func (b *Tasks) SetPriority(ctx context.Context, a api.Actor, id uuid.UUID, priority string) error {
	return b.versioned(ctx, a, id, func(v int) error {
		_, err := b.task.Update(ctx, a, id, task.UpdateInput{Priority: &priority, Version: &v})
		return err
	})
}

// AddLabel labels the task.
func (b *Tasks) AddLabel(ctx context.Context, a api.Actor, id, labelID uuid.UUID) error {
	_, err := b.task.AddLabels(ctx, a, id, task.LabelIDsInput{LabelIDs: []uuid.UUID{labelID}})
	return err
}

// Assign assigns a project member to the task.
func (b *Tasks) Assign(ctx context.Context, a api.Actor, id, userID uuid.UUID) error {
	_, err := b.task.AddAssignees(ctx, a, id, task.UserIDsInput{UserIDs: []uuid.UUID{userID}})
	return err
}

// SetDue sets the due date.
func (b *Tasks) SetDue(ctx context.Context, a api.Actor, id uuid.UUID, due time.Time) error {
	return b.versioned(ctx, a, id, func(v int) error {
		_, err := b.task.Update(ctx, a, id, task.UpdateInput{DueAt: task.Opt[time.Time]{Set: true, Val: &due}, Version: &v})
		return err
	})
}

// Comment adds a comment authored by the actor.
func (b *Tasks) Comment(ctx context.Context, a api.Actor, id uuid.UUID, text string) error {
	_, err := b.collab.CreateComment(ctx, a, id, collab.CommentInput{Body: text})
	return err
}

// versioned reads the current version and runs fn, repeating when the version moved underneath.
func (b *Tasks) versioned(ctx context.Context, a api.Actor, id uuid.UUID, fn func(version int) error) error {
	var err error
	for i := 0; i < retries; i++ {
		var v *task.View
		if v, err = b.task.Get(ctx, a, id); err != nil {
			return err
		}
		if err = fn(v.Version); err == nil {
			return nil
		}
		if _, ok := kernel.AsVersionConflict(err); !ok {
			return err
		}
	}
	return err
}

// Template reads the template task for a recurrence.
func (b *Tasks) Template(ctx context.Context, a api.Actor, id uuid.UUID) (recurrence.Template, error) {
	v, err := b.task.Get(ctx, a, id)
	if err != nil {
		return recurrence.Template{}, err
	}
	return recurrence.Template{ID: v.ID, ProjectID: v.ProjectID, StartOn: v.StartOn, DueAt: v.DueAt, CreatedAt: v.CreatedAt}, nil
}

// Spawn duplicates the template (labels and assignees included), then titles the copy like the template and sets its
// dates. The copy exists after the first step; if the second fails the id is returned with the error.
func (b *Tasks) Spawn(ctx context.Context, a api.Actor, templateID uuid.UUID, d recurrence.Dates) (uuid.UUID, error) {
	tpl, err := b.task.Get(ctx, a, templateID)
	if err != nil {
		return uuid.Nil, err
	}
	cp, err := b.task.Duplicate(ctx, a, templateID, task.DuplicateInput{CopyAssignees: true, CopyLabels: true})
	if err != nil {
		return uuid.Nil, err
	}
	start, due := task.Opt[kernel.Date]{Set: true, Val: d.StartOn}, task.Opt[time.Time]{Set: true, Val: d.DueAt}
	_, err = b.task.Update(ctx, a, cp.ID, task.UpdateInput{Title: &tpl.Title, StartOn: start, DueAt: due, Version: &cp.Version})
	return cp.ID, err
}
