// Package feed serves meetings to calendar applications (DMT-16, CS-1/CS-2): a personal iCalendar feed authenticated by
// a revocable token whose SHA-256 hash only is stored, and .ics downloads of one occurrence or one series.
package feed

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

const (
	maxActiveTokens = 10
	maxTokenLen     = 64
	feedPath        = "/api/v1/calendar/meetings.ics?token="
)

// Token authenticates one personal feed. Token and FeedPath are set once, in the response to creation.
type Token struct {
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

// TableName maps to meeting_feed_tokens.
func (Token) TableName() string { return "meeting_feed_tokens" }

// Service implements the feed context.
type Service struct {
	db    *gorm.DB
	authz api.Authorizer
	users identityport.Users
	audit identityport.Auditor
	now   func() time.Time
}

// NewService builds the feed service.
func NewService(db *gorm.DB, authz api.Authorizer, users identityport.Users, audit identityport.Auditor) *Service {
	return &Service{db: db, authz: authz, users: users, audit: audit, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// TokenInput labels a new token.
type TokenInput struct {
	Label string `json:"label"`
}

// CreateToken issues a feed token; the raw token is returned only here (at most 10 active per user).
func (s *Service) CreateToken(ctx context.Context, a api.Actor, in TokenInput) (*Token, error) {
	label, err := kernel.Text("label", in.Label, 0, 60)
	if err != nil {
		return nil, err
	}
	var active int64
	if err := s.db.WithContext(ctx).Model(&Token{}).Where("tenant_id = ? AND user_id = ? AND revoked_at IS NULL", a.TenantID, a.UserID).
		Count(&active).Error; err != nil {
		return nil, err
	}
	if active >= maxActiveTokens {
		return nil, kernel.Conflict("TOKEN_LIMIT", "revoke an unused calendar token first")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	t := &Token{ID: uuid.New(), TenantID: a.TenantID, UserID: a.UserID, TokenHash: hashToken(raw), Label: label, CreatedAt: s.now().UTC()}
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, s.db, a.TenantID.String(), a.UserID.String(), "meeting_feed_token.created", "meeting_feed_token", t.ID.String(), nil, a.IP, a.UserAgent)
	t.Token, t.FeedPath = raw, feedPath+raw
	return t, nil
}

// ListTokens lists the caller active tokens (never the secrets).
func (s *Service) ListTokens(ctx context.Context, a api.Actor) ([]Token, error) {
	var rows []Token
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND revoked_at IS NULL", a.TenantID, a.UserID).
		Order("created_at DESC, id DESC").Find(&rows).Error
	return rows, err
}

// RevokeToken disables one of the caller tokens at once (J8 step 4); another user's token is a 404.
func (s *Service) RevokeToken(ctx context.Context, a api.Actor, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Model(&Token{}).Where("id = ? AND tenant_id = ? AND user_id = ? AND revoked_at IS NULL", id, a.TenantID, a.UserID).
		Update("revoked_at", s.now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return kernel.NotFound("calendar token")
	}
	return s.audit.Log(ctx, s.db, a.TenantID.String(), a.UserID.String(), "meeting_feed_token.revoked", "meeting_feed_token", id.String(), nil, a.IP, a.UserAgent)
}
