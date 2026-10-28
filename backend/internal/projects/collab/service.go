// Package collab is the collaboration context of M2: comments with mentions, watchers, time tracking and file
// attachments (specification sections 3, 4 and 5). It acts on tasks only through api.TaskResolver and on project
// access only through api.Authorizer, so it never imports the task or project contexts.
package collab

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/storage"
	"gorm.io/gorm"
)

// Scanner is the malware-scan hook that runs before an upload becomes ready (AT-02).
type Scanner interface {
	// Scan inspects the stored object; clean is false when it must be rejected.
	Scan(ctx context.Context, key string, info storage.ObjectInfo) (clean bool, err error)
}

// NoopScanner accepts every file until a real scanner exists.
type NoopScanner struct{}

// Scan always reports the object as clean.
func (NoopScanner) Scan(context.Context, string, storage.ObjectInfo) (bool, error) { return true, nil }

// Ports are the services the collaboration context needs from the rest of the platform.
type Ports struct {
	Authorizer api.Authorizer
	Tasks      api.TaskResolver
	Storage    storage.Store // nil: every attachment endpoint answers 503 STORAGE_UNAVAILABLE
	Users      identityport.Users
	Audit      identityport.Auditor
	Scanner    Scanner // nil: NoopScanner
}

// Service holds the collaboration use cases.
type Service struct {
	db  *gorm.DB
	p   Ports
	now func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB, p Ports) *Service {
	if p.Scanner == nil {
		p.Scanner = NoopScanner{}
	}
	return &Service{db: db, p: p, now: time.Now}
}

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

// inTx runs fn in a short transaction.
func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// auditEntry is one audit record; metadata holds ids and codes only (SECURITY 7, PG12).
type auditEntry struct {
	Action, Resource string
	ID               uuid.UUID
	Meta             map[string]any
}

// audit records a successful mutation; a failure never fails the request.
func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, e auditEntry) {
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), e.Action, e.Resource, e.ID.String(), e.Meta, a.IP, a.UserAgent)
}

// taskAccess resolves the task through its project and requires perm; unknown, foreign and unreadable tasks are 404 (PG4).
func (s *Service) taskAccess(ctx context.Context, a api.Actor, taskID uuid.UUID, perm api.Perm) (api.TaskRef, api.Access, error) {
	ref, acc, err := s.p.Tasks.Task(ctx, s.db, a, taskID)
	if err != nil {
		return api.TaskRef{}, api.Access{}, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return api.TaskRef{}, api.Access{}, err
	}
	if perm != api.TaskRead {
		if err := acc.Require(perm); err != nil {
			return api.TaskRef{}, api.Access{}, err
		}
	}
	return ref, acc, nil
}

// PageRequest is a cursor and a clamped page size.
type PageRequest struct {
	Cursor string
	Limit  int
}

func (p PageRequest) limit() int {
	if p.Limit < 1 || p.Limit > kernel.MaxLimit {
		return kernel.DefaultLimit
	}
	return p.Limit
}

// page trims the probe row and encodes the cursor of the last returned row.
func page[T any](rows []T, limit int, cursor func(T) any) ([]T, string) {
	rows, more := kernel.Trim(rows, limit)
	if !more || len(rows) == 0 {
		return rows, ""
	}
	return rows, kernel.EncodeCursor(cursor(rows[len(rows)-1]))
}
