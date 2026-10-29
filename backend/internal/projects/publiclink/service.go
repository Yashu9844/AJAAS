package publiclink

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Ports are the services the public link context needs from the platform.
type Ports struct {
	Audit identityport.Auditor
}

// Service holds the link management and the public read.
type Service struct {
	db      *gorm.DB
	authz   api.Authorizer
	p       Ports
	now     func() time.Time
	limiter gin.HandlerFunc
}

// NewService builds the service with the default per-IP limiter for the public route.
func NewService(db *gorm.DB, authz api.Authorizer, p Ports) *Service {
	return &Service{db: db, authz: authz, p: p, now: time.Now, limiter: newLimiter(publicRatePerMinute, time.Minute)}
}

// SetLimiter replaces the default in-process limiter of the public route (for example with the shared Redis middleware).
func (s *Service) SetLimiter(h gin.HandlerFunc) { s.limiter = h }

// hashToken is the stored form of a token.
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

// ref names the audited link and carries its metadata (ids and flags only).
type ref struct {
	id   uuid.UUID
	meta map[string]any
}

func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, action string, r ref) {
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, "public_link", r.id.String(), r.meta, a.IP, a.UserAgent)
}

func (s *Service) require(ctx context.Context, a api.Actor, projectID uuid.UUID, p api.Perm) error {
	acc, err := s.authz.Access(ctx, nil, a, projectID)
	if err != nil {
		return err
	}
	return acc.Require(p)
}

// List lists the active links of a project, never their secrets (project.update).
func (s *Service) List(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Link, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var rows []Link
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ? AND revoked_at IS NULL", projectID, a.TenantID).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// Create issues a link (project.update); the raw token is returned only here and at most 3 links are active per project.
func (s *Service) Create(ctx context.Context, a api.Actor, projectID uuid.UUID, in Input) (*Link, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	raw, err := newRawToken()
	if err != nil {
		return nil, err
	}
	l := Link{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, TokenHash: hashToken(raw), ShowDescriptions: in.ShowDescriptions,
		CreatedBy: a.UserID, CreatedAt: s.now().UTC()}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT id FROM projects WHERE id = ? AND tenant_id = ? FOR UPDATE`, projectID, a.TenantID).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Raw(`SELECT count(*) FROM public_links WHERE project_id = ? AND tenant_id = ? AND revoked_at IS NULL`, projectID, a.TenantID).Scan(&n).Error; err != nil {
			return err
		}
		if n >= maxActiveLinks {
			return kernel.Conflict("LINK_LIMIT", "a project has at most 3 active public links; revoke one first")
		}
		if err := tx.Create(&l).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "public_link.created", ref{l.ID, map[string]any{"project_id": projectID, "show_descriptions": l.ShowDescriptions}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	l.Token, l.Path = raw, "/api/v1/public/projects/"+raw
	return &l, nil
}

// Revoke disables a link at once (project.update); an unknown or already revoked link is a 404.
func (s *Service) Revoke(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE public_links SET revoked_at = ? WHERE id = ? AND project_id = ? AND tenant_id = ? AND revoked_at IS NULL`,
			s.now().UTC(), id, projectID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("public link")
		}
		s.audit(ctx, tx, a, "public_link.revoked", ref{id, map[string]any{"project_id": projectID}})
		return nil
	})
}
