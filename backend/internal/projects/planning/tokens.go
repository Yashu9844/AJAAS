package planning

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

const (
	maxActiveTokens = 10
	maxTokenLen     = 64
	maxTokenLabel   = 60
	feedPath        = "/api/v1/calendar/feed.ics?token="
)

// CalendarToken authenticates one user iCalendar feed (VW-03). Only the SHA-256 hash is stored; Token and FeedPath are
// set once, in the response to creation.
type CalendarToken struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID   uuid.UUID  `gorm:"type:uuid" json:"-"`
	UserID     uuid.UUID  `gorm:"type:uuid" json:"-"`
	TokenHash  string     `json:"-"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	Token      string     `gorm:"-" json:"token,omitempty"`
	FeedPath   string     `gorm:"-" json:"feed_path,omitempty"`
}

// TableName maps to calendar_tokens.
func (CalendarToken) TableName() string { return "calendar_tokens" }

// hashToken is the stored form of a feed token.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newRawToken returns 32 random bytes, base64url encoded.
func newRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateToken issues a feed token for the caller; the raw token is returned only here (at most 10 active per user).
func (s *Service) CreateToken(ctx context.Context, a api.Actor, label string) (*CalendarToken, error) {
	label, err := text("label", label, 0, maxTokenLabel)
	if err != nil {
		return nil, err
	}
	var active int64
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM calendar_tokens WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL`,
		a.TenantID, a.UserID).Scan(&active).Error; err != nil {
		return nil, err
	}
	if active >= maxActiveTokens {
		return nil, kernel.Conflict("TOKEN_LIMIT", "revoke an unused calendar token first")
	}
	raw, err := newRawToken()
	if err != nil {
		return nil, err
	}
	t := CalendarToken{ID: uuid.New(), TenantID: a.TenantID, UserID: a.UserID, TokenHash: hashToken(raw), Label: label, CreatedAt: s.now().UTC()}
	if err := s.db.WithContext(ctx).Create(&t).Error; err != nil {
		return nil, err
	}
	s.audit(ctx, s.db, a, "calendar_token.created", ref{"calendar_token", t.ID})
	t.Token, t.FeedPath = raw, feedPath+raw
	return &t, nil
}

// ListTokens lists the caller active tokens (never the secrets). At most 10 can exist, so the list is not paged.
func (s *Service) ListTokens(ctx context.Context, a api.Actor) ([]CalendarToken, error) {
	var rows []CalendarToken
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND revoked_at IS NULL", a.TenantID, a.UserID).
		Order("created_at DESC, id DESC").Find(&rows).Error
	return rows, err
}

// RevokeToken disables one of the caller tokens at once; another user's token is a 404.
func (s *Service) RevokeToken(ctx context.Context, a api.Actor, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Exec(`UPDATE calendar_tokens SET revoked_at = ? WHERE id = ? AND tenant_id = ? AND user_id = ? AND revoked_at IS NULL`,
		s.now().UTC(), id, a.TenantID, a.UserID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return kernel.NotFound("calendar token")
	}
	s.audit(ctx, s.db, a, "calendar_token.revoked", ref{"calendar_token", id})
	return nil
}
