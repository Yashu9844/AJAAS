// Package planning is the planning and views context of M2: sprints, the backlog and bulk planning moves, and the read
// models (board, calendar, Gantt, My work, search, saved views, iCalendar feed). Specification sections 6 (SP, VW) and
// TK-14.
//
// Boundary decision: this context does not import the task context. The read models query the tasks, board_columns,
// task_assignees and task_labels tables of the shared projects schema with SQL (always scoped by tenant_id), and
// authorisation goes through api.Authorizer. The single write it makes to a task row is tasks.sprint_id (plus
// version+1 and updated_at): the task context owns every other task column and its own rules (WIP, ranks, completion).
// A planning move changes one column only, so it can never clobber a concurrent edit of another field, and the
// per-task version predicate still makes it safe against concurrent edits (PG2 style).
package planning

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Ports are the platform services the planning context needs. Both are optional: without Users the iCalendar feed does
// not re-check that the token owner is still active; without Audit nothing is audited.
type Ports struct {
	Users identityport.Users
	Audit identityport.Auditor
}

// Service holds the planning use cases.
type Service struct {
	db      *gorm.DB
	authz   api.Authorizer
	p       Ports
	now     func() time.Time
	limiter gin.HandlerFunc
}

// NewService builds the service; authz is the project context.
func NewService(db *gorm.DB, authz api.Authorizer, p Ports) *Service {
	return &Service{db: db, authz: authz, p: p, now: time.Now, limiter: newFeedLimiter(feedRatePerMinute, time.Minute)}
}

// SetFeedLimiter replaces the default in-process limiter of the public iCalendar feed (for example with the shared
// Redis-backed middleware.RateLimiter).
func (s *Service) SetFeedLimiter(h gin.HandlerFunc) { s.limiter = h }

// Limits (specification section 6).
const (
	maxName        = 120
	maxGoal        = 1000
	maxViewName    = 80
	maxPlanTasks   = 100
	maxWindowDays  = 62
	maxGanttWindow = 731
	maxPageTasks   = 500
	maxSearchQuery = 200
	maxSearchLimit = 50
)

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

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

// inTx runs fn in a short transaction.
func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// access resolves the caller access to a project; the caller cannot tell a private project from a missing one (PG4).
func (s *Service) access(ctx context.Context, a api.Actor, projectID uuid.UUID) (api.Access, error) {
	return s.authz.Access(ctx, s.db, a, projectID)
}

// requireScrum answers 409 PROJECT_MODE for kanban projects: sprints exist only in scrum mode (SP-01).
func requireScrum(acc api.Access) error {
	if acc.Project.Mode != api.ModeScrum {
		return kernel.Conflict("PROJECT_MODE", "sprints are available in scrum projects only")
	}
	return nil
}

// ref names the audited resource.
type ref struct {
	resource string
	id       uuid.UUID
}

// audit records a successful mutation; metadata holds ids and codes only. Failures never fail the request (SECURITY 7).
func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, action string, r ref) {
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, r.resource, r.id.String(), nil, a.IP, a.UserAgent)
}

// idArray renders ids as one Postgres uuid[] literal so a long list stays a single bind parameter (uuid.String only
// yields hex digits and dashes, so the literal cannot carry SQL).
func idArray(ids []uuid.UUID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id.String()
	}
	return "{" + strings.Join(parts, ",") + "}"
}
