package automation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Tasks is what rules need from the task context. Every method acts as the given actor and goes through the same use
// cases a person would, so permissions, WIP limits, done rules and versions apply unchanged. The composition root
// binds the real task and collaboration services to it (see the bridge package); tests use a fake.
type Tasks interface {
	// State reads the task with the actor's access (404 when the actor cannot read it).
	State(ctx context.Context, a api.Actor, id uuid.UUID) (TaskState, error)
	Move(ctx context.Context, a api.Actor, id, columnID uuid.UUID) error
	SetPriority(ctx context.Context, a api.Actor, id uuid.UUID, priority string) error
	AddLabel(ctx context.Context, a api.Actor, id, labelID uuid.UUID) error
	Assign(ctx context.Context, a api.Actor, id, userID uuid.UUID) error
	SetDue(ctx context.Context, a api.Actor, id uuid.UUID, due time.Time) error
	Comment(ctx context.Context, a api.Actor, id uuid.UUID, text string) error
}

// Ports are the services the automation context needs from the platform.
type Ports struct {
	Audit identityport.Auditor
}

// Service holds the rule use cases; Engine (engine.go) runs them.
type Service struct {
	db    *gorm.DB
	authz api.Authorizer
	tasks Tasks
	p     Ports
	now   func() time.Time
}

// NewService builds the service; authz resolves project access and tasks acts on tasks.
func NewService(db *gorm.DB, authz api.Authorizer, tasks Tasks, p Ports) *Service {
	return &Service{db: db, authz: authz, tasks: tasks, p: p, now: time.Now}
}

func isDup(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

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

func (s *Service) require(ctx context.Context, a api.Actor, projectID uuid.UUID, p api.Perm) error {
	acc, err := s.authz.Access(ctx, nil, a, projectID)
	if err != nil {
		return err
	}
	return acc.Require(p)
}

func countRows(db *gorm.DB, table, where string, args ...any) (int64, error) {
	var n int64
	err := db.Table(table).Where(where, args...).Count(&n).Error
	return n, err
}

// agentOf is the context label every change made by a rule carries; the engine never reacts to events with it.
func agentOf(ruleID uuid.UUID) string { return AgentPrefix + "." + ruleID.String()[:8] }

// AgentPrefix starts the agent label of every change made by a rule.
const AgentPrefix = "automation"
