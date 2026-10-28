package task

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Ports are the platform services the task context needs.
type Ports struct {
	Audit identityport.Auditor
}

// Service holds the task use cases. It implements api.TaskResolver.
type Service struct {
	db    *gorm.DB
	auth  api.Authorizer
	p     Ports
	now   func() time.Time
	hooks api.TaskHooks // nil until the collaboration context binds itself
}

// NewService builds the service; auth resolves what the caller may do in a project.
func NewService(db *gorm.DB, auth api.Authorizer, p Ports) *Service {
	return &Service{db: db, auth: auth, p: p, now: time.Now}
}

// Limits (specification section 2).
const (
	maxTitle       = 300
	maxDescription = 50000
	maxEstimate    = 10000000
	maxDepth       = 3
	maxBulk        = 100
	maxAssignees   = 20
	maxLabels      = 20
	maxGraphWalk   = 10000
	maxRankLen     = 24
	rebalanceLimit = 5000
)

var priorities = map[string]bool{"none": true, "low": true, "medium": true, "high": true, "urgent": true}

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

func isCheck(err error) bool { return errors.Is(err, gorm.ErrCheckConstraintViolated) }

func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

func runes(v string) int { return utf8.RuneCountInString(v) }

func cleanTitle(v string) (string, error) {
	v = strings.TrimSpace(v)
	if n := runes(v); n < 1 || n > maxTitle {
		return "", kernel.Invalid("title", "length must be between 1 and 300")
	}
	return v, nil
}

func checkDescription(v string) error {
	if runes(v) > maxDescription {
		return kernel.Invalid("description", "is too long")
	}
	return nil
}

func checkPriority(v string) error {
	if !priorities[v] {
		return kernel.Invalid("priority", "must be none, low, medium, high or urgent")
	}
	return nil
}

func checkEstimate(v *int) error {
	if v != nil && (*v < 0 || *v > maxEstimate) {
		return kernel.Invalid("estimate_minutes", "must be between 0 and 10000000")
	}
	return nil
}

// checkDates enforces start_on <= due_at (a day comparison in UTC).
func checkDates(start *kernel.Date, due *time.Time) error {
	if start == nil || due == nil {
		return nil
	}
	y, m, d := due.UTC().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if day.Before(start.Time) {
		return kernel.Invalid("due_at", "must not be before start_on")
	}
	return nil
}

func uniqueIDs(ids []uuid.UUID, max int, field string) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]bool, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, kernel.Invalid(field, "contains an empty id")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > max {
		return nil, kernel.Invalid(field, "has too many entries")
	}
	return out, nil
}

// audit records a successful mutation; metadata holds ids only. Failures never fail the request.
func (s *Service) audit(u unit, action string, id uuid.UUID, meta map[string]any) {
	ctx, tx, a := u.ctx, u.tx, u.a
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, "task", id.String(), meta, a.IP, a.UserAgent)
}

// access resolves the caller access to a project (404 when the caller cannot read it).
func (s *Service) access(ctx context.Context, a api.Actor, projectID uuid.UUID) (api.Access, error) {
	return s.auth.Access(ctx, s.db, a, projectID)
}
