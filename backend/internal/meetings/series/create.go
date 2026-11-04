package series

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// Create schedules a series: the caller becomes its organizer, every invitee receives their M0 role on the series, and
// the occurrences are materialised in the same transaction (J1; SC-1, RC-1/2, PA-1/4).
func (s *Service) Create(ctx context.Context, a api.Actor, in CreateInput) (*View, error) {
	if !a.Create && !a.ManageAll {
		return nil, kernel.Forbidden("meetings:create")
	}
	t, err := parseTiming(in.Start, in.TZ, in.DurationMin, in.AllDay, in.RRule)
	if err != nil {
		return nil, err
	}
	f, err := checkFields(in.Title, in.Description, in.Visibility, in.Location, in.VideoURL)
	if err != nil {
		return nil, err
	}
	if len(in.Participants) > MaxParticipants {
		return nil, kernel.Invalid("participants", "at most 500")
	}
	row := newSeries(a, t, f, in)
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if e := tx.WithContext(ctx).Create(row).Error; e != nil {
			return e
		}
		if e := s.addOrganizer(ctx, tx, a, row); e != nil {
			return e
		}
		for _, p := range in.Participants {
			if p.UserID == a.UserID {
				continue // the creator is already the organizer
			}
			if e := s.addParticipant(ctx, tx, a, row, p); e != nil {
				return e
			}
		}
		if _, e := s.materialise(ctx, tx, row, time.Time{}); e != nil {
			return e
		}
		if e := s.logChange(ctx, tx, a, change{series: row.ID, action: "series.created"}); e != nil {
			return e
		}
		if e := s.audit(ctx, tx, a, "meeting.created", row.ID, nil); e != nil {
			return e
		}
		return kernel.Emit(ctx, tx, a.TenantID, "meeting.scheduled", map[string]any{"series_id": row.ID, "version": row.Version})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, a, row.ID)
}

func newSeries(a api.Actor, t timing, f fields, in CreateInput) *Series {
	id := uuid.New()
	row := &Series{ID: id, TenantID: a.TenantID, Title: f.title, DescriptionMD: f.description, OrganizerID: a.UserID, TZ: t.loc.String(),
		DTStartLocal: t.local, DurationMin: t.duration, AllDay: in.AllDay, Visibility: f.visibility, Confidential: in.Confidential,
		Location: f.location, VideoURL: f.video, DefaultProjectID: in.DefaultProjectID, RecordingAllowed: in.RecordingAllowed,
		ICalUID: id.String() + "@jaas", Status: SeriesActive, Version: 1, CreatedBy: a.UserID}
	if t.rule != nil {
		r := t.rule.String()
		row.RRule = &r
	}
	return row
}

func (s *Service) addOrganizer(ctx context.Context, tx *gorm.DB, a api.Actor, row *Series) error {
	return s.addParticipant(ctx, tx, a, row, ParticipantInput{UserID: a.UserID, Kind: KindRequired, Role: api.RoleOrganizer})
}
