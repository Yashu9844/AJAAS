package series

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// ResponseView is one participant answer for an occurrence.
type ResponseView struct {
	UserID  uuid.UUID `json:"user_id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	RSVP    string    `json:"rsvp"`
	Comment string    `json:"comment"`
}

// OccurrenceDetail is an occurrence with its series and the answers of the participants.
type OccurrenceDetail struct {
	OccurrenceView
	Series    *View           `json:"series"`
	Responses []ResponseView  `json:"responses"`
	Counts    []ResponseCount `json:"counts"`
}

func (s *Service) loadOccurrence(ctx context.Context, db *gorm.DB, a api.Actor, id uuid.UUID) (*Occurrence, *Series, api.Access, error) {
	var o Occurrence
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, api.Access{}, kernel.NotFound("occurrence")
	}
	if err != nil {
		return nil, nil, api.Access{}, err
	}
	row, err := loadSeries(ctx, db, a.TenantID, o.SeriesID)
	if err != nil {
		return nil, nil, api.Access{}, kernel.NotFound("occurrence")
	}
	acc, err := s.accessFor(ctx, db, a, row)
	if err != nil {
		return nil, nil, api.Access{}, kernel.NotFound("occurrence")
	}
	return &o, row, acc, nil
}

// GetOccurrence returns one occurrence; answers are shown to content readers only.
func (s *Service) GetOccurrence(ctx context.Context, a api.Actor, id uuid.UUID) (*OccurrenceDetail, error) {
	o, row, acc, err := s.loadOccurrence(ctx, s.db, a, id)
	if err != nil {
		return nil, err
	}
	s.auditRead(ctx, a, acc)
	d := &OccurrenceDetail{OccurrenceView: occurrenceView(o, row, acc.Level, s.now()), Series: seriesView(row, acc),
		Responses: []ResponseView{}, Counts: []ResponseCount{}}
	if !acc.ReadsContent() {
		return d, nil
	}
	parts, err := s.participantViews(ctx, s.db, a.TenantID, row.ID)
	if err != nil {
		return nil, err
	}
	answers, err := occurrenceRSVP(ctx, s.db, row.ID, o.ID)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, p := range parts {
		if !p.Active {
			continue
		}
		r := ResponseView{UserID: p.UserID, Name: p.Name, Kind: p.Kind, RSVP: rsvpOr(answers[p.UserID].RSVP), Comment: answers[p.UserID].Comment}
		d.Responses = append(d.Responses, r)
		counts[r.RSVP]++
		if p.UserID == a.UserID {
			d.MyRSVP = r.RSVP
		}
	}
	for _, k := range []string{RSVPAccepted, RSVPTentative, RSVPDeclined, RSVPNeedsAction} {
		d.Counts = append(d.Counts, ResponseCount{RSVP: k, Count: counts[k]})
	}
	return d, nil
}

// OccurrenceInput changes one occurrence only ("this occurrence", MR-04): it becomes an exception.
type OccurrenceInput struct {
	StartAt  *time.Time `json:"start_at"`
	EndAt    *time.Time `json:"end_at"`
	Title    *string    `json:"title"`
	Location *string    `json:"location"`
	Reason   string     `json:"reason"`
	Version  int        `json:"version"`
}

// UpdateOccurrence reschedules or retitles one occurrence (J2 step 1).
func (s *Service) UpdateOccurrence(ctx context.Context, a api.Actor, id uuid.UUID, in OccurrenceInput) (*OccurrenceDetail, error) {
	reason, err := kernel.Text("reason", in.Reason, 0, 500)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		o, row, _, err := s.editableOccurrence(ctx, tx, a, editable{id, in.Version, requirer(api.Update)})
		if err != nil {
			return err
		}
		moved, err := applyOccurrence(o, in)
		if err != nil {
			return err
		}
		if err := s.saveOccurrence(ctx, tx, o); err != nil {
			return err
		}
		key := "meeting.updated"
		if moved {
			key = "meeting.rescheduled"
		}
		return s.occurrenceChanged(ctx, tx, a, occChange{row: row, o: o, key: key, reason: reason})
	})
	if err != nil {
		return nil, err
	}
	return s.GetOccurrence(ctx, a, id)
}

func applyOccurrence(o *Occurrence, in OccurrenceInput) (bool, error) {
	moved := false
	if in.StartAt != nil || in.EndAt != nil {
		start, end := o.StartAt, o.EndAt
		if in.StartAt != nil {
			end = in.StartAt.Add(o.EndAt.Sub(o.StartAt))
			start = *in.StartAt
		}
		if in.EndAt != nil {
			end = *in.EndAt
		}
		if !end.After(start) || end.Sub(start) > 24*time.Hour {
			return false, kernel.Invalid("end_at", "must be after start_at and at most 24 hours later")
		}
		o.StartAt, o.EndAt, moved = start.UTC(), end.UTC(), true
	}
	if in.Title != nil {
		t, err := kernel.Text("title", *in.Title, 1, 200)
		if err != nil {
			return false, err
		}
		o.TitleOverride = &t
	}
	if in.Location != nil {
		l, err := kernel.Text("location", *in.Location, 0, 300)
		if err != nil {
			return false, err
		}
		o.LocationOverride = &l
	}
	o.IsException = true
	return moved, nil
}

// editable is a write on an occurrence: its version and the permission check run before any change.
type editable struct {
	id      uuid.UUID
	version int
	allow   func(api.Access) error
}

// editableOccurrence loads an occurrence for a write: permission, then published and cancelled refusals, then version.
func (s *Service) editableOccurrence(ctx context.Context, tx *gorm.DB, a api.Actor, e editable) (*Occurrence, *Series, api.Access, error) {
	id, version := e.id, e.version
	o, row, acc, err := s.loadOccurrence(ctx, tx, a, id)
	if err != nil {
		return nil, nil, api.Access{}, err
	}
	if !acc.ReadsContent() {
		return nil, nil, api.Access{}, kernel.NotFound("occurrence")
	}
	if err := e.allow(acc); err != nil {
		return nil, nil, api.Access{}, err
	}
	switch o.Status {
	case StatusPublished:
		return nil, nil, api.Access{}, kernel.Conflict("OCCURRENCE_HAS_MINUTES", "the minutes of this occurrence are published")
	case StatusCancelled:
		return nil, nil, api.Access{}, kernel.Conflict("OCCURRENCE_CANCELLED", "the occurrence is cancelled")
	}
	if err := kernel.CheckVersion(version, o.Version); err != nil {
		return nil, nil, api.Access{}, err
	}
	return o, row, acc, nil
}

func (s *Service) saveOccurrence(ctx context.Context, tx *gorm.DB, o *Occurrence) error {
	o.Version++
	o.UpdatedAt = s.now().UTC()
	return tx.WithContext(ctx).Save(o).Error
}

func requirer(p api.Perm) func(api.Access) error {
	return func(acc api.Access) error { return acc.Require(p) }
}
