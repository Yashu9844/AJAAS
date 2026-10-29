package recurrence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// normalise validates the schedule and fills the defaults; first is the first run (or the zero time when unknown).
func normalise(in *Input, now time.Time) error {
	if in.Frequency != Daily && in.Frequency != Weekly && in.Frequency != Monthly {
		return kernel.Invalid("frequency", "must be daily, weekly or monthly")
	}
	if in.Interval == 0 {
		in.Interval = 1
	}
	if in.Interval < 1 || in.Interval > maxInterval {
		return kernel.Invalid("interval", "must be between 1 and 52")
	}
	if in.FirstRunAt != nil && !in.FirstRunAt.After(now) {
		return kernel.Invalid("first_run_at", "must be in the future")
	}
	probe := now
	if in.FirstRunAt != nil {
		probe = in.FirstRunAt.UTC()
	}
	if err := dayRules(in, probe); err != nil {
		return err
	}
	if in.UntilOn != nil && in.UntilOn.Before(kernelDate(probe)) {
		return kernel.Invalid("until_on", "must not be before the first run")
	}
	return nil
}

// dayRules checks weekday and day_of_month against the frequency and defaults them from the day of probe.
func dayRules(in *Input, probe time.Time) error {
	if in.Frequency != Weekly && in.Weekday != nil {
		return kernel.Invalid("weekday", "applies to weekly schedules only")
	}
	if in.Frequency != Monthly && in.DayOfMonth != nil {
		return kernel.Invalid("day_of_month", "applies to monthly schedules only")
	}
	if in.Frequency == Weekly {
		if in.Weekday == nil {
			w := int(probe.Weekday())
			in.Weekday = &w
		}
		if *in.Weekday < 0 || *in.Weekday > 6 {
			return kernel.Invalid("weekday", "must be between 0 (Sunday) and 6 (Saturday)")
		}
	}
	if in.Frequency == Monthly {
		if in.DayOfMonth == nil {
			d := probe.Day()
			in.DayOfMonth = &d
		}
		if *in.DayOfMonth < 1 || *in.DayOfMonth > 31 {
			return kernel.Invalid("day_of_month", "must be between 1 and 31")
		}
	}
	return nil
}

// List lists the recurrences of a project, oldest first (project.read); at most 20, so the list is not paged.
func (s *Service) List(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Recurrence, error) {
	if err := s.require(ctx, a, projectID, api.ProjectRead); err != nil {
		return nil, err
	}
	var rows []Recurrence
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// Create schedules copies of a template task of the project (project.update); the caller must be able to create tasks.
func (s *Service) Create(ctx context.Context, a api.Actor, projectID uuid.UUID, in Input) (*Recurrence, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ProjectUpdate); err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskCreate); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if err := normalise(&in, now); err != nil {
		return nil, err
	}
	r := Recurrence{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, TemplateTaskID: in.TemplateTaskID, Frequency: in.Frequency,
		Interval: in.Interval, Weekday: in.Weekday, DayOfMonth: in.DayOfMonth, UntilOn: in.UntilOn, Active: true, CreatedBy: a.UserID,
		Version: 1, CreatedAt: now, UpdatedAt: now}
	r.NextRunAt = firstAfter(now, r)
	if in.FirstRunAt != nil {
		r.NextRunAt = in.FirstRunAt.UTC()
	}
	if err := s.inTx(ctx, func(tx *gorm.DB) error { return s.insert(ctx, tx, a, &r) }); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) insert(ctx context.Context, tx *gorm.DB, a api.Actor, r *Recurrence) error {
	if err := tx.Exec(`SELECT id FROM projects WHERE id = ? AND tenant_id = ? FOR UPDATE`, r.ProjectID, a.TenantID).Error; err != nil {
		return err
	}
	var tpl int64
	if err := tx.Raw(`SELECT count(*) FROM tasks WHERE id = ? AND project_id = ? AND tenant_id = ? AND deleted_at IS NULL`,
		r.TemplateTaskID, r.ProjectID, a.TenantID).Scan(&tpl).Error; err != nil {
		return err
	}
	if tpl == 0 {
		return kernel.NotFound("template task")
	}
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM task_recurrences WHERE project_id = ? AND tenant_id = ?`, r.ProjectID, a.TenantID).Scan(&n).Error; err != nil {
		return err
	}
	if n >= maxPerProject {
		return kernel.Conflict("RECURRENCE_LIMIT", "a project has at most 20 recurring tasks")
	}
	if err := tx.Create(r).Error; err != nil {
		return err
	}
	s.audit(ctx, tx, a, "task_recurrence.created", ref{r.ID, map[string]any{"project_id": r.ProjectID, "template_task_id": r.TemplateTaskID, "frequency": r.Frequency}})
	return nil
}

// Patch edits a recurrence (project.update). Re-activating or changing the schedule clears the flag.
func (s *Service) Patch(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in Update) (*Recurrence, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var r Recurrence
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("recurrence")
		}
		if err != nil {
			return err
		}
		if in.Version != nil && *in.Version != r.Version {
			return &kernel.VersionConflict{Current: r.Version}
		}
		if err := s.applyUpdate(&r, in); err != nil {
			return err
		}
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "task_recurrence.updated", ref{r.ID, map[string]any{"project_id": projectID, "active": r.Active}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// applyUpdate lays the edit over r and re-validates the schedule.
func (s *Service) applyUpdate(r *Recurrence, in Update) error {
	now := s.now().UTC()
	probe := Input{TemplateTaskID: r.TemplateTaskID, Frequency: r.Frequency, Interval: r.Interval, Weekday: r.Weekday, DayOfMonth: r.DayOfMonth, UntilOn: r.UntilOn}
	changed := in.Frequency != nil || in.Interval != nil || in.Weekday != nil || in.DayOfMonth != nil
	if in.Frequency != nil {
		probe.Frequency, probe.Weekday, probe.DayOfMonth = *in.Frequency, nil, nil
	}
	if in.Interval != nil {
		probe.Interval = *in.Interval
	}
	if in.Weekday != nil {
		probe.Weekday = in.Weekday
	}
	if in.DayOfMonth != nil {
		probe.DayOfMonth = in.DayOfMonth
	}
	if in.UntilOn.Set {
		probe.UntilOn = in.UntilOn.Val
	}
	if err := normalise(&probe, now); err != nil {
		return err
	}
	r.Frequency, r.Interval, r.Weekday, r.DayOfMonth, r.UntilOn = probe.Frequency, probe.Interval, probe.Weekday, probe.DayOfMonth, probe.UntilOn
	switch {
	case in.NextRunAt != nil && !in.NextRunAt.After(now):
		return kernel.Invalid("next_run_at", "must be in the future")
	case in.NextRunAt != nil:
		r.NextRunAt = in.NextRunAt.UTC()
	case changed:
		r.NextRunAt = firstAfter(now, *r)
	}
	if in.Active != nil {
		r.Active = *in.Active
	}
	r.FlaggedCode, r.Version, r.UpdatedAt = nil, r.Version+1, now
	return nil
}

// Delete removes a recurrence; tasks it already created stay (project.update).
func (s *Service) Delete(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`DELETE FROM task_recurrences WHERE id = ? AND project_id = ? AND tenant_id = ?`, id, projectID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("recurrence")
		}
		s.audit(ctx, tx, a, "task_recurrence.deleted", ref{id, map[string]any{"project_id": projectID}})
		return nil
	})
}
