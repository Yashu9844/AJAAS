package webhooks

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Ports are the services the webhooks context needs from the platform.
type Ports struct {
	Audit      identityport.Auditor
	SecretsKey string // base64 32-byte key; empty leaves secret features answering 503
}

// Service holds the webhook, delivery and git-integration use cases.
type Service struct {
	db      *gorm.DB
	authz   api.Authorizer
	tasks   api.TaskResolver
	p       Ports
	sealer  *Sealer
	sender  Doer
	limiter gin.HandlerFunc
	now     func() time.Time
	jitter  func() float64
}

// NewService builds the service. A configured but invalid secrets key is an error; an empty one leaves secrets disabled.
func NewService(db *gorm.DB, authz api.Authorizer, tasks api.TaskResolver, p Ports) (*Service, error) {
	s := &Service{db: db, authz: authz, tasks: tasks, p: p, sender: NewSender(), now: time.Now, jitter: rand.Float64}
	s.limiter = newLimiter(incomingRatePerMinute, time.Minute)
	if p.SecretsKey != "" {
		sealer, err := NewSealer(p.SecretsKey)
		if err != nil {
			return nil, err
		}
		s.sealer = sealer
	}
	return s, nil
}

// SetSender replaces the outgoing sender (tests only; production always uses the guarded Sender).
func (s *Service) SetSender(d Doer) { s.sender = d }

// SetLimiter replaces the per-process limiter of the public integration route with a shared (Redis) one.
func (s *Service) SetLimiter(h gin.HandlerFunc) { s.limiter = h }

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

func (s *Service) stamp() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// ref names the audited resource and carries its metadata (ids and codes only; never URLs or secrets).
type ref struct {
	resource string
	id       uuid.UUID
	meta     map[string]any
}

// audit records a successful mutation; failures never fail the request (SECURITY 7).
func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, action string, r ref) {
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, r.resource, r.id.String(), r.meta, a.IP, a.UserAgent)
}

func (s *Service) require(ctx context.Context, a api.Actor, projectID uuid.UUID, p api.Perm) (api.Access, error) {
	acc, err := s.authz.Access(ctx, nil, a, projectID)
	if err != nil {
		return acc, err
	}
	return acc, acc.Require(p)
}

func countRows(db *gorm.DB, table, where string, args ...any) (int64, error) {
	var n int64
	err := db.Table(table).Where(where, args...).Count(&n).Error
	return n, err
}

func mapMissing(err error, what string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return kernel.NotFound(what)
	}
	return err
}

func (s *Service) secretsOff() error {
	if s.sealer == nil {
		return kernel.Err(http.StatusServiceUnavailable, "SECRETS_UNAVAILABLE", "webhook and integration secrets are not configured")
	}
	return nil
}

// newSealedSecret creates a secret and its encrypted form.
func (s *Service) newSealedSecret() (secret string, sealed []byte, err error) {
	if err = s.secretsOff(); err != nil {
		return "", nil, err
	}
	if secret, err = NewSecret(); err != nil {
		return "", nil, err
	}
	sealed, err = s.sealer.Seal([]byte(secret))
	return secret, sealed, err
}

// openSecret decrypts a stored secret; ok is false when secrets are off or the value cannot be opened.
func (s *Service) openSecret(sealed []byte) ([]byte, bool) {
	if s.sealer == nil {
		return nil, false
	}
	plain, err := s.sealer.Open(sealed)
	return plain, err == nil
}
