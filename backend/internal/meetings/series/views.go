package series

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"gorm.io/gorm"
)

// View is a series as the caller may see it: busy and internal levels hide everything but the slot or the title (DMT-12).
type View struct {
	ID               uuid.UUID       `json:"id"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	OrganizerID      *uuid.UUID      `json:"organizer_id"`
	OrganizerName    string          `json:"organizer_name"`
	TZ               string          `json:"tz"`
	Start            string          `json:"start"`
	DurationMin      int             `json:"duration_min"`
	AllDay           bool            `json:"all_day"`
	RRule            *string         `json:"rrule"`
	Visibility       string          `json:"visibility"`
	Confidential     bool            `json:"confidential"`
	Location         string          `json:"location"`
	VideoURL         string          `json:"video_url"`
	DefaultProjectID *uuid.UUID      `json:"default_project_id"`
	RecordingAllowed bool            `json:"recording_allowed"`
	Status           string          `json:"status"`
	Version          int             `json:"version"`
	Sequence         int             `json:"sequence"`
	Level            string          `json:"level"`
	Role             string          `json:"role"`
	Template         string          `json:"template"`
	Participant      bool            `json:"participant"`
	Perms            []string        `json:"permissions"`
	NextOccurrence   *OccurrenceView `json:"next_occurrence"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// OccurrenceView is one occurrence as the caller may see it.
type OccurrenceView struct {
	ID           uuid.UUID `json:"id"`
	SeriesID     uuid.UUID `json:"series_id"`
	Title        string    `json:"title"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	Status       string    `json:"status"`
	IsException  bool      `json:"is_exception"`
	CancelReason string    `json:"cancel_reason,omitempty"`
	Location     string    `json:"location"`
	Version      int       `json:"version"`
	Level        string    `json:"level"`
	Visibility   string    `json:"visibility"`
	Confidential bool      `json:"confidential"`
	Recurring    bool      `json:"recurring"`
	MyRSVP       string    `json:"my_rsvp,omitempty"`
}

const (
	busyTitle         = "Busy"
	confidentialTitle = "Confidential meeting"
)

func permList(m map[api.Perm]bool) []string {
	out := make([]string, 0, len(m))
	for p, ok := range m {
		if ok {
			out = append(out, string(p))
		}
	}
	sort.Strings(out)
	return out
}

func seriesView(row *Series, acc api.Access) *View {
	v := &View{ID: row.ID, TZ: row.TZ, Start: row.DTStartLocal.Format(LocalLayout), DurationMin: row.DurationMin, AllDay: row.AllDay,
		Visibility: row.Visibility, Confidential: row.Confidential, Status: row.Status, Version: row.Version, Sequence: row.Sequence,
		Level: acc.Level, Role: acc.Role, Template: acc.Template, Participant: acc.Participant, Perms: permList(acc.Perms),
		RRule: row.RRule, UpdatedAt: row.UpdatedAt}
	switch acc.Level {
	case api.LevelBusy:
		v.Title = busyTitle
		return v
	case api.LevelInternal:
		v.Title, v.Location = row.Title, row.Location
		if row.Confidential {
			v.Title, v.Location = confidentialTitle, ""
		}
		return v
	}
	org := row.OrganizerID
	v.Title, v.Description, v.OrganizerID, v.Location, v.VideoURL = row.Title, row.DescriptionMD, &org, row.Location, row.VideoURL
	v.DefaultProjectID, v.RecordingAllowed = row.DefaultProjectID, row.RecordingAllowed
	return v
}

// occurrenceView renders o with the series fields allowed at level.
func occurrenceView(o *Occurrence, row *Series, level string, now time.Time) OccurrenceView {
	v := OccurrenceView{ID: o.ID, SeriesID: o.SeriesID, StartAt: o.StartAt, EndAt: o.EndAt, Status: o.EffectiveStatus(now),
		IsException: o.IsException, Version: o.Version, Level: level, Visibility: row.Visibility, Confidential: row.Confidential,
		Recurring: row.RRule != nil}
	switch {
	case level == api.LevelBusy:
		v.Title = busyTitle
		return v
	case level == api.LevelInternal && row.Confidential:
		v.Title = confidentialTitle
		return v
	}
	v.Title, v.Location, v.CancelReason = row.Title, row.Location, o.CancelReason
	if o.TitleOverride != nil {
		v.Title = *o.TitleOverride
	}
	if o.LocationOverride != nil {
		v.Location = *o.LocationOverride
	}
	return v
}

// Get returns the series with its next occurrence; privileged reads of meetings the caller is not invited to are audited.
func (s *Service) Get(ctx context.Context, a api.Actor, id uuid.UUID) (*View, error) {
	row, err := loadSeries(ctx, s.db, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	acc, err := s.accessFor(ctx, s.db, a, row)
	if err != nil {
		return nil, err
	}
	s.auditRead(ctx, a, acc)
	v := seriesView(row, acc)
	if acc.Level == api.LevelFull || acc.Level == api.LevelAudit {
		org := s.names(ctx, a.TenantID, []uuid.UUID{row.OrganizerID})
		v.OrganizerName = org[row.OrganizerID]
	}
	next, err := s.nextOccurrence(ctx, s.db, row, acc.Level)
	v.NextOccurrence = next
	return v, err
}

func (s *Service) nextOccurrence(ctx context.Context, db *gorm.DB, row *Series, level string) (*OccurrenceView, error) {
	var o Occurrence
	res := db.WithContext(ctx).Where("series_id = ? AND end_at > ? AND status <> ? AND NOT orphaned", row.ID, s.now().UTC(), StatusCancelled).
		Order("start_at").Limit(1).Find(&o)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, res.Error
	}
	v := occurrenceView(&o, row, level, s.now())
	return &v, nil
}
