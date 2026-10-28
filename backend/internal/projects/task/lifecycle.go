package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

// Archive hides a task from default lists; archiving an archived task changes nothing (TK-10).
func (s *Service) Archive(ctx context.Context, a api.Actor, id uuid.UUID) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskUpdate, NoColumnLock: true, AllowArchived: true}, func(m *mut) error {
		return m.setArchived(true)
	})
}

// Restore brings an archived task back; restoring a live task changes nothing.
func (s *Service) Restore(ctx context.Context, a api.Actor, id uuid.UUID) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskUpdate, NoColumnLock: true, AllowArchived: true}, func(m *mut) error {
		return m.setArchived(false)
	})
}

func (m *mut) setArchived(archived bool) error {
	if archived == (m.t.ArchivedAt != nil) {
		return nil
	}
	if archived {
		now := m.s.now().UTC()
		m.change("archived_at", &now)
		m.add(note{event: "task.archived", activity: "archived"})
		return nil
	}
	m.change("archived_at", nil)
	m.add(note{event: "task.updated", activity: "restored", payload: map[string]any{"restored": true}})
	return nil
}

// Delete soft-deletes a task and its subtasks (task.delete); the retention job purges them later (TK-10).
func (s *Service) Delete(ctx context.Context, a api.Actor, id uuid.UUID) error {
	_, err := s.mutate(ctx, a, id, mutSpec{Perm: api.TaskDelete, NoColumnLock: true, AllowArchived: true, NoView: true}, func(m *mut) error {
		now := m.s.now().UTC()
		err := m.tx.Exec(`WITH RECURSIVE sub AS (
				SELECT id FROM tasks WHERE parent_task_id = ? AND tenant_id = ? AND deleted_at IS NULL
				UNION SELECT c.id FROM tasks c JOIN sub ON c.parent_task_id = sub.id WHERE c.tenant_id = ? AND c.deleted_at IS NULL)
			UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id IN (SELECT id FROM sub) AND tenant_id = ?`,
			m.t.ID, m.a.TenantID, m.a.TenantID, now, now, m.a.TenantID).Error
		if err != nil {
			return err
		}
		m.change("deleted_at", &now)
		m.add(note{event: "task.archived", activity: "deleted", payload: map[string]any{"deleted": true}})
		return nil
	})
	return err
}
