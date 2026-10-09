// Package payroll wires Module 5 (Payroll & Statutory Compliance) into the application.
package payroll

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	identityModels "github.com/jaas/jaas/internal/identity/models"
	identityRepos "github.com/jaas/jaas/internal/identity/repositories"
	"github.com/jaas/jaas/internal/payroll/controllers"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/repositories"
	"github.com/jaas/jaas/internal/payroll/routes"
	"github.com/jaas/jaas/internal/payroll/services"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// SeedPermissions idempotently inserts payroll:read|manage|approve (connections C7).
func (m *Module) SeedPermissions(ctx context.Context) error {
	for action, desc := range map[string]string{
		routes.ActionRead:    "View salary structures, assignments, payroll runs and payslips",
		routes.ActionManage:  "Manage salary structures and assignments; create and calculate payroll runs",
		routes.ActionApprove: "Approve and finalize payroll runs",
	} {
		d := desc
		err := m.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "resource"}, {Name: "action"}}, DoNothing: true}).
			Create(&identityModels.Permission{Resource: "payroll", Action: action, Description: &d}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RouteDeps is the Module 0 infrastructure the routes need.
type RouteDeps struct {
	TenantResolver, Authenticate gin.HandlerFunc
	UserRoles                    identityRepos.UserRoleRepository
	RolePerms                    identityRepos.RolePermissionRepository
}

// RegisterRoutes mounts Module 5 under rg.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{TenantResolver: d.TenantResolver, Authenticate: d.Authenticate,
		Require: func(action string) gin.HandlerFunc {
			return identityMiddleware.RequirePermission(m.db, "payroll", action, d.UserRoles, d.RolePerms)
		}}, m.controllers)
}

// Relay exposes the outbox relay.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }
