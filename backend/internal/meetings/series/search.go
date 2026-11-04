package series

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
)

// SearchHit is one series found by title or participant (SE-1).
type SearchHit struct {
	ID         uuid.UUID  `json:"id"`
	Title      string     `json:"title"`
	Visibility string     `json:"visibility"`
	Status     string     `json:"status"`
	Recurring  bool       `json:"recurring"`
	NextStart  *time.Time `json:"next_start"`
}

// SearchFilter narrows a search; at least one of Query and ParticipantID is required.
type SearchFilter struct {
	Query         string
	ParticipantID *uuid.UUID
}

// Search finds series the caller can read in full: by title words, by participant, or both. Meetings the caller sees
// only as a busy block or a title are not searchable, so search reveals nothing beyond the calendar.
func (s *Service) Search(ctx context.Context, a api.Actor, f SearchFilter) ([]SearchHit, error) {
	q, err := kernel.Text("q", f.Query, 0, 100)
	if err != nil {
		return nil, err
	}
	if q == "" && f.ParticipantID == nil {
		return nil, kernel.Invalid("q", "a search text or participant_id is required")
	}
	where := []string{"s.tenant_id = @t", "s.deleted_at IS NULL"}
	args := map[string]any{"t": a.TenantID, "u": a.UserID, "now": s.now().UTC()}
	readable := []string{memberExpr}
	if a.ManageAll {
		readable = append(readable, "TRUE")
	}
	if a.ReadAll {
		readable, args["rc"] = append(readable, "(NOT s.confidential OR @rc)"), a.ReadConfidential
	}
	where = append(where, "("+strings.Join(readable, " OR ")+")")
	if q != "" {
		where, args["q"] = append(where, "s.title ILIKE @q ESCAPE '\\'"), "%"+escapeLike(q)+"%"
	}
	if f.ParticipantID != nil {
		where, args["pid"] = append(where, "EXISTS (SELECT 1 FROM meeting_participants x WHERE x.series_id = s.id AND x.user_id = @pid)"), *f.ParticipantID
	}
	var hits []SearchHit
	err = s.db.WithContext(ctx).Raw(`SELECT s.id, s.title, s.visibility, s.status, s.rrule IS NOT NULL AS recurring,
		(SELECT min(o.start_at) FROM meeting_occurrences o WHERE o.series_id = s.id AND o.end_at > @now AND o.status <> 'cancelled') AS next_start
		FROM meeting_series s WHERE `+strings.Join(where, " AND ")+` ORDER BY next_start NULLS LAST, s.title LIMIT 50`, args).Scan(&hits).Error
	return hits, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// NeedsOrganizer lists active series whose organizer no longer takes part (DMT-15); manage_all only.
func (s *Service) NeedsOrganizer(ctx context.Context, a api.Actor) ([]SearchHit, error) {
	if !a.ManageAll {
		return nil, kernel.Forbidden("meetings:manage_all")
	}
	var hits []SearchHit
	err := s.db.WithContext(ctx).Raw(`SELECT s.id, s.title, s.visibility, s.status, s.rrule IS NOT NULL AS recurring FROM meeting_series s
		WHERE s.tenant_id = ? AND s.deleted_at IS NULL AND s.status = 'active' AND NOT EXISTS (SELECT 1 FROM meeting_participants p
		WHERE p.series_id = s.id AND p.user_id = s.organizer_id AND p.active) ORDER BY s.title LIMIT 200`, a.TenantID).Scan(&hits).Error
	return hits, err
}
