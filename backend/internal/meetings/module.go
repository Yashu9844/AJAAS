// Package meetings is Module 3, Meetings: its own service in the same repository, with its own schema (`meetings`),
// migrations, routes and events (docs/modules/m3-meetings/design.md). It reaches other services only through
// internal/shared ports (identity, tasks), never through their packages.
package meetings

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/meetings/actions"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/content"
	"github.com/jaas/jaas/internal/meetings/feed"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/meetings/series"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/outbox"
	"github.com/jaas/jaas/internal/shared/queue"
	"github.com/jaas/jaas/internal/shared/tasksport"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

const (
	// Schema is the Postgres schema the module owns.
	Schema = "meetings"
	// Exchange is the topic exchange the module publishes to.
	Exchange = "jaas.meetings.events"
	// feedRatePerMinute caps requests per client IP on the public iCalendar feed.
	feedRatePerMinute = 30
)

// Options are the infrastructure handles M3 needs.
type Options struct {
	DB         *gorm.DB
	Publisher  queue.EventPublisher
	Logger     *zerolog.Logger
	Identity   identityport.Port
	Tasks      tasksport.Tasks           // nil: action items answer 503 until M2 is wired
	Names      series.Names              // nil: no display names
	RateLimits middleware.RateLimitStore // nil: the public feed uses a per-process limiter
	Background context.Context           // nil: no relays or jobs (tests)
}

// Module is the M3 service.
type Module struct {
	o       Options
	series  *series.Service
	content *content.Service
	actions *actions.Service
	feed    *feed.Service
}

// New builds the module and registers its permissions with M0.
func New(ctx context.Context, o Options) (*Module, error) {
	m := &Module{o: o}
	m.series = series.NewService(o.DB, series.Ports{Users: o.Identity.Users(), Audit: o.Identity.Audit(), Resources: o.Identity.Resources(), Names: o.Names})
	m.content = content.NewService(o.DB, content.Ports{Authz: m.series, Lifecycle: m.series, Audit: o.Identity.Audit(), Names: o.Names})
	m.actions = actions.NewService(o.DB, m.series, o.Tasks)
	m.feed = feed.NewService(o.DB, m.series, o.Identity.Users(), o.Identity.Audit())
	o.Identity.OnUserDeactivated(m.series) // DMT-15: deactivated logins leave meetings, history stays
	if err := o.Identity.RegisterPermissions(ctx, Catalogue()); err != nil {
		return nil, fmt.Errorf("meetings permission catalogue: %w", err)
	}
	m.startBackground()
	return m, nil
}

func (m *Module) startBackground() {
	if m.o.Background == nil || m.o.Logger == nil {
		return
	}
	log := m.o.Logger
	go m.series.Run(m.o.Background, func(msg string, err error) { log.Error().Err(err).Msg(msg) })
	if m.o.Publisher != nil {
		relay := outbox.NewTableRelay(m.o.DB, kernel.OutboxTable, Exchange, m.o.Publisher)
		go relay.Run(m.o.Background, func(err error) { log.Error().Err(err).Str("outbox", kernel.OutboxTable).Msg("outbox relay failed") })
	}
}

// Actions exposes the action-item service (the M2 task-event subscription).
func (m *Module) Actions() *actions.Service { return m.actions }

// RegisterRoutes mounts the M3 routes under rg (/api/v1); every route but the public feed needs a verified tenant user.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	idp := m.o.Identity
	g := rg.Group("", idp.TenantResolver(), idp.Authenticate(),
		idp.Flag("meetings", "create", api.FlagCreate), idp.Flag("meetings", "read_all", api.FlagReadAll),
		idp.Flag("meetings", "read_confidential", api.FlagReadConfidential), idp.Flag("meetings", "manage_all", api.FlagManageAll))
	m.series.Register(g)
	m.content.Register(g)
	m.actions.Register(g)
	m.feed.Register(g)
	var store middleware.RateLimitStore = middleware.NewMemoryRateStore()
	if m.o.RateLimits != nil {
		store = m.o.RateLimits
	}
	m.feed.RegisterPublic(rg, middleware.RateLimiter(store, feedRatePerMinute, time.Minute))
}

// meetingTemplates are the M0 meeting-series role templates, least privileged first (DMT-09).
var meetingTemplates = []string{access.TemplateMeetingViewer, access.TemplateMeetingAttendee, access.TemplateMeetingNoteTaker,
	access.TemplateMeetingOrganizer}

// Catalogue is the M3 permission catalogue registered in M0: the tenant permissions (DMT-10) and the meeting-series
// permissions that meeting roles are built from (DMT-09).
func Catalogue() []access.PermissionDef {
	defs := make([]access.PermissionDef, 0, len(api.AllPerms)+5)
	for _, p := range api.AllPerms {
		resource, action, _ := strings.Cut(string(p), ":")
		d := access.PermissionDef{Module: "meetings", Resource: resource, Action: action, AppliesTo: access.AppliesMeetingSeries,
			Description: "Meeting permission " + string(p), Defaults: map[string]access.Scope{}}
		for _, t := range meetingTemplates {
			if api.TemplatePerms(t)[p] {
				d.Defaults[t] = access.ScopeTenant
			}
		}
		defs = append(defs, d)
	}
	return append(defs, tenantPermissionDefs()...)
}

func tenantPermissionDefs() []access.PermissionDef {
	all := map[string]access.Scope{access.TemplateExecutive: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}
	return []access.PermissionDef{
		{Module: "meetings", Resource: "meetings", Action: "create", Description: "Schedule meetings",
			Defaults: map[string]access.Scope{access.TemplateEmployee: access.ScopeTenant}},
		{Module: "meetings", Resource: "meetings", Action: "read_all", Description: "Read every meeting and its published minutes (audited)",
			Sensitive: true, Defaults: all},
		{Module: "meetings", Resource: "meetings", Action: "read_confidential", Description: "Also read confidential meetings (audited)",
			Sensitive: true, Defaults: map[string]access.Scope{access.TemplateAuditor: access.ScopeTenant}},
		{Module: "meetings", Resource: "meetings", Action: "manage_all", Description: "Administer any meeting",
			Sensitive: true, Defaults: map[string]access.Scope{access.TemplateITAdmin: access.ScopeTenant}},
		{Module: "meetings", Resource: "meetings", Action: "attendance_report", Description: "See attendance reports",
			Sensitive: true, Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
	}
}
