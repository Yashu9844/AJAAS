package series

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/meetings/rrule"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// SplitInput changes an occurrence and every later one ("this and following", MR-04, J2 step 3): the series ends before
// the occurrence and a new series starts at it with the changes applied.
type SplitInput struct {
	OccurrenceID uuid.UUID   `json:"occurrence_id"`
	Changes      UpdateInput `json:"changes"`
	Version      int         `json:"version"`
}

// Split ends the series before the chosen occurrence and returns the new series.
func (s *Service) Split(ctx context.Context, a api.Actor, id uuid.UUID, in SplitInput) (*View, error) {
	var newID uuid.UUID
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		old, _, err := s.writable(ctx, tx, a, id, api.Update)
		if err != nil {
			return err
		}
		if err := kernel.CheckVersion(in.Version, old.Version); err != nil {
			return err
		}
		at, err := s.splitPoint(ctx, tx, old, in.OccurrenceID)
		if err != nil {
			return err
		}
		next, err := s.splitRows(old, at, in.Changes)
		if err != nil {
			return err
		}
		newID = next.ID
		return s.applySplit(ctx, tx, a, splitPlan{old, next, at})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, a, newID)
}

// splitPoint is the original start of the chosen occurrence; it must be a later, generated occurrence of a recurring series.
func (s *Service) splitPoint(ctx context.Context, tx *gorm.DB, old *Series, occurrenceID uuid.UUID) (time.Time, error) {
	if old.RRule == nil {
		return time.Time{}, kernel.Conflict("NOT_RECURRING", "only a recurring meeting can be split")
	}
	var o Occurrence
	if err := tx.WithContext(ctx).Where("id = ? AND series_id = ?", occurrenceID, old.ID).Take(&o).Error; err != nil {
		return time.Time{}, kernel.NotFound("occurrence")
	}
	first, _, err := firstStart(old)
	if err != nil {
		return time.Time{}, err
	}
	if !o.RecurrenceID.After(first) {
		return time.Time{}, kernel.Conflict("SPLIT_AT_FIRST", "edit the whole series to change it from its first occurrence")
	}
	if o.RecurrenceID.Before(s.now()) {
		return time.Time{}, kernel.Conflict("OCCURRENCE_IN_PAST", "only a future occurrence can start a new schedule")
	}
	return o.RecurrenceID, nil
}

// splitRows ends old before at and builds the new series starting at at, with changes applied.
func (s *Service) splitRows(old *Series, at time.Time, changes UpdateInput) (*Series, error) {
	r, err := rrule.Parse(*old.RRule)
	if err != nil {
		return nil, err
	}
	first, loc, err := firstStart(old)
	if err != nil {
		return nil, err
	}
	rest := r
	if r.Count > 0 {
		before := len(r.Expand(first, at.Add(-time.Second), rrule.MaxCount))
		r.Count, rest.Count = before, r.Count-before
	} else {
		until := at.Add(-time.Second).UTC()
		r.Until = &until
	}
	oldRule, nextRule := r.String(), rest.String()
	old.RRule = &oldRule
	next := *old
	next.ID, next.SplitFrom, next.Version, next.Sequence, next.WindowUntil = uuid.New(), &old.ID, 1, 0, nil
	next.ICalUID, next.RRule, next.DTStartLocal = next.ID.String()+"@jaas", &nextRule, localFields(at.In(loc))
	next.CreatedAt, next.UpdatedAt = time.Time{}, time.Time{}
	if err := applyUpdate(&next, changes); err != nil {
		return nil, err
	}
	return &next, nil
}

func localFields(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}

// applySplit saves both series, moves the later occurrences that hold data to the new series, copies participants and
// their roles, and regenerates the new schedule.
type splitPlan struct {
	old, next *Series
	at        time.Time
}

func (s *Service) applySplit(ctx context.Context, tx *gorm.DB, a api.Actor, p splitPlan) error {
	old, next, at := p.old, p.next, p.at
	old.Version, old.Sequence, old.UpdatedAt = old.Version+1, old.Sequence+1, s.now().UTC()
	if err := tx.WithContext(ctx).Save(old).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(next).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(`UPDATE meeting_occurrences o SET series_id = ? WHERE o.series_id = ? AND o.recurrence_id >= ?
		AND (o.is_exception OR `+hasData+`)`, next.ID, old.ID, at).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(`DELETE FROM meeting_occurrences WHERE series_id = ? AND recurrence_id >= ?`, old.ID, at).Error; err != nil {
		return err
	}
	if _, err := s.materialise(ctx, tx, old, at); err != nil {
		return err
	}
	if err := s.copyParticipants(ctx, tx, a, old.ID, next.ID); err != nil {
		return err
	}
	if err := s.regenerate(ctx, tx, next); err != nil {
		return err
	}
	if err := s.logChange(ctx, tx, a, change{series: old.ID, action: "series.split", fields: map[string]any{"new_series_id": next.ID}}); err != nil {
		return err
	}
	if err := kernel.Emit(ctx, tx, a.TenantID, "meeting.updated", map[string]any{"series_id": old.ID, "version": old.Version}); err != nil {
		return err
	}
	return kernel.Emit(ctx, tx, a.TenantID, "meeting.scheduled", map[string]any{"series_id": next.ID, "version": next.Version, "split_from": old.ID})
}

func (s *Service) copyParticipants(ctx context.Context, tx *gorm.DB, a api.Actor, from, to uuid.UUID) error {
	if err := tx.WithContext(ctx).Exec(`INSERT INTO meeting_participants (tenant_id, series_id, user_id, kind, active, added_by)
		SELECT tenant_id, ?, user_id, kind, active, added_by FROM meeting_participants WHERE series_id = ?`, to, from).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(`INSERT INTO meeting_consents (tenant_id, series_id, user_id, state, method, decided_at)
		SELECT tenant_id, ?, user_id, state, method, decided_at FROM meeting_consents WHERE series_id = ?`, to, from).Error; err != nil {
		return err
	}
	roles, err := s.p.Resources.RolesOn(ctx, tx, ref(a.TenantID, from))
	if err != nil {
		return err
	}
	for _, r := range roles {
		in := identityport.RoleAssignment{ActorID: a.UserID, UserID: r.UserID, RoleID: r.RoleID, Source: r.Source}
		if err := s.p.Resources.AssignRole(ctx, tx, ref(a.TenantID, to), in); err != nil {
			return err
		}
	}
	return nil
}
