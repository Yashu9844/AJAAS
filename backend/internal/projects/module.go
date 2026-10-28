// Package projects is Module 2, Projects & Tasks: its own service in the same repository, with its own schema
// (`projects`), migrations, routes and events (docs/modules/m2-projects-tasks/design.md). It reaches other services only
// through the interfaces in ports/ and internal/shared (identity, people, storage), never through their packages.
package projects

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/shared/identityport"
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

// Options are the infrastructure handles M2 needs.
type Options struct {
	DB         *gorm.DB
	Publisher  queue.EventPublisher
	Logger     *zerolog.Logger
	Identity   identityport.Port
	Storage    storage.Store   // nil: attachments answer 503 until storage is configured
	Background context.Context // nil: no relays or sweeps (tests)
}

// Module is the M2 service.
type Module struct {
	o       Options
	project *project.Service
}

// New builds the module and registers its permissions with M0.
func New(ctx context.Context, o Options) (*Module, error) {
	m := &Module{o: o}
	m.project = project.NewService(o.DB, project.Ports{Users: o.Identity.Users(), Audit: o.Identity.Audit()})
	// wire:tasks
	// wire:planning
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
	if m.o.Background == nil || m.o.Publisher == nil {
		return
	}
	relay := outbox.NewTableRelay(m.o.DB, kernel.OutboxTable, Exchange, m.o.Publisher)
	go relay.Run(m.o.Background, func(err error) { m.o.Logger.Error().Err(err).Str("outbox", OutboxTable).Msg("outbox relay failed") })
}

// Authorizer exposes project access resolution for tests and other contexts.
func (m *Module) Authorizer() api.Authorizer { return m.project }

// RegisterRoutes mounts the M2 routes under rg (/api/v1); every route needs a verified tenant user.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	idp := m.o.Identity
	g := rg.Group("", idp.TenantResolver(), idp.Authenticate(),
		idp.Flag("projects", "create", api.FlagCreate), idp.Flag("projects", "read_all", api.FlagReadAll),
		idp.Flag("projects", "manage_all", api.FlagManageAll), idp.Flag("project_roles", "manage", api.FlagRoles))
	m.project.Register(g)
	// routes:tasks
	// routes:planning
	// routes:platform
}
