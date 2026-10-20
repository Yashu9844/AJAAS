// Package leave wires Module 4 (Leave & Absence Management) into the application.
package leave

import (
	"time"

	"github.com/gin-gonic/gin"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	"github.com/jaas/jaas/internal/leave/controllers"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/repositories"
	"github.com/jaas/jaas/internal/leave/routes"
	"github.com/jaas/jaas/internal/leave/services"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Module is the Module 4 DI container.
type Module struct {
	db          *gorm.DB
	controllers routes.Controllers
	relay       *services.OutboxRelay
	unpaid      services.UnpaidLeave
}

// Ports are the cross-module collaborators Module 4 consumes (connections C2, C3, C8).
type Ports struct {
	Employees  services.EmployeeDirectory // Module 2 EmployeeService
	Audit      services.AuditLogger       // Module 0 AuditService
	Attendance services.AttendanceSync    // Module 3 LeaveSync
	Publisher  queue.EventPublisher
}

// NewModule wires repositories, services and controllers.
func NewModule(db *gorm.DB, p Ports) *Module {
	deps := services.Deps{
		Tx: services.NewTxRunner(db),
		Repos: services.Repos{
			Types: repositories.NewTypeRepository(), Holidays: repositories.NewHolidayRepository(),
			Balances: repositories.NewBalanceRepository(), Requests: repositories.NewRequestRepository(),
			Ledger: repositories.NewLedgerRepository(), Outbox: repositories.NewOutboxRepository(),
		},
		Employees: p.Employees, Audit: p.Audit, Attendance: p.Attendance, Publisher: p.Publisher,
		Now: time.Now,
	}
	return &Module{
		db: db,
		controllers: routes.Controllers{
			Policy:  controllers.NewPolicyController(services.NewTypeService(deps), services.NewHolidayService(deps)),
			Balance: controllers.NewBalanceController(services.NewBalanceService(deps)),
			Request: controllers.NewRequestController(services.NewRequestService(deps)),
		},
		relay:  services.NewOutboxRelay(deps),
		unpaid: services.NewUnpaidLeave(deps),
	}
}

// RegisterModels lists Module 4 models for AutoMigrate (dev path; SQL 000032–000037 in prod).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&models.LeaveType{}, &models.Holiday{}, &models.Balance{}, &models.Request{},
		&models.LedgerEntry{}, &models.OutboxEvent{}}
}


// RouteDeps is the Module 0 infrastructure the routes need (connections C4).
type RouteDeps struct {
	TenantResolver, Authenticate gin.HandlerFunc
	Authz                        identityMiddleware.Authorizer
}

// RegisterRoutes mounts Module 4 under rg using Module 0 tenant/auth/RBAC middleware.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{
		TenantResolver: d.TenantResolver,
		Authenticate:   d.Authenticate,
		Require: func(action string) gin.HandlerFunc {
			return identityMiddleware.RequirePermission(d.Authz, "leave", action)
		},
	}, m.controllers)
}

// UnpaidLeave exposes the read-only loss-of-pay port for Module 5 (D5-03).
func (m *Module) UnpaidLeave() services.UnpaidLeave { return m.unpaid }

// Relay exposes the outbox relay; cmd/main.go runs it until shutdown.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }
