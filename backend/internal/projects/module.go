// Package projects is Module 2, Projects & Tasks: its own service in the same repository, with its own schema
// (`projects`), migrations, routes and events (docs/modules/m2-projects-tasks/design.md). It reaches other services only
// through the interfaces in ports/ and internal/shared (identity, people, storage), never through their packages.
package projects

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/projects/aihooks"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/automation/bridge"
	"github.com/jaas/jaas/internal/projects/collab"
	"github.com/jaas/jaas/internal/projects/fields"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/planning"
	"github.com/jaas/jaas/internal/projects/ports"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/task"
	"github.com/jaas/jaas/internal/projects/webhooks"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/outbox"
	"github.com/jaas/jaas/internal/shared/queue"
	"github.com/jaas/jaas/internal/shared/storage"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

const (
	// Schema is the Postgres schema the module owns.
	Schema = "projects"
	// OutboxTable and InboxTable hold the module published events and consumed-event ledger.
	OutboxTable = "projects_events_outbox"
	InboxTable  = "projects_events_inbox"
	// Exchange is the topic exchange the module publishes to.
	Exchange = "jaas.projects.events"
)

// feedRatePerMinute caps requests per client IP on the public iCalendar feed.
const feedRatePerMinute = 30

// publicRatePerMinute caps requests per client IP on the public project page.
const publicRatePerMinute = 60

// incomingRatePerMinute caps provider calls per client IP on the public git-integration route.
const incomingRatePerMinute = 120

// groupRefreshEvery is how often group membership rules are re-resolved against HR data.
const groupRefreshEvery = 10 * time.Minute

// collabSweepEvery is how often stale timers are stopped and abandoned uploads removed.
const collabSweepEvery = 5 * time.Minute

// Options are the infrastructure handles M2 needs.
type Options struct {
	DB         *gorm.DB
	Publisher  queue.EventPublisher
	Logger     *zerolog.Logger
	Identity   identityport.Port
	Storage    storage.Store             // nil: attachments answer 503 until storage is configured
	People     ports.People              // nil: group rules answer 503 until organisation data is wired
	RateLimits middleware.RateLimitStore // nil: the public feed keeps its per-process limiter
	Background context.Context           // nil: no relays or sweeps (tests)
	SecretsKey string                    // base64 32-byte key for webhook and integration secrets; empty: those routes answer 503
}

// Module is the M2 service.
type Module struct {
	o        Options
	project  *project.Service
	task     *task.Service
	planning *planning.Service
	fields   *fields.Service
	aihooks  *aihooks.Service
	webhooks *webhooks.Service
	collab   *collab.Service
	suite    *bridge.Suite
}

// New builds the module and registers its permissions with M0.
func New(ctx context.Context, o Options) (*Module, error) {
	m := &Module{o: o}
	m.project = project.NewService(o.DB, project.Ports{Users: o.Identity.Users(), Audit: o.Identity.Audit(), People: o.People})
	m.task = task.NewService(o.DB, m.project, task.Ports{Audit: o.Identity.Audit()})
	m.planning = planning.NewService(o.DB, m.project, planning.Ports{Users: o.Identity.Users(), Audit: o.Identity.Audit()}) // wire:planning
	if o.RateLimits != nil {                                                                                                // the shared (Redis) limiter replaces the per-process default of the public feed
		m.planning.SetFeedLimiter(middleware.RateLimiter(o.RateLimits, feedRatePerMinute, time.Minute))
	}
	o.Identity.OnUserDeactivated(m.project) // specification section 7: deactivated logins leave projects, history stays
	// wire:collab
	m.collab = collab.NewService(o.DB, collab.Ports{Authorizer: m.project, Tasks: m.task, Storage: o.Storage, Users: o.Identity.Users(), Audit: o.Identity.Audit()})
	m.task.SetHooks(m.collab.TaskHooks()) // CM-02/CM-03: description mentions and automatic watchers
	if o.Background != nil && o.Logger != nil {
		go m.collab.Run(o.Background, collabSweepEvery, func(err error) { o.Logger.Error().Err(err).Msg("collaboration sweep failed") })
	}
	m.fields = fields.NewService(o.DB, m.project, m.task, fields.Ports{Audit: o.Identity.Audit()}) // wire:fields
	m.aihooks = aihooks.NewService(o.DB, m.project, m.task)                                        // wire:aihooks
	if err := m.wireWebhooks(); err != nil {                                                       // wire:webhooks
		return nil, err
	}
	m.suite = bridge.NewSuite(bridge.Deps{DB: o.DB, Authz: m.project, Tasks: m.task, Collab: m.collab, Audit: o.Identity.Audit()}) // wire:automation - rules, recurring tasks, public links
	if o.RateLimits != nil {
		m.suite.SetPublicLimiter(middleware.RateLimiter(o.RateLimits, publicRatePerMinute, time.Minute))
	}
	// wire:platform
	if err := o.Identity.RegisterPermissions(ctx, Catalogue()); err != nil {
		return nil, fmt.Errorf("projects permission catalogue: %w", err)
	}
	m.startBackground()
	return m, nil
}

// Catalogue is M2 tenant-level permission catalogue (specification section 8); project-level permissions live in M2.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "projects", Resource: "projects", Action: "create", Description: "Create projects",
			Defaults: map[string]access.Scope{access.TemplateEmployee: access.ScopeTenant}},
		{Module: "projects", Resource: "projects", Action: "read_all", Description: "See every project, including private ones",
			Defaults: map[string]access.Scope{access.TemplateExecutive: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant, access.TemplateITAdmin: access.ScopeTenant}},
		{Module: "projects", Resource: "projects", Action: "manage_all", Description: "Administer any project: members, settings, archive and delete",
			Defaults: map[string]access.Scope{access.TemplateITAdmin: access.ScopeTenant}},
		{Module: "projects", Resource: "project_roles", Action: "manage", Description: "Create and edit custom project roles",
			Defaults: map[string]access.Scope{access.TemplateITAdmin: access.ScopeTenant}},
	}
}

func (m *Module) startBackground() {
	if m.o.Background != nil && m.o.Logger != nil { // rule engine and recurrence scheduler read the database, not the broker
		m.suite.Run(m.o.Background, func(msg string, err error) { m.o.Logger.Error().Err(err).Msg(msg) })
	}
	if m.o.Background == nil || m.o.Publisher == nil {
		return
	}
	relay := outbox.NewTableRelay(m.o.DB, kernel.OutboxTable, Exchange, m.o.Publisher)
	go relay.Run(m.o.Background, func(err error) { m.o.Logger.Error().Err(err).Str("outbox", OutboxTable).Msg("outbox relay failed") })
	go m.project.RunGroupRefresh(m.o.Background, groupRefreshEvery, func(err error) { m.o.Logger.Error().Err(err).Msg("group rule refresh failed") })
}

// Authorizer exposes project access resolution for tests and other contexts.
func (m *Module) Authorizer() api.Authorizer { return m.project }

// RegisterRoutes mounts the M2 routes under rg (/api/v1); every route needs a verified tenant user.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	idp := m.o.Identity
	g := rg.Group("", idp.TenantResolver(), idp.Authenticate(),
		idp.Flag("projects", "create", api.FlagCreate), idp.Flag("projects", "read_all", api.FlagReadAll),
		idp.Flag("projects", "manage_all", api.FlagManageAll), idp.Flag("project_roles", "manage", api.FlagRoles), agentLabel())
	m.project.Register(g)
	m.task.Register(g)
	m.planning.Register(g) // routes:planning
	m.fields.Register(g)   // routes:fields
	m.aihooks.Register(g)  // routes:aihooks
	m.webhooks.Register(g) // routes:webhooks
	m.suite.Register(g)    // routes:automation
	// routes:collab
	m.collab.Register(g)
	// routes:platform
	// routes:planning-public
	m.suite.RegisterPublic(rg)    // routes:publiclink-public - read-only project page authenticated by a secret token + per-IP limit
	m.planning.RegisterPublic(rg) // the iCalendar feed is the one M2 route without a bearer token (token + per-IP limit)
	m.webhooks.RegisterPublic(rg) // routes:webhooks-public - provider calls, authenticated by the HMAC of the raw body + per-IP limit
}

// wireWebhooks builds the webhooks context (outgoing webhooks, chat messages, git integrations) and starts its dispatcher
// and sender when background work is enabled.
func (m *Module) wireWebhooks() error {
	o := m.o
	svc, err := webhooks.NewService(o.DB, m.project, m.task, webhooks.Ports{Audit: o.Identity.Audit(), SecretsKey: o.SecretsKey})
	if err != nil {
		return fmt.Errorf("projects webhooks: %w", err)
	}
	m.webhooks = svc
	if o.RateLimits != nil {
		svc.SetLimiter(middleware.RateLimiter(o.RateLimits, incomingRatePerMinute, time.Minute))
	}
	if o.Background != nil && o.Logger != nil {
		go svc.Run(o.Background, func(err error) { o.Logger.Error().Err(err).Msg("webhook dispatch failed") })
	}
	return nil
}
