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

// StartTimer starts the caller's timer on a task. One running timer per user is a database constraint, so two racing
// starts leave exactly one running and the other gets 409 TIMER_RUNNING (TM-01, PG6).
func (s *Service) StartTimer(ctx context.Context, a api.Actor, taskID uuid.UUID) (*TimeEntry, error) {
	_, acc, err := s.taskAccess(ctx, a, taskID, api.TimeLog)
	if err != nil {
		return nil, err
	}
	if err := s.requireTracking(acc); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	e := &TimeEntry{ID: uuid.New(), TenantID: a.TenantID, TaskID: taskID, UserID: a.UserID, StartedAt: now, Source: "timer", CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		if isDup(err) {
			return nil, kernel.Conflict("TIMER_RUNNING", "you already have a running timer; stop it first")
		}
		return nil, err
	}
	return e, nil
}

// RunningTimer returns the caller's running timer, or nil when none runs.
func (s *Service) RunningTimer(ctx context.Context, a api.Actor) (*TimeEntry, error) {
	var e TimeEntry
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND ended_at IS NULL", a.TenantID, a.UserID).Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// StopTimer stops the caller's running timer and logs the elapsed minutes (TM-01). Stopping needs no project permission:
// the timer is the caller's own row, and a user who lost access must still be able to end it.
func (s *Service) StopTimer(ctx context.Context, a api.Actor) (*TimeEntry, error) {
	var out TimeEntry
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		end := s.now().UTC()
		res := tx.Raw(`UPDATE time_entries SET ended_at = GREATEST(?::timestamptz, started_at), updated_at = ?,
				minutes = LEAST(1440, GREATEST(1, ROUND(EXTRACT(EPOCH FROM (?::timestamptz - started_at)) / 60)::int))
			WHERE tenant_id = ? AND user_id = ? AND ended_at IS NULL RETURNING *`,
			end, end, end, a.TenantID, a.UserID).Scan(&out)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("running timer")
		}
		ref, err := s.taskRefOf(ctx, tx, a.TenantID, out.TaskID)
		if err != nil {
			return err
		}
		return s.announceTime(ctx, tx, a, timeFact{ref: ref, entry: &out})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// taskRefOf reads the ids and version an announcement needs without an access check (the caller owns the timer).
func (s *Service) taskRefOf(ctx context.Context, tx *gorm.DB, tenantID, taskID uuid.UUID) (api.TaskRef, error) {
	var r struct {
		ID, ProjectID uuid.UUID
		Version       int
	}
	err := tx.WithContext(ctx).Raw(`SELECT id, project_id, version FROM tasks WHERE id = ? AND tenant_id = ?`, taskID, tenantID).Scan(&r).Error
	if err == nil && r.ID == uuid.Nil {
		return api.TaskRef{}, kernel.NotFound("task")
	}
	return api.TaskRef{ID: r.ID, ProjectID: r.ProjectID, Version: r.Version}, err
}

// StopStaleTimers stops every timer that has run longer than olderThan, caps its span at MaxTimerAge and marks it
// auto_stopped so the owner can correct it (TM-03). It handles one batch of up to 500 timers and returns how many it
// stopped; the Run loop calls it on a schedule.
func (s *Service) StopStaleTimers(ctx context.Context, olderThan time.Duration) (int, error) {
	now := s.now().UTC()
	var stopped []TimeEntry
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Raw(`WITH stale AS (SELECT id FROM time_entries WHERE ended_at IS NULL AND started_at < ?
				ORDER BY started_at LIMIT 500 FOR UPDATE SKIP LOCKED)
			UPDATE time_entries t SET auto_stopped = TRUE, updated_at = ?,
				ended_at = LEAST(?::timestamptz, t.started_at + interval '24 hours'),
				minutes = LEAST(1440, GREATEST(1, ROUND(EXTRACT(EPOCH FROM (LEAST(?::timestamptz, t.started_at + interval '24 hours') - t.started_at)) / 60)::int))
			FROM stale WHERE t.id = stale.id RETURNING t.*`,
			now.Add(-olderThan), now, now, now).Scan(&stopped).Error; err != nil {
			return err
		}
		for i := range stopped {
			if err := s.announceStopped(ctx, tx, &stopped[i]); err != nil {
				return err
			}
		}
		return nil
	})
	return len(stopped), err
}

// announceStopped records a system stop in the activity log and the outbox on behalf of the timer owner.
func (s *Service) announceStopped(ctx context.Context, tx *gorm.DB, e *TimeEntry) error {
	ref, err := s.taskRefOf(ctx, tx, e.TenantID, e.TaskID)
	if err != nil {
		return err
	}
	tid := ref.ID
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: e.TenantID, ProjectID: ref.ProjectID, TaskID: &tid, ActorID: &e.UserID,
		Type: "time_logged", Source: "system", Data: map[string]any{"time_entry_id": e.ID, "minutes": e.Minutes, "auto_stopped": true}}); err != nil {
		return err
	}
	return kernel.Emit(ctx, tx, e.TenantID, "time.logged", map[string]any{"task_id": ref.ID, "project_id": ref.ProjectID,
		"time_entry_id": e.ID, "user_id": e.UserID})
}
