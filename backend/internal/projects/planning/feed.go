package planning

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Feed window and size: tasks due or starting from 90 days back to a year ahead, at most 1000 events.
const (
	feedDaysBack  = 90
	feedDaysAhead = 365
	maxFeedEvents = 1000
)

// FeedFor resolves a raw feed token to its user and renders that user iCalendar feed (VW-03). The tenant and the user
// come from the token row only; an unknown, revoked or malformed token, and a token whose owner is no longer active,
// all answer the same 404. Visibility is the owner's: only tasks assigned to them in projects they can read.
func (s *Service) FeedFor(ctx context.Context, raw string) (string, error) {
	tok, err := s.resolveToken(ctx, raw)
	if err != nil {
		return "", err
	}
	if err := s.ownerActive(ctx, tok); err != nil {
		return "", err
	}
	actor := api.Actor{TenantID: tok.TenantID, UserID: tok.UserID}
	ids, err := s.taskProjects(ctx, actor)
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	cards := []Card{}
	if len(ids) > 0 {
		if cards, err = s.feedTasks(ctx, actor, ids, now); err != nil {
			return "", err
		}
	}
	// Best effort: a failed bookkeeping write must not break calendar sync.
	_ = s.db.WithContext(ctx).Exec(`UPDATE calendar_tokens SET last_used_at = ? WHERE id = ? AND tenant_id = ?`, now, tok.ID, tok.TenantID).Error
	return buildICS(cards, now), nil
}

// resolveToken finds the active token row by hash and compares the hashes in constant time.
func (s *Service) resolveToken(ctx context.Context, raw string) (*CalendarToken, error) {
	if raw == "" || len(raw) > maxTokenLen {
		return nil, kernel.NotFound("calendar feed")
	}
	want := hashToken(raw)
	var tok CalendarToken
	err := s.db.WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL", want).Take(&tok).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("calendar feed")
	}
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(tok.TokenHash), []byte(want)) != 1 {
		return nil, kernel.NotFound("calendar feed")
	}
	return &tok, nil
}

// ownerActive fails closed when the token owner was deactivated or removed.
func (s *Service) ownerActive(ctx context.Context, tok *CalendarToken) error {
	if s.p.Users == nil {
		return nil
	}
	u, err := s.p.Users.GetByID(ctx, s.db, tok.TenantID, tok.UserID)
	if err != nil || u == nil || u.Status != "active" {
		return kernel.NotFound("calendar feed")
	}
	return nil
}

func (s *Service) feedTasks(ctx context.Context, a api.Actor, ids []uuid.UUID, now time.Time) ([]Card, error) {
	from, to := now.AddDate(0, 0, -feedDaysBack), now.AddDate(0, 0, feedDaysAhead)
	where := `t.tenant_id = ? AND t.project_id = ANY(?::uuid[]) AND ` + liveTask + ` AND ` + assignedTo +
		` AND ((t.due_at >= ? AND t.due_at < ?) OR (t.start_on >= ?::date AND t.start_on <= ?::date))`
	return cardQuery{where: where, args: []any{a.TenantID, idArray(ids), a.UserID, from, to, from.Format("2006-01-02"), to.Format("2006-01-02")},
		order: calendarOrder, limit: maxFeedEvents}.run(ctx, s.db)
}
