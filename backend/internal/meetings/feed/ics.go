package feed

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/ical"
	"gorm.io/gorm"
)

const (
	stamp       = "20060102T150405Z"
	feedBack    = 30 * 24 * time.Hour
	feedAhead   = 365 * 24 * time.Hour
	maxEvents   = 2000
	busySubject = "Busy"
)

// event is one occurrence row joined with its series, as the feed renders it.
type event struct {
	ID            uuid.UUID
	SeriesID      uuid.UUID
	StartAt       time.Time
	EndAt         time.Time
	Status        string
	Title         string
	TitleOverride *string
	Location      string
	VideoURL      string
	Confidential  bool
	Sequence      int
	UpdatedAt     time.Time
	Busy          bool // render as a busy block only
}

const eventColumns = `o.id, o.series_id, o.start_at, o.end_at, o.status, o.title_override, o.updated_at,
	s.title, s.location, s.video_url, s.confidential, s.sequence`

// FeedFor renders the personal feed of a token owner (J8): the meetings they take part in, from 30 days back to a year
// ahead. An unknown, revoked or malformed token, and a token whose owner is inactive, answer the same 404.
func (s *Service) FeedFor(ctx context.Context, raw string) (string, error) {
	tok, err := s.resolve(ctx, raw)
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	var rows []event
	err = s.db.WithContext(ctx).Raw(`SELECT `+eventColumns+` FROM meeting_occurrences o JOIN meeting_series s ON s.id = o.series_id
		WHERE o.tenant_id = ? AND s.deleted_at IS NULL AND o.start_at < ? AND o.end_at > ?
		AND EXISTS (SELECT 1 FROM meeting_participants p WHERE p.series_id = s.id AND p.user_id = ? AND p.active)
		ORDER BY o.start_at LIMIT ?`, tok.TenantID, now.Add(feedAhead), now.Add(-feedBack), tok.UserID, maxEvents).Scan(&rows).Error
	if err != nil {
		return "", err
	}
	for i := range rows {
		rows[i].Busy = rows[i].Confidential // confidential titles never leave the system (DMT-12)
	}
	_ = s.db.WithContext(ctx).Model(&Token{}).Where("id = ?", tok.ID).Update("last_used_at", now).Error
	return render(rows, now, "JAAS meetings"), nil
}

func (s *Service) resolve(ctx context.Context, raw string) (*Token, error) {
	if raw == "" || len(raw) > maxTokenLen {
		return nil, kernel.NotFound("calendar feed")
	}
	want := hashToken(raw)
	var tok Token
	err := s.db.WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL", want).Take(&tok).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && subtle.ConstantTimeCompare([]byte(tok.TokenHash), []byte(want)) != 1) {
		return nil, kernel.NotFound("calendar feed")
	}
	if err != nil {
		return nil, err
	}
	u, err := s.users.GetByID(ctx, s.db, tok.TenantID, tok.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active() {
		return nil, kernel.NotFound("calendar feed")
	}
	return &tok, nil
}

// Occurrence renders one occurrence as a .ics download (CS-2); busy and internal levels get only what they may see.
func (s *Service) Occurrence(ctx context.Context, a api.Actor, id uuid.UUID) (string, error) {
	acc, _, err := s.authz.Occurrence(ctx, s.db, a, id)
	if err != nil {
		return "", err
	}
	return s.renderWhere(ctx, acc, "o.id = ?", id)
}

// Series renders the occurrences of a series in the feed window as a .ics download.
func (s *Service) Series(ctx context.Context, a api.Actor, id uuid.UUID) (string, error) {
	acc, err := s.authz.Access(ctx, s.db, a, id)
	if err != nil {
		return "", err
	}
	return s.renderWhere(ctx, acc, "o.series_id = ? AND o.end_at > now() - interval '30 days'", id)
}

func (s *Service) renderWhere(ctx context.Context, acc api.Access, where string, id uuid.UUID) (string, error) {
	var rows []event
	if err := s.db.WithContext(ctx).Raw(`SELECT `+eventColumns+` FROM meeting_occurrences o JOIN meeting_series s ON s.id = o.series_id
		WHERE `+where+` ORDER BY o.start_at LIMIT ?`, id, maxEvents).Scan(&rows).Error; err != nil {
		return "", err
	}
	busy := !acc.ReadsContent()
	for i := range rows {
		rows[i].Busy = busy
	}
	name := acc.Series.Title
	if busy {
		name = "JAAS meeting" // the calendar name must not reveal what the events hide
	}
	return render(rows, s.now().UTC(), name), nil
}

func render(rows []event, now time.Time, name string) string {
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//JAAS//Meetings//EN", "CALSCALE:GREGORIAN", "METHOD:PUBLISH",
		"X-WR-CALNAME:" + ical.Escape(name)}
	for _, e := range rows {
		lines = append(lines, eventLines(e, now)...)
	}
	lines = append(lines, "END:VCALENDAR")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(ical.Fold(l))
		b.WriteString("\r\n")
	}
	return b.String()
}

func eventLines(e event, now time.Time) []string {
	status := "CONFIRMED"
	if e.Status == "cancelled" {
		status = "CANCELLED"
	}
	title := e.Title
	if e.TitleOverride != nil {
		title = *e.TitleOverride
	}
	lines := []string{"BEGIN:VEVENT", "UID:" + e.ID.String() + "@jaas", "DTSTAMP:" + now.Format(stamp),
		"DTSTART:" + e.StartAt.UTC().Format(stamp), "DTEND:" + e.EndAt.UTC().Format(stamp), "STATUS:" + status,
		"LAST-MODIFIED:" + e.UpdatedAt.UTC().Format(stamp)}
	if e.Busy {
		return append(lines, "SUMMARY:"+busySubject, "CLASS:CONFIDENTIAL", "TRANSP:OPAQUE", "END:VEVENT")
	}
	lines = append(lines, "SUMMARY:"+ical.Escape(title))
	if e.Location != "" {
		lines = append(lines, "LOCATION:"+ical.Escape(e.Location))
	}
	if e.VideoURL != "" {
		lines = append(lines, "URL:"+e.VideoURL)
	}
	return append(lines, "END:VEVENT")
}

func writeICS(c *gin.Context, name, body string, err error) {
	if err != nil {
		kernel.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `inline; filename="`+name+`"`)
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", []byte(body))
}
