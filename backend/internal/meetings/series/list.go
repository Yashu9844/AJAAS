package series

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
)

// maxListRows caps one window of occurrences.
const maxListRows = 2000

// memberExpr is true when the caller (@u) takes part in the series s.
const memberExpr = "EXISTS (SELECT 1 FROM meeting_participants p WHERE p.series_id = s.id AND p.user_id = @u AND p.active)"

// ListFilter selects occurrences in a window of at most 62 days (MR-13).
type ListFilter struct {
	From, To time.Time
	Mine     bool       // only meetings the caller takes part in (My meetings)
	SeriesID *uuid.UUID // only one series
}

type listRow struct {
	Occurrence
	STitle        string  `gorm:"column:s_title"`
	SLocation     string  `gorm:"column:s_location"`
	SVisibility   string  `gorm:"column:s_visibility"`
	SConfidential bool    `gorm:"column:s_confidential"`
	SRRule        *string `gorm:"column:s_rrule"`
	Member        bool    `gorm:"column:member"`
	MyRSVP        *string `gorm:"column:my_rsvp"`
	SeriesRSVP    *string `gorm:"column:series_rsvp"`
}

func (f ListFilter) check() error {
	if f.From.IsZero() || f.To.IsZero() || !f.To.After(f.From) {
		return kernel.Invalid("from", "from and to are required and to must be after from")
	}
	if f.To.Sub(f.From) > MaxListDays*24*time.Hour {
		return kernel.Invalid("to", "the window is at most 62 days")
	}
	return nil
}

// List returns the occurrences the caller may see in the window, ordered by start (J12). Rows are filtered in SQL by a
// readable predicate, never after paging (design.md section 3).
func (s *Service) List(ctx context.Context, a api.Actor, f ListFilter) ([]OccurrenceView, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	where := []string{"o.tenant_id = @t", "s.deleted_at IS NULL", "o.start_at < @to", "o.end_at > @from"}
	args := map[string]any{"t": a.TenantID, "u": a.UserID, "from": f.From.UTC(), "to": f.To.UTC()}
	readable := []string{memberExpr}
	if !f.Mine {
		if a.ManageAll {
			readable = append(readable, "TRUE")
		}
		if a.ReadAll {
			readable = append(readable, "(NOT s.confidential OR @rc)")
			args["rc"] = a.ReadConfidential
		}
		readable = append(readable, "s.visibility IN ('internal', 'busy_only')")
	}
	where = append(where, "("+strings.Join(readable, " OR ")+")")
	if f.SeriesID != nil {
		where, args["sid"] = append(where, "o.series_id = @sid"), *f.SeriesID
	}
	var rows []listRow
	err := s.db.WithContext(ctx).Raw(`SELECT o.*, s.title AS s_title, s.location AS s_location, s.visibility AS s_visibility,
		s.confidential AS s_confidential, s.rrule AS s_rrule, `+memberExpr+` AS member,
		(SELECT r.rsvp FROM meeting_responses r WHERE r.series_id = s.id AND r.occurrence_id = o.id AND r.user_id = @u) AS my_rsvp,
		(SELECT r.rsvp FROM meeting_responses r WHERE r.series_id = s.id AND r.occurrence_id IS NULL AND r.user_id = @u) AS series_rsvp
		FROM meeting_occurrences o JOIN meeting_series s ON s.id = o.series_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY o.start_at, o.id LIMIT `+strconv.Itoa(maxListRows), args).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]OccurrenceView, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].view(a, now))
	}
	return out, nil
}

func (r *listRow) view(a api.Actor, now time.Time) OccurrenceView {
	row := &Series{ID: r.SeriesID, Title: r.STitle, Location: r.SLocation, Visibility: r.SVisibility, Confidential: r.SConfidential, RRule: r.SRRule}
	level := api.LevelBusy
	switch {
	case r.Member || a.ManageAll:
		level = api.LevelFull
	case a.ReadAll && (!r.SConfidential || a.ReadConfidential):
		level = api.LevelAudit
	case r.SVisibility == api.VisibilityInternal:
		level = api.LevelInternal
	}
	v := occurrenceView(&r.Occurrence, row, level, now)
	if r.Member {
		v.MyRSVP = RSVPNeedsAction
		if r.SeriesRSVP != nil {
			v.MyRSVP = *r.SeriesRSVP
		}
		if r.MyRSVP != nil {
			v.MyRSVP = *r.MyRSVP
		}
	}
	return v
}
