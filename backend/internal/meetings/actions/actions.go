// Package actions links meeting action items to M2 tasks (DMT-05, specification MR-10): a task is created or linked as
// the calling user through the tasks port, so M2 checks every permission; M3 stores the link and a display snapshot only.
package actions

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/inbox"
	"github.com/jaas/jaas/internal/shared/tasksport"
	"gorm.io/gorm"
)

// Link is one action item: an M2 task referenced from an occurrence.
type Link struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid" json:"-"`
	OccurrenceID   uuid.UUID  `gorm:"type:uuid" json:"occurrence_id"`
	AgendaItemID   *uuid.UUID `gorm:"type:uuid" json:"agenda_item_id"`
	TaskID         uuid.UUID  `gorm:"type:uuid" json:"task_id"`
	CreatedBy      uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	SnapshotTitle  string     `json:"-"`
	SnapshotStatus string     `json:"-"`
	SnapshotDueAt  *time.Time `json:"-"`
	StaleAt        *time.Time `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}

// TableName maps to meeting_action_links.
func (Link) TableName() string { return "meeting_action_links" }

// Item is an action item as shown: live from M2 when the viewer can read the task, otherwise restricted.
type Item struct {
	Link
	Restricted bool            `json:"restricted"`
	Stale      bool            `json:"stale"`
	Task       *tasksport.Task `json:"task"`
}

// Input creates a task (project_id, title...) or links an existing one (task_id).
type Input struct {
	TaskID       *uuid.UUID  `json:"task_id"`
	ProjectID    *uuid.UUID  `json:"project_id"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	AssigneeIDs  []uuid.UUID `json:"assignee_ids"`
	DueAt        *time.Time  `json:"due_at"`
	AgendaItemID *uuid.UUID  `json:"agenda_item_id"`
}

// Service implements the action-item context.
type Service struct {
	db    *gorm.DB
	authz api.Authorizer
	tasks tasksport.Tasks // nil: action items answer 503 until M2 is wired
}

// NewService builds the action-item service.
func NewService(db *gorm.DB, authz api.Authorizer, tasks tasksport.Tasks) *Service {
	return &Service{db: db, authz: authz, tasks: tasks}
}

func port(a api.Actor) tasksport.Actor {
	return tasksport.Actor{TenantID: a.TenantID, UserID: a.UserID}
}

var errNoTasks = kernel.Err(503, "TASKS_UNAVAILABLE", "the projects service is not available")

// List shows the action items of an occurrence; tasks the viewer cannot read are restricted (AI-3), and readable ones
// refresh the snapshot that other viewers fall back on.
func (s *Service) List(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]Item, error) {
	acc, _, err := s.authz.Occurrence(ctx, s.db, a, occurrenceID)
	if err != nil {
		return nil, err
	}
	if !acc.ReadsContent() {
		return nil, kernel.NotFound("occurrence")
	}
	var links []Link
	if err := s.db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID).Order("created_at, id").Find(&links).Error; err != nil {
		return nil, err
	}
	live := map[uuid.UUID]tasksport.Task{}
	if s.tasks != nil && len(links) > 0 {
		ids := make([]uuid.UUID, len(links))
		for i, l := range links {
			ids[i] = l.TaskID
		}
		if live, err = s.tasks.Get(ctx, port(a), ids); err != nil {
			return nil, err
		}
	}
	out := make([]Item, 0, len(links))
	for _, l := range links {
		it := Item{Link: l}
		if t, ok := live[l.TaskID]; ok {
			task := t
			it.Task = &task
			s.refresh(ctx, l, t)
		} else {
			it.Restricted, it.Stale = true, l.StaleAt != nil
		}
		out = append(out, it)
	}
	return out, nil
}

// refresh keeps the snapshot current, best effort (a failed bookkeeping write must not fail a read).
func (s *Service) refresh(ctx context.Context, l Link, t tasksport.Task) {
	if l.SnapshotTitle == t.Title && l.SnapshotStatus == t.Status {
		return
	}
	_ = s.db.WithContext(ctx).Model(&Link{}).Where("id = ?", l.ID).
		Updates(map[string]any{"snapshot_title": t.Title, "snapshot_status": t.Status, "snapshot_due_at": t.DueAt, "stale_at": nil}).Error
}

// TaskEvent is the part of an M2 task event the snapshot needs.
type TaskEvent struct {
	EventID   uuid.UUID
	EventType string // task.completed or task.deleted
	TenantID  uuid.UUID
	TaskID    uuid.UUID
}

// HandleTaskEvent marks snapshots stale when M2 reports a task change (AI-6); redeliveries are skipped by the inbox.
func (s *Service) HandleTaskEvent(ctx context.Context, e TaskEvent) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fresh, err := inbox.Claim(tx, kernel.InboxTable, "meetings.action_snapshots", e.TenantID, e.EventID, e.EventType)
		if err != nil || !fresh {
			return err
		}
		status := "done"
		if e.EventType == "task.deleted" {
			status = "deleted"
		}
		return tx.Model(&Link{}).Where("tenant_id = ? AND task_id = ?", e.TenantID, e.TaskID).
			Updates(map[string]any{"snapshot_status": status, "stale_at": time.Now().UTC()}).Error
	})
}

// Unlink removes an action item; the M2 task stays (DMT-05).
func (s *Service) Unlink(ctx context.Context, a api.Actor, id uuid.UUID) error {
	var l Link
	err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return kernel.NotFound("action item")
	}
	if err != nil {
		return err
	}
	acc, _, err := s.authz.Occurrence(ctx, s.db, a, l.OccurrenceID)
	if err != nil || !acc.ReadsContent() {
		return kernel.NotFound("action item")
	}
	if err := acc.Require(api.CreateAction); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&Link{}, "id = ?", l.ID).Error
}
