// Package attendance wires Module 3 (Attendance, Shifts & Time Tracking) into the application.
package attendance

import (
	"context"
	"github.com/jaas/jaas/internal/hr/attendance/api"
	"github.com/jaas/jaas/internal/hr/identityport"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/attendance/controllers"
	"github.com/jaas/jaas/internal/hr/attendance/models"
	"github.com/jaas/jaas/internal/hr/attendance/repositories"
	"github.com/jaas/jaas/internal/hr/attendance/routes"
	"github.com/jaas/jaas/internal/hr/attendance/services"
	employeeAPI "github.com/jaas/jaas/internal/hr/employee/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/cache"
	sharedMiddleware "github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// punchRateLimit caps punches per IP (security.md AS-T5).
const punchRateLimit = 30

// Module is the Module 3 DI container.
type Module struct {
	db          *gorm.DB
	controllers routes.Controllers
	relay       *services.OutboxRelay
	leaveSync   services.LeaveSync
	writer      services.TimeWriter
	regs        services.RegularizationService
	records     repositories.RecordRepository
}

// NewModule wires repositories, services and controllers over Module 0 audit and Module 2 employees.
func NewModule(db *gorm.DB, publisher queue.EventPublisher, employees employeeAPI.Directory, audit identityport.Auditor) *Module {
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
		writer:    services.NewTimeWriter(deps),
		regs:      services.NewRegularizationService(deps),
		records:   deps.Repos.Records,
	}
}

// RegisterModels lists Module 3 models for AutoMigrate (dev path; SQL 000026–000031 in prod — A3-06).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&models.Shift{}, &models.ShiftAssignment{}, &models.AttendanceRecord{},
		&models.AttendancePunch{}, &models.Regularization{}, &models.OutboxEvent{}}
}

// RouteDeps is the Module 0 + shared infrastructure the routes need (connections C4).
type RouteDeps struct {
	Identity identityport.HTTP
	Redis    *cache.RedisClient
}

// RegisterRoutes mounts Module 3 under rg using Module 0 tenant/auth/RBAC middleware.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	mw := routes.Middleware{
		TenantResolver: d.Identity.TenantResolver(),
		Authenticate:   d.Identity.Authenticate(),
		Require: func(action string) gin.HandlerFunc {
			return d.Identity.Require("attendance", action)
		},
		PunchLimit: sharedMiddleware.RateLimiter(d.Redis, punchRateLimit, time.Minute),
	}
	routes.RegisterRoutes(rg, mw, m.controllers)
}

// LeaveSync exposes contract C10 for Module 4 (D3-17).
func (m *Module) LeaveSync() services.LeaveSync { return m.leaveSync }

// Relay exposes the outbox relay; cmd/main.go runs it until shutdown.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

// employeeDirectory adapts the HR/Employee directory to services.EmployeeDirectory (connections C2).
type employeeDirectory struct {
	svc employeeAPI.Directory
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

// TimeWriter is the write port the parity package (attendance/ops) uses.
func (m *Module) TimeWriter() services.TimeWriter { return m.writer }

// Regularizations is the regularization service the manager inbox decides through (AP-02).
func (m *Module) Regularizations() services.RegularizationService { return m.regs }

// Presence is the worked-time port other HR contexts use (attendance/api).
func (m *Module) Presence() api.Presence { return presence{records: m.records} }

type presence struct{ records repositories.RecordRepository }

func (p presence) WorkedMinutes(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (int, error) {
	rec, err := p.records.FindByDate(ctx, db, tenantID, employeeID, date)
	if err != nil || rec == nil {
		return 0, err
	}
	return rec.TotalWorkMinutes, nil
}
