package fields

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Ports are the services the fields context needs from the platform.
type Ports struct {
	Audit identityport.Auditor
}

// Service holds the custom-field use cases.
type Service struct {
	db    *gorm.DB
	authz api.Authorizer
	tasks api.TaskResolver
	p     Ports
	now   func() time.Time
}

// NewService builds the service; authz resolves project access and tasks resolves a task with its project access.
func NewService(db *gorm.DB, authz api.Authorizer, tasks api.TaskResolver, p Ports) *Service {
	return &Service{db: db, authz: authz, tasks: tasks, p: p, now: time.Now}
}

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

// inTx runs fn in a short transaction.
func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// ref names the audited resource and carries its metadata (ids and codes only).
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

func countRows(db *gorm.DB, table, where string, args ...any) (int64, error) {
	var n int64
	err := db.Table(table).Where(where, args...).Count(&n).Error
	return n, err
}
