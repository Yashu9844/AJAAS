package recurrence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

const claimBatch = 50

// Run ticks until ctx ends (module.go uses 60 s); a failed tick is reported and retried on the next one.
func (s *Service) Run(ctx context.Context, every time.Duration, onErr func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Tick(ctx); err != nil && onErr != nil {
				onErr(err)
			}
		}
	}
}

// due is one claimed occurrence: the recurrence and the scheduled time that is being run.
type due struct {
	rec   Recurrence
	runAt time.Time
}

// Tick runs every recurrence whose time has come and returns how many tasks it created. At most one task per recurrence
// per tick: a recurrence is claimed (next run moved ahead, past now) in a short transaction with SKIP LOCKED, so several
// instances never run the same occurrence, and missed occurrences collapse into this one catch-up run.
func (s *Service) Tick(ctx context.Context) (int, error) {
	claimed, err := s.claim(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	var failures []error
	for _, d := range claimed {
		ok, err := s.execute(ctx, d)
		if err != nil {
			failures = append(failures, err)
		}
		if ok {
			created++
		}
	}
	return created, errors.Join(failures...)
}

func (s *Service) claim(ctx context.Context) ([]due, error) {
	var out []due
	now := s.now().UTC()
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		var rows []Recurrence
		err := tx.Raw(`SELECT * FROM task_recurrences WHERE active AND next_run_at <= ? ORDER BY next_run_at, id LIMIT ? FOR UPDATE SKIP LOCKED`,
			now, claimBatch).Scan(&rows).Error
		if err != nil {
			return err
		}
		for _, r := range rows {
			d, err := s.advance(tx, r, now)
			if err != nil {
				return err
			}
			if d != nil {
				out = append(out, *d)
			}
		}
		return nil
	})
	return out, err
}

// advance moves one claimed recurrence to its next run, or ends it when the until date has passed; it returns the
// occurrence to run, or nil when the recurrence has ended without one.
func (s *Service) advance(tx *gorm.DB, r Recurrence, now time.Time) (*due, error) {
	next, active := advancePast(r.NextRunAt, now, r), true
	var run *due
	if r.UntilOn != nil && day(r.NextRunAt).After(r.UntilOn.Time) {
		active = false
	} else {
		run = &due{rec: r, runAt: r.NextRunAt}
		if r.UntilOn != nil && day(next).After(r.UntilOn.Time) {
			active = false
		}
	}
	err := tx.Exec(`UPDATE task_recurrences SET next_run_at = ?, active = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		next, active, now, r.ID, r.TenantID).Error
	return run, err
}

// execute creates the copy as the recurrence creator after re-checking that they still may; a recurrence that cannot
// run is flagged (and a missing template ends it) but keeps its schedule.
func (s *Service) execute(ctx context.Context, d due) (bool, error) {
	r := d.rec
	actor := api.Actor{TenantID: r.TenantID, UserID: r.CreatedBy}
	flag, err := s.blocked(ctx, actor, r)
	if err != nil {
		return false, err
	}
	if flag != "" {
		return false, s.flag(ctx, r, flag, flag == FlagTemplateGone)
	}
	id, err := s.spawn(ctx, actor, r, d.runAt)
	switch {
	case err != nil && id == uuid.Nil:
		return false, errors.Join(err, s.flag(ctx, r, FlagFailed, false))
	case err != nil:
		return true, s.finish(ctx, r, id, FlagDatesFailed)
	}
	return true, s.finish(ctx, r, id, "")
}

// blocked returns the flag that stops a run: the creator lost access, the project is archived or the template is gone.
func (s *Service) blocked(ctx context.Context, a api.Actor, r Recurrence) (string, error) {
	acc, err := s.access(ctx, a, r.ProjectID)
	if isNotFound(err) {
		return FlagNotAllowed, nil
	}
	if err != nil {
		return "", err
	}
	if acc.Project.Status == api.StatusArchived {
		return FlagProjectArchived, nil
	}
	if !acc.Member || !acc.Can(api.TaskCreate) {
		return FlagNotAllowed, nil
	}
	_, err = s.tasks.Template(ctx, a, r.TemplateTaskID)
	if isNotFound(err) {
		return FlagTemplateGone, nil
	}
	return "", err
}

func isNotFound(err error) bool {
	var app *sharedErrors.AppError
	return errors.As(err, &app) && app.StatusCode == 404
}

// spawn copies the template as the actor with the recurrence agent label and dates relative to the run.
func (s *Service) spawn(ctx context.Context, a api.Actor, r Recurrence, runAt time.Time) (uuid.UUID, error) {
	tpl, err := s.tasks.Template(ctx, a, r.TemplateTaskID)
	if err != nil {
		return uuid.Nil, err
	}
	return s.tasks.Spawn(kernel.WithAgent(ctx, agentOf(r.ID)), a, r.TemplateTaskID, datesFor(tpl, runAt))
}

// flag stores why a recurrence could not run; end deactivates it.
func (s *Service) flag(ctx context.Context, r Recurrence, code string, end bool) error {
	return s.db.WithContext(ctx).Exec(`UPDATE task_recurrences SET flagged_code = ?, active = active AND NOT ?::boolean, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		code, end, s.now().UTC(), r.ID, r.TenantID).Error
}

// finish records the created task; a flag is kept when only the dates failed.
func (s *Service) finish(ctx context.Context, r Recurrence, task uuid.UUID, flag string) error {
	var code *string
	if flag != "" {
		code = &flag
	}
	return s.db.WithContext(ctx).Exec(`UPDATE task_recurrences SET last_task_id = ?, last_run_at = ?, flagged_code = ? WHERE id = ? AND tenant_id = ?`,
		task, s.now().UTC(), code, r.ID, r.TenantID).Error
}

// RunNow creates the copy at once as the caller (project.update + task.create) without moving the schedule.
func (s *Service) RunNow(ctx context.Context, a api.Actor, projectID, id uuid.UUID) (uuid.UUID, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := acc.Require(api.ProjectUpdate); err != nil {
		return uuid.Nil, err
	}
	var r Recurrence
	err = s.db.WithContext(ctx).Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, kernel.NotFound("recurrence")
	}
	if err != nil {
		return uuid.Nil, err
	}
	task, err := s.spawn(ctx, a, r, s.now().UTC())
	if task != uuid.Nil {
		if ferr := s.finish(ctx, r, task, ""); ferr != nil {
			return task, ferr
		}
	}
	return task, err
}
