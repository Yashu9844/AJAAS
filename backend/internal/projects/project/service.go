package project

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/ports"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Ports are the services the project context needs from the rest of the platform.
type Ports struct {
	Users  identityport.Users
	Audit  identityport.Auditor
	People ports.People // nil: group rules answer 503 PEOPLE_UNAVAILABLE
	Names  ports.Names  // nil: members carry no display names
}

// Service holds the project use cases. It also implements api.Authorizer.
type Service struct {
	db  *gorm.DB
	p   Ports
	now func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB, p Ports) *Service { return &Service{db: db, p: p, now: time.Now} }

var (
	keyPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
	colorPattern = regexp.MustCompile(`^(#[0-9A-Fa-f]{6})?$`)
)

// Limits (specification section 1).
const (
	maxName        = 120
	maxDescription = 10000
	maxIcon        = 40
)

func (s *Service) conn(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return s.db
}

func tooLong(v string, max int) bool { return utf8.RuneCountInString(v) > max }

func text(field, v string, min, max int) (string, error) {
	v = strings.TrimSpace(v)
	if n := utf8.RuneCountInString(v); n < min || n > max {
		return "", kernel.Invalid(field, "length must be between "+itoa(min)+" and "+itoa(max))
	}
	return v, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// countRows counts rows of table matching where, reporting database errors instead of treating them as zero.
func countRows(db *gorm.DB, table, where string, args ...any) (int64, error) {
	var n int64
	err := db.Table(table).Where(where, args...).Count(&n).Error
	return n, err
}

// dropWatches removes a user watches on the tasks of one project (all projects when projectID is the zero uuid): losing
// access to a project ends the right to follow its tasks (CM-03).
func dropWatches(tx *gorm.DB, tenantID, projectID, userID uuid.UUID) error {
	if projectID == uuid.Nil {
		return tx.Exec(`DELETE FROM task_watchers WHERE tenant_id = ? AND user_id = ?`, tenantID, userID).Error
	}
	return tx.Exec(`DELETE FROM task_watchers WHERE tenant_id = ? AND user_id = ? AND task_id IN (SELECT id FROM tasks WHERE project_id = ?)`,
		tenantID, userID, projectID).Error
}

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

// inTx runs fn in a transaction.
func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// ref names the audited resource and carries the metadata (ids and codes only).
type ref struct {
	resource string
	id       uuid.UUID
	meta     map[string]any
}

// audit records a successful mutation; metadata holds ids and codes only. Failures never fail the request (SECURITY 7).
func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, action string, r ref) {
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, r.resource, r.id.String(), r.meta, a.IP, a.UserAgent)
}
