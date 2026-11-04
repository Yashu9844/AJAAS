package actions

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/tasksport"
	"gorm.io/gorm"
)

// Add creates an M2 task as the caller (in the given project or the series default project) or links a task the caller
// can read, and stores the link (J5). The task is created first, in M2's own transaction; a retry with the same
// Idempotency-Key replays the whole response (shared idempotency middleware).
func (s *Service) Add(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in Input) (*Item, error) {
	if s.tasks == nil {
		return nil, errNoTasks
	}
	acc, oi, err := s.authz.Occurrence(ctx, s.db, a, occurrenceID)
	if err != nil {
		return nil, err
	}
	if !acc.ReadsContent() {
		return nil, kernel.NotFound("occurrence")
	}
	if err := acc.Require(api.CreateAction); err != nil {
		return nil, err
	}
	if oi.Status == "cancelled" {
		return nil, kernel.Conflict("OCCURRENCE_CANCELLED", "the occurrence is cancelled")
	}
	if err := s.checkAgendaItem(ctx, occurrenceID, in.AgendaItemID); err != nil {
		return nil, err
	}
	task, err := s.resolveTask(ctx, a, acc, in)
	if err != nil {
		return nil, err
	}
	l := &Link{ID: uuid.New(), TenantID: a.TenantID, OccurrenceID: occurrenceID, AgendaItemID: in.AgendaItemID, TaskID: task.ID,
		CreatedBy: a.UserID, SnapshotTitle: task.Title, SnapshotStatus: task.Status, SnapshotDueAt: task.DueAt}
	err = kernel.InTx(ctx, s.db, func(tx *gorm.DB) error {
		if err := tx.Create(l).Error; err != nil {
			return err
		}
		return kernel.Emit(ctx, tx, a.TenantID, "action_item.linked", map[string]any{"series_id": oi.SeriesID, "occurrence_id": occurrenceID,
			"task_id": task.ID})
	})
	if kernel.IsUniqueViolation(err) {
		return nil, kernel.Conflict("ALREADY_LINKED", "the task is already an action item of this occurrence")
	}
	if err != nil {
		return nil, err
	}
	return &Item{Link: *l, Task: &task}, nil
}

func (s *Service) resolveTask(ctx context.Context, a api.Actor, acc api.Access, in Input) (tasksport.Task, error) {
	if in.TaskID != nil {
		got, err := s.tasks.Get(ctx, port(a), []uuid.UUID{*in.TaskID})
		if err != nil {
			return tasksport.Task{}, err
		}
		t, ok := got[*in.TaskID]
		if !ok {
			return tasksport.Task{}, kernel.NotFound("task")
		}
		return t, nil
	}
	title, err := kernel.Text("title", in.Title, 1, 500)
	if err != nil {
		return tasksport.Task{}, err
	}
	project := in.ProjectID
	if project == nil {
		project = acc.Series.DefaultProjectID
	}
	if project == nil {
		return tasksport.Task{}, kernel.Unprocessable("NO_TASK_PROJECT",
			"choose a project for the task, or ask the organizer to set a default project for this meeting")
	}
	return s.tasks.Create(ctx, port(a), tasksport.NewTask{ProjectID: *project, Title: title, Description: in.Description,
		AssigneeIDs: in.AssigneeIDs, DueAt: in.DueAt})
}

func (s *Service) checkAgendaItem(ctx context.Context, occurrenceID uuid.UUID, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	var n int64
	if err := s.db.WithContext(ctx).Table("meeting_agenda_items").Where("id = ? AND occurrence_id = ?", *id, occurrenceID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("agenda_item_id", "must be an agenda item of this occurrence")
	}
	return nil
}

// Register mounts the action-item routes.
func (s *Service) Register(g *gin.RouterGroup) {
	g.GET("/occurrences/:id/action-items", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.List(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/occurrences/:id/action-items", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in Input) {
			v, err := s.Add(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.DELETE("/action-items/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.NoContent(c, s.Unlink(c.Request.Context(), a, id))
	}))
}
