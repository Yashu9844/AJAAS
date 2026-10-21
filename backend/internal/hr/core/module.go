// Package core wires HR/Core (job architecture, positions, assignments, HR actions) into M1
// (docs/modules/m1-hr-management/contexts/core).
package core

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/hr/core/controllers"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"github.com/jaas/jaas/internal/hr/core/routes"
	"github.com/jaas/jaas/internal/hr/core/services"
	employeeAPI "github.com/jaas/jaas/internal/hr/employee/api"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// sweepInterval is how often due effects are applied (CO-041).
const sweepInterval = time.Hour

// Ports are the collaborators HR/Core consumes (design.md C2–C4).
type Ports struct {
	Employees services.EmployeeDirectory
	Lifecycle employeeAPI.Lifecycle
	Org       services.OrgDirectory
	Logins    services.Logins
	Audit     services.AuditLogger
	Publisher queue.EventPublisher
}

// Module is the HR/Core DI container.
type Module struct {
	controller *controllers.Controller
	relay      *services.OutboxRelay
	effects    *services.EffectsService
	dir        directory
	hooks      *services.Hooks
}

// NewModule wires repositories, services and the controller.
func NewModule(db *gorm.DB, p Ports) *Module {
	d := services.Deps{Tx: services.NewTxRunner(db), Repo: repositories.New(), Employees: p.Employees, Lifecycle: p.Lifecycle, Org: p.Org,
		Logins: p.Logins, Audit: p.Audit, Publisher: p.Publisher, Now: time.Now, Hooks: &services.Hooks{}}
	effects := services.NewEffectsService(d)
	return &Module{
		controller: controllers.New(controllers.Services{Architecture: services.NewArchitectureService(d), Positions: services.NewPositionService(d),
			Actions: services.NewActionService(d), Workers: services.NewWorkerService(d), Effects: effects}),
		relay:   services.NewOutboxRelay(d),
		effects: effects,
		dir:     directory{db: db, repo: d.Repo},
		hooks:   d.Hooks,
	}
}

// RegisterModels lists HR/Core models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&models.JobFamily{}, &models.Grade{}, &models.JobProfile{}, &models.Position{}, &models.Action{}, &models.Assignment{},
		&models.HeadcountPlan{}, &models.OutboxEvent{}}
}

// RouteDeps is the M0 infrastructure the routes need.
type RouteDeps struct {
	Identity identityport.HTTP
}

// RegisterRoutes mounts HR/Core under rg.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{TenantResolver: d.Identity.TenantResolver(), Authenticate: d.Identity.Authenticate(),
		Require: func(action string) gin.HandlerFunc { return d.Identity.Require("core", action) }}, m.controller)
}

// Relay exposes the outbox relay.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

// RunEffects runs the hourly effective-date sweep until ctx is cancelled (CO-041).
func (m *Module) RunEffects(ctx context.Context) { m.effects.Run(ctx, sweepInterval) }
