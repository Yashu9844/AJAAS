package series

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// UpdateInput edits the whole series ("all occurrences", MR-04); absent fields stay. Version is required.
type UpdateInput struct {
	Title            *string    `json:"title"`
	Description      *string    `json:"description"`
	Start            *string    `json:"start"`
	TZ               *string    `json:"tz"`
	DurationMin      *int       `json:"duration_min"`
	AllDay           *bool      `json:"all_day"`
	RRule            *string    `json:"rrule"` // "" turns the series into a single meeting
	Visibility       *string    `json:"visibility"`
	Confidential     *bool      `json:"confidential"`
	Location         *string    `json:"location"`
	VideoURL         *string    `json:"video_url"`
	DefaultProjectID *uuid.UUID `json:"default_project_id"`
	ClearProject     bool       `json:"clear_default_project"`
	RecordingAllowed *bool      `json:"recording_allowed"`
	Version          int        `json:"version"`
}

func (in UpdateInput) timingChanged() bool {
	return in.Start != nil || in.TZ != nil || in.DurationMin != nil || in.AllDay != nil || in.RRule != nil
}

func pick[T any](p *T, cur T) T {
	if p != nil {
		return *p
	}
	return cur
}

// Update edits the series; a schedule change regenerates the future occurrences that hold no data (MR-04).
func (s *Service) Update(ctx context.Context, a api.Actor, id uuid.UUID, in UpdateInput) (*View, error) {
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		row, _, err := s.writable(ctx, tx, a, id, api.Update)
		if err != nil {
			return err
		}
		if err := kernel.CheckVersion(in.Version, row.Version); err != nil {
			return err
		}
		if err := applyUpdate(row, in); err != nil {
			return err
		}
		row.Version, row.Sequence, row.UpdatedAt = row.Version+1, row.Sequence+1, s.now().UTC()
		if err := tx.WithContext(ctx).Save(row).Error; err != nil {
			return err
		}
		if in.timingChanged() {
			if err := s.regenerate(ctx, tx, row); err != nil {
				return err
			}
		}
		if err := s.logChange(ctx, tx, a, change{series: row.ID, action: "series.updated", fields: map[string]any{"version": row.Version}}); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, a, auditEntry{"meeting.updated", row.ID, nil}); err != nil {
			return err
		}
		return kernel.Emit(ctx, tx, a.TenantID, "meeting.updated", map[string]any{"series_id": row.ID, "version": row.Version})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, a, id)
}

func applyUpdate(row *Series, in UpdateInput) error {
	f, err := checkFields(fields{pick(in.Title, row.Title), pick(in.Description, row.DescriptionMD), pick(in.Visibility, row.Visibility),
		pick(in.Location, row.Location), pick(in.VideoURL, row.VideoURL)})
	if err != nil {
		return err
	}
	row.Title, row.DescriptionMD, row.Visibility, row.Location, row.VideoURL = f.title, f.description, f.visibility, f.location, f.video
	row.Confidential, row.RecordingAllowed = pick(in.Confidential, row.Confidential), pick(in.RecordingAllowed, row.RecordingAllowed)
	if in.DefaultProjectID != nil || in.ClearProject {
		row.DefaultProjectID = in.DefaultProjectID
	}
	if !in.timingChanged() {
		return nil
	}
	allDay := pick(in.AllDay, row.AllDay)
	startLayout := LocalLayout
	if allDay {
		startLayout = "2006-01-02"
	}
	rule := ""
	if row.RRule != nil {
		rule = *row.RRule
	}
	t, err := parseTiming(timingInput{start: pick(in.Start, row.DTStartLocal.Format(startLayout)), tz: pick(in.TZ, row.TZ),
		duration: pick(in.DurationMin, row.DurationMin), allDay: allDay, rule: pick(in.RRule, rule)})
	if err != nil {
		return err
	}
	row.TZ, row.DTStartLocal, row.DurationMin, row.AllDay, row.RRule = t.loc.String(), t.local, t.duration, allDay, nil
	if t.rule != nil {
		r := t.rule.String()
		row.RRule = &r
	}
	return nil
}

// hasData is true for an occurrence (alias o) that holds agenda, notes, minutes, decisions, actions or attendance.
const hasData = `(EXISTS (SELECT 1 FROM meeting_agenda_items x WHERE x.occurrence_id = o.id)
	OR EXISTS (SELECT 1 FROM meeting_note_versions x WHERE x.occurrence_id = o.id)
	OR EXISTS (SELECT 1 FROM meeting_minutes x WHERE x.occurrence_id = o.id)
	OR EXISTS (SELECT 1 FROM meeting_decisions x WHERE x.occurrence_id = o.id)
	OR EXISTS (SELECT 1 FROM meeting_action_links x WHERE x.occurrence_id = o.id)
	OR EXISTS (SELECT 1 FROM meeting_attendance x WHERE x.occurrence_id = o.id)
	OR EXISTS (SELECT 1 FROM meeting_private_points x WHERE x.occurrence_id = o.id))`

// regenerate replaces the future generated occurrences after a schedule change. Rows with data are kept: those still
// on the new schedule take the new duration, the others are cancelled and marked orphaned so their notes stay readable.
func (s *Service) regenerate(ctx context.Context, tx *gorm.DB, row *Series) error {
	now := s.now().UTC()
	if err := tx.WithContext(ctx).Exec(`DELETE FROM meeting_occurrences o WHERE o.series_id = ? AND o.start_at >= ? AND NOT o.is_exception
		AND NOT `+hasData, row.ID, now).Error; err != nil {
		return err
	}
	if _, err := s.materialise(ctx, tx, row, now); err != nil {
		return err
	}
	starts, _, err := expansion(row, s.windowEnd(row))
	if err != nil {
		return err
	}
	keep := make([]time.Time, 0, len(starts))
	for _, t := range starts {
		keep = append(keep, t.UTC())
	}
	base := func() *gorm.DB {
		return tx.WithContext(ctx).Model(&Occurrence{}).Where("series_id = ? AND start_at >= ? AND NOT is_exception AND status NOT IN ?",
			row.ID, now, []string{StatusPublished, StatusCancelled})
	}
	q := base()
	if len(keep) > 0 {
		if err := base().Where("recurrence_id IN ?", keep).
			Update("end_at", gorm.Expr("start_at + make_interval(mins => ?)", row.DurationMin)).Error; err != nil {
			return err
		}
		q = q.Where("recurrence_id NOT IN ?", keep)
	}
	return q.Updates(map[string]any{"status": StatusCancelled, "orphaned": true, "cancel_reason": "the series schedule changed; notes are kept",
		"version": gorm.Expr("version + 1")}).Error
}
