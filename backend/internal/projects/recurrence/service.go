package recurrence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// AgentPrefix starts the agent label of every task a recurrence creates.
const AgentPrefix = "recurrence"

// Ports are the services the recurrence context needs from the platform.
type Ports struct {
	Audit identityport.Auditor
}

// Service holds the recurrence use cases; Scheduler (scheduler.go) runs them on time.
type Service struct {
	db    *gorm.DB
	authz api.Authorizer
	tasks Tasks
	p     Ports
	now   func() time.Time
}

// NewService builds the service; authz resolves project access and tasks copies template tasks.
func NewService(db *gorm.DB, authz api.Authorizer, tasks Tasks, p Ports) *Service {
	return &Service{db: db, authz: authz, tasks: tasks, p: p, now: time.Now}
}

func (s *Service) inTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

type ref struct {
	id   uuid.UUID
	meta map[string]any
}

// audit records a successful mutation; failures never fail the request (SECURITY 7).
func (s *Service) audit(ctx context.Context, tx *gorm.DB, a api.Actor, action string, r ref) {
	if s.p.Audit == nil {
		return
	}
	_ = s.p.Audit.Log(ctx, tx, a.TenantID.String(), a.UserID.String(), action, "task_recurrence", r.id.String(), r.meta, a.IP, a.UserAgent)
}

func (s *Service) access(ctx context.Context, a api.Actor, projectID uuid.UUID) (api.Access, error) {
	return s.authz.Access(ctx, nil, a, projectID)
}

func (s *Service) require(ctx context.Context, a api.Actor, projectID uuid.UUID, p api.Perm) error {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return err
	}
	return acc.Require(p)
}

// agentOf is the label of tasks created by a recurrence.
func agentOf(id uuid.UUID) string { return AgentPrefix + "." + id.String()[:8] }
