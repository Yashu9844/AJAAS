package bridge

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/automation"
	"github.com/jaas/jaas/internal/projects/collab"
	"github.com/jaas/jaas/internal/projects/publiclink"
	"github.com/jaas/jaas/internal/projects/recurrence"
	"github.com/jaas/jaas/internal/projects/task"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Loop intervals of the background workers.
const (
	EngineEvery     = 5 * time.Second
	RecurrenceEvery = 60 * time.Second
)

// Suite bundles the automation rules, recurring tasks and public links so the module wires them in one place.
type Suite struct {
	Rules      *automation.Service
	Engine     *automation.Engine
	Recurrence *recurrence.Service
	Links      *publiclink.Service
}

// Deps are what the suite is built on.
type Deps struct {
	DB     *gorm.DB
	Authz  api.Authorizer
	Tasks  *task.Service
	Collab *collab.Service
	Audit  identityport.Auditor
}

// NewSuite builds the three services on the task and collaboration services.
func NewSuite(d Deps) *Suite {
	tasks := New(d.Tasks, d.Collab)
	db, authz, audit := d.DB, d.Authz, d.Audit
	rules := automation.NewService(db, authz, tasks, automation.Ports{Audit: audit})
	return &Suite{
		Rules:      rules,
		Engine:     automation.NewEngine(rules),
		Recurrence: recurrence.NewService(db, authz, tasks, recurrence.Ports{Audit: audit}),
		Links:      publiclink.NewService(db, authz, publiclink.Ports{Audit: audit}),
	}
}

// SetPublicLimiter replaces the in-process limiter of the public project route (the shared Redis limiter).
func (s *Suite) SetPublicLimiter(h gin.HandlerFunc) { s.Links.SetLimiter(h) }

// Run starts the rule engine and the recurrence scheduler and returns; both stop when ctx ends.
func (s *Suite) Run(ctx context.Context, onErr func(string, error)) {
	go s.Engine.Run(ctx, EngineEvery, func(err error) { onErr("automation engine failed", err) })
	go s.Recurrence.Run(ctx, RecurrenceEvery, func(err error) { onErr("recurrence scheduler failed", err) })
}

// Register mounts the authenticated routes of the three contexts.
func (s *Suite) Register(g *gin.RouterGroup) {
	s.Rules.Register(g)
	s.Recurrence.Register(g)
	s.Links.Register(g)
}

// RegisterPublic mounts the anonymous public project route.
func (s *Suite) RegisterPublic(rg *gin.RouterGroup) { s.Links.RegisterPublic(rg) }
