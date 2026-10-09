// Package attendance wires Module 3 (Attendance, Shifts & Time Tracking) into the application.
package attendance

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/controllers"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/repositories"
	"github.com/jaas/jaas/internal/attendance/routes"
	"github.com/jaas/jaas/internal/attendance/services"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	employeeServices "github.com/jaas/jaas/internal/employee/services"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	identityModels "github.com/jaas/jaas/internal/identity/models"
	identityRepos "github.com/jaas/jaas/internal/identity/repositories"
	identityServices "github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/cache"
	sharedMiddleware "github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// punchRateLimit caps punches per IP (security.md AS-T5).
const punchRateLimit = 30

// Module is the Module 3 DI container.
type Module struct {
	db          *gorm.DB
	controllers routes.Controllers
	relay       *services.OutboxRelay
	leaveSync   services.LeaveSync
}

// NewModule wires repositories, services and controllers over Module 0 audit and Module 2 employees.
func NewModule(db *gorm.DB, publisher queue.EventPublisher, employees employeeServices.EmployeeService, audit identityServices.AuditService) *Module {
	deps := services.Deps{
		Tx: services.NewTxRunner(db),
		Repos: services.Repos{
			Shifts: repositories.NewShiftRepository(), Assignments: repositories.NewAssignmentRepository(),
			Records: repositories.NewRecordRepository(), Punches: repositories.NewPunchRepository(),
			Regularizations: repositories.NewRegularizationRepository(), Outbox: repositories.NewOutboxRepository(),
		},
		Employees: employeeDirectory{svc: employees},
		Audit:     audit,
		Publisher: publisher,
		Now:       time.Now,
	}
	return &Module{
		db: db,
		controllers: routes.Controllers{
			Attendance:     controllers.NewAttendanceController(services.NewPunchService(deps), services.NewQueryService(deps)),
			Regularization: controllers.NewRegularizationController(services.NewRegularizationService(deps)),
			Shift:          controllers.NewShiftController(services.NewShiftService(deps), services.NewAssignmentService(deps)),
		},
		relay:     services.NewOutboxRelay(deps),
		leaveSync: services.NewLeaveSync(deps),
	}
}

// RegisterModels lists Module 3 models for AutoMigrate (dev path; SQL 000025–000030 in prod — A3-06).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&models.Shift{}, &models.ShiftAssignment{}, &models.AttendanceRecord{},
		&models.AttendancePunch{}, &models.Regularization{}, &models.OutboxEvent{}}
}

// SeedPermissions idempotently inserts attendance:read|manage|approve (D3-10, connections C7).
func (m *Module) SeedPermissions(ctx context.Context) error {
	for action, desc := range map[string]string{
		routes.ActionRead:    "View attendance records, summaries, shifts and regularizations",
		routes.ActionManage:  "Create and change shifts and shift assignments",
		routes.ActionApprove: "Approve or reject attendance regularizations",
	} {
		d := desc
		p := &identityModels.Permission{Resource: "attendance", Action: action, Description: &d}
		err := m.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "resource"}, {Name: "action"}}, DoNothing: true,
		}).Create(p).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RouteDeps is the Module 0 + shared infrastructure the routes need (connections C4).
type RouteDeps struct {
	TenantResolver, Authenticate gin.HandlerFunc
	UserRoles                    identityRepos.UserRoleRepository
	RolePerms                    identityRepos.RolePermissionRepository
	Redis                        *cache.RedisClient
}

// RegisterRoutes mounts Module 3 under rg using Module 0 tenant/auth/RBAC middleware.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	mw := routes.Middleware{
		TenantResolver: d.TenantResolver,
		Authenticate:   d.Authenticate,
		Require: func(action string) gin.HandlerFunc {
			return identityMiddleware.RequirePermission(m.db, "attendance", action, d.UserRoles, d.RolePerms)
		},
		PunchLimit: sharedMiddleware.RateLimiter(d.Redis, punchRateLimit, time.Minute),
	}
	routes.RegisterRoutes(rg, mw, m.controllers)
}

// LeaveSync exposes contract C10 for Module 4 (D3-17).
func (m *Module) LeaveSync() services.LeaveSync { return m.leaveSync }

// Relay exposes the outbox relay; cmd/main.go runs it until shutdown.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

// employeeDirectory adapts Module 2 EmployeeService to services.EmployeeDirectory (connections C2).
type employeeDirectory struct {
	svc employeeServices.EmployeeService
}

func (d employeeDirectory) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	return d.svc.GetByUserID(ctx, tenantID, userID)
}

func (d employeeDirectory) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	return d.svc.GetByID(ctx, tenantID, id)
}

// CountWorking sums Module 2 list totals for the statuses allowed to work (AT-002, AT-020).
func (d employeeDirectory) CountWorking(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var total int64
	for _, status := range []string{"active", "probation", "notice"} {
		_, n, err := d.svc.List(ctx, tenantID, employeeDTO.EmployeeFilter{Status: status, Page: 1, PerPage: 1})
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}
