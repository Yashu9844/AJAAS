package task

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// note is one history entry plus its outbox event, written when the mutation commits (ids and versions only, PG12).
type note struct {
	event    string         // outbox routing key; empty writes activity only
	activity string         // activity type
	data     map[string]any // activity data: ids and enum values, never free text
	payload  map[string]any // extra event payload: ids only
}

// unit bundles what every write inside a transaction needs: the context, the transaction and the caller.
type unit struct {
	ctx context.Context
	tx  *gorm.DB
	a   api.Actor
}

// mut is one task mutation inside a transaction. The row is locked; operations record their effects and finish writes
// the single UPDATE, the version bump, the activity, the events and the audit entries.
type mut struct {
	ctx     context.Context
	tx      *gorm.DB
	s       *Service
	a       api.Actor
	acc     api.Access
	t       *Task
	col0    uuid.UUID // the column the task was in when the request was authorised
	sets    map[string]any
	dirty   bool
	pending []note
	warn    []string
}

// mutSpec says how a mutation is guarded.
type mutSpec struct {
	Perm          api.Perm
	Version       *int
	LockColumn    func(t *Task) uuid.UUID // column advisory-locked first (rank allocation, WIP count); default: the current column
	NoColumnLock  bool
	AllowArchived bool
	NoView        bool // the task is gone afterwards (delete): no view is returned
}

func lockColumn(tx *gorm.DB, columnID uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "task_col:"+columnID.String()).Error
}

func lockRow(tx *gorm.DB, tenantID, id uuid.UUID) (*Task, error) {
	var t Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("task")
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// mutate authorises, locks and runs fn; it returns the refreshed view.
func (s *Service) mutate(ctx context.Context, a api.Actor, id uuid.UUID, spec mutSpec, fn func(m *mut) error) (*View, error) {
	t0, acc, err := s.locate(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(spec.Perm); err != nil {
		return nil, err
	}
	var warns []string
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		m, err := s.begin(unit{ctx, tx, a}, acc, t0, spec)
		if err != nil {
			return err
		}
		if err := fn(m); err != nil {
			return err
		}
		warns = m.warn
		return m.finish()
	})
	if err != nil || spec.NoView {
		return nil, err
	}
	v, err := s.view(ctx, s.db, a.TenantID, id)
	if v != nil {
		v.Warnings = warns
	}
	return v, err
}

func (s *Service) begin(u unit, acc api.Access, t0 *Task, spec mutSpec) (*mut, error) {
	ctx, tx, a := u.ctx, u.tx, u.a
	switch {
	case spec.LockColumn != nil:
		if err := lockColumn(tx, spec.LockColumn(t0)); err != nil {
			return nil, err
		}
	case !spec.NoColumnLock:
		if err := lockColumn(tx, t0.ColumnID); err != nil {
			return nil, err
		}
	}
	t, err := lockRow(tx, a.TenantID, t0.ID)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != t0.ProjectID {
		return nil, kernel.Conflict("TASK_MOVED", "the task moved to another project; retry")
	}
	if !spec.NoColumnLock && spec.LockColumn == nil && t.ColumnID != t0.ColumnID {
		return nil, kernel.Conflict("TASK_MOVED", "the task moved to another column; retry")
	}
	if spec.Version != nil && *spec.Version != t.Version {
		return nil, &kernel.VersionConflict{Current: t.Version}
	}
	if t.ArchivedAt != nil && !spec.AllowArchived {
		return nil, kernel.Conflict("TASK_ARCHIVED", "the task is archived; restore it first")
	}
	return &mut{ctx: ctx, tx: tx, s: s, a: a, acc: acc, t: t, col0: t0.ColumnID, sets: map[string]any{}}, nil
}

// change records a column update that bumps the version.
func (m *mut) change(col string, v any) {
	m.sets[col] = v
	m.dirty = true
}

// quiet records a column update that leaves the version alone (a rank move inside a column).
func (m *mut) quiet(col string, v any) { m.sets[col] = v }

// touch marks the task changed without a column update (assignee and label rows).
func (m *mut) touch() { m.dirty = true }

func (m *mut) unit() unit { return unit{m.ctx, m.tx, m.a} }

func (m *mut) add(n note) { m.pending = append(m.pending, n) }

func (m *mut) finish() error {
	if m.dirty {
		m.sets["version"] = gorm.Expr("version + 1")
		m.sets["updated_at"] = m.s.now().UTC()
	}
	if len(m.sets) > 0 {
		if err := m.tx.Model(&Task{}).Where("id = ? AND tenant_id = ?", m.t.ID, m.a.TenantID).Updates(m.sets).Error; err != nil {
			return err
		}
	}
	if m.dirty {
		m.t.Version++
	}
	for _, n := range m.pending {
		if err := m.s.record(m.unit(), m.t, n); err != nil {
			return err
		}
	}
	return nil
}

// record writes the activity entry, the outbox event and the audit entry of one note.
func (s *Service) record(u unit, t *Task, n note) error {
	ctx, tx, a := u.ctx, u.tx, u.a
	actor, tid := a.UserID, t.ID
	if n.activity != "" {
		err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: t.ProjectID, TaskID: &tid, ActorID: &actor,
			Type: n.activity, Data: n.data})
		if err != nil {
			return err
		}
	}
	if n.event == "" {
		return nil
	}
	payload := map[string]any{"task_id": t.ID, "project_id": t.ProjectID, "version": t.Version}
	for k, v := range n.payload {
		payload[k] = v
	}
	if err := kernel.Emit(ctx, tx, a.TenantID, n.event, payload); err != nil {
		return err
	}
	s.audit(u, n.event, t.ID, map[string]any{"project_id": t.ProjectID, "version": t.Version})
	return nil
}
