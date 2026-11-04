package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings"
	"github.com/jaas/jaas/internal/meetings/series"
	"github.com/jaas/jaas/internal/projects"
	papi "github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/task"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/tasksport"
)

// buildMeetings wires M3 Meetings; action items reach M2 through the in-process tasks adapter (DMT-05).
func buildMeetings(o Options, idp identityport.Port, pm *projects.Module) (*meetings.Module, error) {
	opts := meetings.Options{DB: o.DB, Publisher: o.Publisher, Logger: &o.Logger.Logger, Identity: idp, Background: o.Background,
		Tasks: tasksAdapter{svc: pm.Tasks()}}
	if n, ok := idp.(series.Names); ok {
		opts.Names = n
	}
	if o.Redis != nil {
		opts.RateLimits = o.Redis
	}
	return meetings.New(context.Background(), opts)
}

// tasksAdapter implements tasksport.Tasks over the M2 task service, acting as the given user.
type tasksAdapter struct{ svc *task.Service }

func (t tasksAdapter) actor(a tasksport.Actor) papi.Actor {
	return papi.Actor{TenantID: a.TenantID, UserID: a.UserID}
}

func toPort(v *task.View) tasksport.Task {
	return tasksport.Task{ID: v.ID, ProjectID: v.ProjectID, Key: v.Key, Title: v.Title, Status: v.ColumnCategory, DueAt: v.DueAt,
		AssigneeIDs: v.AssigneeIDs}
}

// Create adds a task to a project as the actor; M2 checks task:create.
func (t tasksAdapter) Create(ctx context.Context, a tasksport.Actor, in tasksport.NewTask) (tasksport.Task, error) {
	v, err := t.svc.Create(ctx, t.actor(a), in.ProjectID, task.CreateInput{Title: in.Title, Description: in.Description,
		AssigneeIDs: in.AssigneeIDs, DueAt: in.DueAt})
	if err != nil {
		return tasksport.Task{}, err
	}
	return toPort(v), nil
}

// Get returns the tasks the actor can read; unreadable and deleted tasks are left out.
func (t tasksAdapter) Get(ctx context.Context, a tasksport.Actor, ids []uuid.UUID) (map[uuid.UUID]tasksport.Task, error) {
	out := make(map[uuid.UUID]tasksport.Task, len(ids))
	for _, id := range ids {
		v, err := t.svc.Get(ctx, t.actor(a), id)
		var app *sharedErrors.AppError
		if errors.As(err, &app) && (app.StatusCode == 404 || app.StatusCode == 403) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if v != nil {
			out[id] = toPort(v)
		}
	}
	return out, nil
}
