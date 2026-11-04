// Package series is the M3 core: meeting series, materialised occurrences, participants and their M0 roles, responses,
// consent and the Authorizer every other M3 context uses (docs/modules/m3-meetings/design.md section 1).
package series

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Window and size limits (DMT-03, MR-13).
const (
	WindowMonths      = 12
	MaxParticipants   = 500
	MaxListDays       = 62
	maxInTxOccurrence = 2000
	tenantOccurrences = 200000
)

// Names resolves display names; nil means none are shown.
type Names interface {
	DisplayNames(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Ports are the services the series context depends on.
type Ports struct {
	Users     identityport.Users
	Audit     identityport.Auditor
	Resources identityport.ResourceAccess
	Names     Names
}

// Service implements the series context and api.Authorizer.
type Service struct {
	db  *gorm.DB
	p   Ports
	now func() time.Time
}

// NewService builds the series service; the M0 resource-role port is required (DMT-09).
func NewService(db *gorm.DB, p Ports) *Service {
	if p.Resources == nil || p.Users == nil || p.Audit == nil {
		panic("meetings: series.NewService needs the M0 users, audit and resource-role ports")
	}
	return &Service{db: db, p: p, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Now returns the service clock.
func (s *Service) Now() time.Time { return s.now() }

// DB returns the database handle the service uses.
func (s *Service) DB() *gorm.DB { return s.db }

func ref(tenantID, seriesID uuid.UUID) identityport.ResourceRef {
	return identityport.ResourceRef{TenantID: tenantID, Type: access.AppliesMeetingSeries, ID: seriesID}
}

func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return kernel.InTx(ctx, s.db, fn)
}

// audit records an M0 audit entry; failures surface so a write and its audit commit together.
func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, action string, id uuid.UUID, meta any) error {
	return s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, "meeting_series", id.String(), meta, a.IP, a.UserAgent)
}

// change appends a change-log row (SC-6).
type change struct {
	series     uuid.UUID
	occurrence *uuid.UUID
	action     string
	fields     map[string]any
	reason     string
}

func (s *Service) logChange(ctx context.Context, tx *gorm.DB, a api.Actor, c change) error {
	raw, err := json.Marshal(c.fields)
	if err != nil || c.fields == nil {
		raw = []byte("{}")
	}
	var actor *uuid.UUID
	if a.UserID != uuid.Nil {
		actor = &a.UserID
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO meeting_change_log (id, tenant_id, series_id, occurrence_id, actor_id, action, changes, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New(), a.TenantID, c.series, c.occurrence, actor, c.action, string(raw), c.reason).Error
}

// names returns display names for ids, best effort.
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
