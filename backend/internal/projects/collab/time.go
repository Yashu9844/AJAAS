package collab

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Time tracking limits (TM-01, TM-03).
const (
	// LockWindow is how long after its start an entry can still be edited or deleted by its author.
	LockWindow = 30 * 24 * time.Hour
	// MaxTimerAge is how long a timer may run before the sweep stops it.
	MaxTimerAge     = 24 * time.Hour
	maxEntryMinutes = 1440
	clockSkew       = 5 * time.Minute
)

// ManualTimeInput logs finished work; EndedAt is derived from StartedAt and Minutes.
type ManualTimeInput struct {
	StartedAt time.Time `json:"started_at"`
	Minutes   int       `json:"minutes"`
	Note      string    `json:"note"`
}

// TimeUpdateInput changes an entry; nil fields stay as they are.
type TimeUpdateInput struct {
	StartedAt *time.Time `json:"started_at"`
	Minutes   *int       `json:"minutes"`
	Note      *string    `json:"note"`
}

func (s *Service) requireTracking(acc api.Access) error {
	if !acc.Project.TimeTracking {
		return kernel.Conflict("TIME_TRACKING_DISABLED", "time tracking is turned off for this project")
	}
	return nil
}

// locked reports whether an entry that started at t is past the author's lock window.
func (s *Service) locked(t time.Time) bool { return t.Before(s.now().Add(-LockWindow)) }

func lockedError() error {
	return kernel.Conflict("TIME_LOCKED", "the entry is older than the lock window; ask a manager to change it")
}

func (s *Service) checkManual(in ManualTimeInput, manager bool) (string, error) {
	note, err := cleanText("note", in.Note, 0, maxNote)
	if err != nil {
		return "", err
	}
	if in.Minutes < 1 || in.Minutes > maxEntryMinutes {
		return "", kernel.Invalid("minutes", "must be between 1 and 1440")
	}
	if in.StartedAt.IsZero() || in.StartedAt.After(s.now().Add(clockSkew)) {
		return "", kernel.Invalid("started_at", "must be a time that has already passed")
	}
	if !manager && s.locked(in.StartedAt) {
		return "", lockedError()
	}
	return note, nil
}

// LogTime records finished work on a task (TM-01); time.log and an enabled project are required.
func (s *Service) LogTime(ctx context.Context, a api.Actor, taskID uuid.UUID, in ManualTimeInput) (*TimeEntry, error) {
	ref, acc, err := s.taskAccess(ctx, a, taskID, api.TimeLog)
	if err != nil {
		return nil, err
	}
	if err := s.requireTracking(acc); err != nil {
		return nil, err
	}
	note, err := s.checkManual(in, acc.Can(api.TimeManage))
	if err != nil {
		return nil, err
	}
	start := in.StartedAt.UTC()
	end := start.Add(time.Duration(in.Minutes) * time.Minute)
	e := &TimeEntry{ID: uuid.New(), TenantID: a.TenantID, TaskID: taskID, UserID: a.UserID, StartedAt: start, EndedAt: &end,
		Minutes: in.Minutes, Note: note, Source: "manual", CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC()}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(e).Error; err != nil {
			return err
		}
		return s.announceTime(ctx, tx, a, timeFact{ref: ref, entry: e})
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// timeFact is a finished entry to record in the activity log, the audit trail and the outbox.
type timeFact struct {
	ref   api.TaskRef
	entry *TimeEntry
}

func (s *Service) announceTime(ctx context.Context, tx *gorm.DB, a api.Actor, f timeFact) error {
	tid := f.ref.ID
	owner := f.entry.UserID
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: f.ref.ProjectID, TaskID: &tid, ActorID: &owner,
		Type: "time_logged", Data: map[string]any{"time_entry_id": f.entry.ID, "minutes": f.entry.Minutes}}); err != nil {
		return err
	}
	s.audit(ctx, tx, a, auditEntry{Action: "time.logged", Resource: "time_entry", ID: f.entry.ID, Meta: map[string]any{"task_id": f.ref.ID}})
	return kernel.Emit(ctx, tx, a.TenantID, "time.logged", map[string]any{"task_id": f.ref.ID, "project_id": f.ref.ProjectID,
		"time_entry_id": f.entry.ID, "user_id": f.entry.UserID})
}

type timeCursor struct {
	At time.Time `json:"a"`
	ID uuid.UUID `json:"i"`
}

// TimeFilter pages a task's entries; Mine limits them to the caller's own.
type TimeFilter struct {
	Page PageRequest
	Mine bool
}

// ListTime pages a task's entries newest first.
func (s *Service) ListTime(ctx context.Context, a api.Actor, taskID uuid.UUID, f TimeFilter) ([]TimeEntry, string, error) {
	pr, mine := f.Page, f.Mine
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return nil, "", err
	}
	var cur timeCursor
	if err := kernel.DecodeCursor(pr.Cursor, &cur); err != nil {
		return nil, "", err
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", a.TenantID, taskID)
	if mine {
		q = q.Where("user_id = ?", a.UserID)
	}
	if cur.ID != uuid.Nil {
		q = q.Where("(started_at, id) < (?, ?)", cur.At, cur.ID)
	}
	var rows []TimeEntry
	if err := q.Order("started_at DESC, id DESC").Limit(pr.limit() + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, next := page(rows, pr.limit(), func(e TimeEntry) any { return timeCursor{At: e.StartedAt, ID: e.ID} })
	return rows, next, nil
}

func (s *Service) loadEntry(ctx context.Context, tenantID, id uuid.UUID) (TimeEntry, error) {
	var e TimeEntry
	err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return e, kernel.NotFound("time entry")
	}
	return e, err
}

// entryAccess authorises an edit or delete: the author needs time.log, anyone else time.manage; only managers get past
// the lock window (TM-01).
func (s *Service) entryAccess(ctx context.Context, a api.Actor, e TimeEntry) (api.Access, error) {
	_, acc, err := s.taskAccess(ctx, a, e.TaskID, api.TaskRead)
	if err != nil || acc.Can(api.TimeManage) {
		return acc, err
	}
	if e.UserID != a.UserID {
		return acc, acc.Require(api.TimeManage)
	}
	if err := acc.Require(api.TimeLog); err != nil {
		return acc, err
	}
	if s.locked(e.StartedAt) {
		return acc, lockedError()
	}
	return acc, nil
}

// UpdateTime changes a finished entry (TM-01). A running timer must be stopped first.
func (s *Service) UpdateTime(ctx context.Context, a api.Actor, id uuid.UUID, in TimeUpdateInput) (*TimeEntry, error) {
	e, err := s.loadEntry(ctx, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	acc, err := s.entryAccess(ctx, a, e)
	if err != nil {
		return nil, err
	}
	if e.EndedAt == nil {
		return nil, kernel.Conflict("TIMER_RUNNING", "stop the timer before editing the entry")
	}
	if err = s.applyUpdate(&e, in, acc.Can(api.TimeManage)); err != nil {
		return nil, err
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE time_entries SET started_at = ?, ended_at = ?, minutes = ?, note = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			e.StartedAt, e.EndedAt, e.Minutes, e.Note, e.UpdatedAt, e.ID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		s.audit(ctx, tx, a, auditEntry{Action: "time.updated", Resource: "time_entry", ID: e.ID, Meta: map[string]any{"task_id": e.TaskID}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// applyUpdate validates the changes and rewrites the entry fields in memory.
func (s *Service) applyUpdate(e *TimeEntry, in TimeUpdateInput, manager bool) error {
	cand := ManualTimeInput{StartedAt: e.StartedAt, Minutes: e.Minutes, Note: e.Note}
	if in.StartedAt != nil {
		cand.StartedAt = *in.StartedAt
	}
	if in.Minutes != nil {
		cand.Minutes = *in.Minutes
	}
	if in.Note != nil {
		cand.Note = *in.Note
	}
	note, err := s.checkManual(cand, manager)
	if err != nil {
		return err
	}
	end := cand.StartedAt.UTC().Add(time.Duration(cand.Minutes) * time.Minute)
	e.StartedAt, e.EndedAt, e.Minutes, e.Note, e.UpdatedAt = cand.StartedAt.UTC(), &end, cand.Minutes, note, s.now().UTC()
	return nil
}

// DeleteTime removes an entry, including a running timer the author wants to discard (TM-01).
func (s *Service) DeleteTime(ctx context.Context, a api.Actor, id uuid.UUID) error {
	e, err := s.loadEntry(ctx, a.TenantID, id)
	if err != nil {
		return err
	}
	if _, err = s.entryAccess(ctx, a, e); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`DELETE FROM time_entries WHERE id = ? AND tenant_id = ?`, e.ID, a.TenantID)
		if res.Error != nil || res.RowsAffected == 0 {
			return orNotFound(res.Error, "time entry")
		}
		s.audit(ctx, tx, a, auditEntry{Action: "time.deleted", Resource: "time_entry", ID: e.ID, Meta: map[string]any{"task_id": e.TaskID}})
		return nil
	})
}
