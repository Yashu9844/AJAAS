// Package payroll wires Module 5 (Payroll & Statutory Compliance) into the application.
package payroll

import (
	"github.com/jaas/jaas/internal/hr/identityport"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/hr/payroll/controllers"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"github.com/jaas/jaas/internal/hr/payroll/repositories"
	"github.com/jaas/jaas/internal/hr/payroll/routes"
	"github.com/jaas/jaas/internal/hr/payroll/services"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Module is the Module 5 DI container.
type Module struct {
	db          *gorm.DB
	controllers routes.Controllers
	relay       *services.OutboxRelay
}

// Ports are the cross-module collaborators Module 5 consumes (connections C2–C4).
type Ports struct {
	Employees services.EmployeeDirectory // Module 2 EmployeeService
	Leave     services.LeaveLOP          // Module 4 UnpaidLeave
	Audit     services.AuditLogger       // Module 0 AuditService
	Publisher queue.EventPublisher
}

// NewModule wires repositories, services and controllers.
func NewModule(db *gorm.DB, p Ports) *Module {
	deps := services.Deps{
		Tx: services.NewTxRunner(db),
		Repos: services.Repos{Structures: repositories.NewStructureRepository(), Assignments: repositories.NewAssignmentRepository(),
			Runs: repositories.NewRunRepository(), Payslips: repositories.NewPayslipRepository(), Outbox: repositories.NewOutboxRepository()},
		Employees: p.Employees, Leave: p.Leave, Audit: p.Audit, Publisher: p.Publisher, Now: time.Now,
	}
	return &Module{
		db: db,
		controllers: routes.Controllers{
			Setup: controllers.NewSetupController(services.NewStructureService(deps), services.NewAssignmentService(deps)),
			Run:   controllers.NewRunController(services.NewRunService(deps), services.NewPayslipService(deps)),
		},
		relay: services.NewOutboxRelay(deps),
	}
}

// RegisterModels lists Module 5 models for legacy AutoMigrate (SQL 000038–000044 is the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&models.Structure{}, &models.Component{}, &models.Assignment{}, &models.Run{},
		&models.Payslip{}, &models.PayslipLine{}, &models.OutboxEvent{}}
}

// RouteDeps is the Module 0 infrastructure the routes need.
type RouteDeps struct {
	Identity identityport.HTTP
}

// RegisterRoutes mounts Module 5 under rg.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{TenantResolver: d.Identity.TenantResolver(), Authenticate: d.Identity.Authenticate(),
		Require: func(action string) gin.HandlerFunc {
			return d.Identity.Require("payroll", action)
		}}, m.controllers)
}

// Relay exposes the outbox relay.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }
