package task

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// fieldChange is one edited field. Free text (title, description) is recorded by name only.
type fieldChange struct {
	field    string
	old, new any
	text     bool
}

func (c fieldChange) entry() map[string]any {
	if c.text {
		return map[string]any{"field": c.field}
	}
	return map[string]any{"field": c.field, "old": c.old, "new": c.new}
}

// setField records a column change when the value differs.
func (m *mut) setField(chs *[]fieldChange, c fieldChange, col string) {
	m.change(col, c.new)
	*chs = append(*chs, c)
}

func (in UpdateInput) validate() error {
	if in.Version == nil {
		return kernel.Invalid("version", "is required (body or If-Match header)")
	}
	if in.Title != nil {
		if _, err := cleanTitle(*in.Title); err != nil {
			return err
		}
	}
	if in.Description != nil {
		if err := checkDescription(*in.Description); err != nil {
			return err
		}
	}
	if in.Priority != nil {
		if err := checkPriority(*in.Priority); err != nil {
			return err
		}
	}
	if in.EstimateMinutes.Set {
		return checkEstimate(in.EstimateMinutes.Val)
	}
	return nil
}

// Update edits a task with optimistic concurrency (TK-03): a stale version is a VersionConflict.
func (s *Service) Update(ctx context.Context, a api.Actor, id uuid.UUID, in UpdateInput) (*View, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	v, err := s.mutate(ctx, a, id, mutSpec{Perm: api.TaskUpdate, Version: in.Version, NoColumnLock: true}, func(m *mut) error {
		return m.applyUpdate(in)
	})
	if isCheck(err) {
		return nil, kernel.Invalid("due_at", "must not be before start_on")
	}
	return v, err
}

func (m *mut) applyUpdate(in UpdateInput) error {
	var chs []fieldChange
	m.textFields(in, &chs)
	if in.Priority != nil && *in.Priority != m.t.Priority {
		m.setField(&chs, fieldChange{field: "priority", old: m.t.Priority, new: *in.Priority}, "priority")
	}
	if err := m.scheduleFields(in, &chs); err != nil {
		return err
	}
	if err := m.linkFields(in, &chs); err != nil {
		return err
	}
	if len(chs) == 0 {
		return nil
	}
	if in.Description != nil && hasField(chs, "description") {
		if err := m.s.textSaved(m.ctx, m.tx, m.a, m.t, *in.Description); err != nil {
			return err
		}
	}
	entries := make([]map[string]any, len(chs))
	for i, c := range chs {
		entries[i] = c.entry()
	}
	m.add(note{event: "task.updated", activity: "updated", data: map[string]any{"changes": entries}})
	if hasField(chs, "due_at") {
		m.add(note{event: "task.due_changed"})
	}
	return nil
}

func hasField(chs []fieldChange, f string) bool { return fieldOf(chs, f).field != "" }

func fieldOf(chs []fieldChange, f string) fieldChange {
	for _, c := range chs {
		if c.field == f {
			return c
		}
	}
	return fieldChange{}
}

func (m *mut) textFields(in UpdateInput, chs *[]fieldChange) {
	if in.Title != nil {
		if t, _ := cleanTitle(*in.Title); t != m.t.Title {
			m.setField(chs, fieldChange{field: "title", new: t, text: true}, "title")
		}
	}
	if in.Description != nil && *in.Description != m.t.Description {
		m.setField(chs, fieldChange{field: "description", new: *in.Description, text: true}, "description")
	}
}

// scheduleFields applies start_on, due_at and the estimate, checking the merged dates.
func (m *mut) scheduleFields(in UpdateInput, chs *[]fieldChange) error {
	start, due := m.t.StartOn, m.t.DueAt
	if in.StartOn.Set {
		start = in.StartOn.Val
	}
	if in.DueAt.Set {
		due = in.DueAt.Val
	}
	if err := checkDates(start, due); err != nil {
		return err
	}
	if in.StartOn.Set && !sameDate(m.t.StartOn, start) {
		m.setField(chs, fieldChange{field: "start_on", old: m.t.StartOn, new: start}, "start_on")
	}
	if in.DueAt.Set && !sameTime(m.t.DueAt, due) {
		m.setField(chs, fieldChange{field: "due_at", old: m.t.DueAt, new: due}, "due_at")
	}
	if in.EstimateMinutes.Set && !sameInt(m.t.EstimateMinutes, in.EstimateMinutes.Val) {
		m.setField(chs, fieldChange{field: "estimate_minutes", old: m.t.EstimateMinutes, new: in.EstimateMinutes.Val}, "estimate_minutes")
	}
	return nil
}

// linkFields applies milestone, sprint and parent, validating each against the project.
func (m *mut) linkFields(in UpdateInput, chs *[]fieldChange) error {
	pid := m.t.ProjectID
	if in.MilestoneID.Set && !sameID(m.t.MilestoneID, in.MilestoneID.Val) {
		if err := checkMilestone(m.tx, m.a.TenantID, pid, in.MilestoneID.Val); err != nil {
			return err
		}
		m.setField(chs, fieldChange{field: "milestone_id", old: m.t.MilestoneID, new: in.MilestoneID.Val}, "milestone_id")
	}
	if in.SprintID.Set && !sameID(m.t.SprintID, in.SprintID.Val) {
		if err := checkSprint(m.tx, m.a.TenantID, pid, in.SprintID.Val); err != nil {
			return err
		}
		m.setField(chs, fieldChange{field: "sprint_id", old: m.t.SprintID, new: in.SprintID.Val}, "sprint_id")
	}
	if in.ParentTaskID.Set && !sameID(m.t.ParentTaskID, in.ParentTaskID.Val) {
		return m.reparent(in.ParentTaskID.Val, chs)
	}
	return nil
}

// reparent changes the parent of a task that has no subtasks of its own (so no descendant can become its ancestor).
func (m *mut) reparent(parent *uuid.UUID, chs *[]fieldChange) error {
	if parent != nil && *parent == m.t.ID {
		return kernel.Invalid("parent_task_id", "a task cannot be its own parent")
	}
	n, err := count(m.tx, `SELECT count(*) FROM tasks WHERE parent_task_id = ? AND tenant_id = ? AND deleted_at IS NULL`, m.t.ID, m.a.TenantID)
	if err != nil {
		return err
	}
	if n > 0 {
		return kernel.Conflict("TASK_HAS_SUBTASKS", "a task with subtasks cannot change its parent")
	}
	depth, err := checkParent(m.tx, m.a.TenantID, m.t.ProjectID, parent)
	if err != nil {
		return err
	}
	m.change("depth", depth)
	m.setField(chs, fieldChange{field: "parent_task_id", old: m.t.ParentTaskID, new: parent}, "parent_task_id")
	return nil
}

func sameID(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func sameInt(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func sameTime(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

func sameDate(a, b *kernel.Date) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Time.Equal(b.Time))
}
