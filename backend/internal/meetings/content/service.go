// Package content holds what happens inside an occurrence: agenda items, private talking points, notes with versions,
// published minutes, decisions and attendance (docs/modules/m3-meetings/specification.md MR-07..MR-09). It depends on the
// api contract only; the series context is reached through api.Authorizer and the Lifecycle interface.
package content

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Lifecycle is what content needs from the series context besides authorization.
type Lifecycle interface {
	// MarkPublished sets the occurrence status after its minutes were published, inside tx.
	MarkPublished(ctx context.Context, tx *gorm.DB, occurrenceID uuid.UUID) error
}

// Names resolves display names; nil means none are shown.
type Names interface {
	DisplayNames(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Ports are the collaborators of the content context.
type Ports struct {
	Authz     api.Authorizer
	Lifecycle Lifecycle
	Audit     identityport.Auditor
	Names     Names
}

// Service implements the content context.
type Service struct {
	db  *gorm.DB
	p   Ports
	now func() time.Time
}

// NewService builds the content service.
func NewService(db *gorm.DB, p Ports) *Service {
	if p.Authz == nil || p.Lifecycle == nil || p.Audit == nil {
		panic("meetings: content.NewService needs the authorizer, the lifecycle and the audit port")
	}
	return &Service{db: db, p: p, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// scope is a resolved occurrence with the caller access.
type scope struct {
	a   api.Actor
	acc api.Access
	occ api.OccurrenceInfo
}

// occurrence resolves an occurrence whose content the caller may read (404 otherwise, so nothing leaks).
func (s *Service) occurrence(ctx context.Context, db *gorm.DB, a api.Actor, id uuid.UUID) (scope, error) {
	acc, oi, err := s.p.Authz.Occurrence(ctx, db, a, id)
	if err != nil {
		return scope{}, err
	}
	if !acc.ReadsContent() {
		return scope{}, kernel.NotFound("occurrence")
	}
	return scope{a: a, acc: acc, occ: oi}, nil
}

// writable additionally requires p and an occurrence that is not cancelled.
func (s *Service) writable(ctx context.Context, tx *gorm.DB, a api.Actor, id uuid.UUID, p api.Perm) (scope, error) {
	sc, err := s.occurrence(ctx, tx, a, id)
	if err != nil {
		return scope{}, err
	}
	if err := sc.acc.Require(p); err != nil {
		return scope{}, err
	}
	if sc.occ.Status == "cancelled" {
		return scope{}, kernel.Conflict("OCCURRENCE_CANCELLED", "the occurrence is cancelled")
	}
	return sc, nil
}

func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return kernel.InTx(ctx, s.db, fn)
}

func (s *Service) audit(ctx context.Context, tx *gorm.DB, sc scope, action string, meta any) error {
	return s.p.Audit.Log(ctx, tx, sc.a.TenantID.String(), sc.a.UserID.String(), action, "meeting_occurrence", sc.occ.ID.String(), meta, sc.a.IP, sc.a.UserAgent)
}

func (s *Service) names(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) map[uuid.UUID]string {
	if s.p.Names == nil || len(ids) == 0 {
		return map[uuid.UUID]string{}
	}
	out, err := s.p.Names.DisplayNames(ctx, tenantID, ids)
	if err != nil || out == nil {
		return map[uuid.UUID]string{}
	}
	return out
}

func emit(ctx context.Context, tx *gorm.DB, sc scope, key string, extra map[string]any) error {
	payload := map[string]any{"series_id": sc.occ.SeriesID, "occurrence_id": sc.occ.ID}
	for k, v := range extra {
		payload[k] = v
	}
	return kernel.Emit(ctx, tx, sc.a.TenantID, key, payload)
}
