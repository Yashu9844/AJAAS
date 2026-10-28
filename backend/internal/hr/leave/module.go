// Package leave wires Module 4 (Leave & Absence Management) into the application.
package leave

import (
	"context"
	attendanceAPI "github.com/jaas/jaas/internal/hr/attendance/api"
	"github.com/jaas/jaas/internal/hr/leave/api"
	"github.com/jaas/jaas/internal/hr/leave/plus"
	"github.com/jaas/jaas/internal/shared/identityport"
	"time"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/hr/leave/controllers"
	"github.com/jaas/jaas/internal/hr/leave/models"
	"github.com/jaas/jaas/internal/hr/leave/repositories"
	"github.com/jaas/jaas/internal/hr/leave/routes"
	"github.com/jaas/jaas/internal/hr/leave/services"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Module is the Module 4 DI container.
type Module struct {
	db          *gorm.DB
	controllers routes.Controllers
	relay       *services.OutboxRelay
	unpaid      services.UnpaidLeave
	holidays    repositories.HolidayRepository
	plus        *plus.Module
	cal         *plus.Calendar
}

// Ports are the cross-module collaborators Module 4 consumes (connections C2, C3, C8).
type Ports struct {
	Employees  services.EmployeeDirectory // Module 2 EmployeeService
	Audit      services.AuditLogger       // Module 0 AuditService
	Attendance services.AttendanceSync    // Module 3 LeaveSync
	Reports    plus.Reports               // HR/Organization direct reports (LP-06)
	Presence   attendanceAPI.Presence     // HR/Attendance worked time (LP-08)
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
	cal := plus.NewCalendarFor(db)
	deps.Calendar = cal
	requests := services.NewRequestService(deps)
	parity := plus.NewModule(db, cal, plus.Ports{Employees: p.Employees, Reports: p.Reports, Requests: requests, Mover: services.NewBalanceMover(deps),
		Presence: p.Presence, Audit: p.Audit})
	return &Module{
		db: db,
		controllers: routes.Controllers{
			Policy:  controllers.NewPolicyController(services.NewTypeService(deps), services.NewHolidayService(deps)),
			Balance: controllers.NewBalanceController(services.NewBalanceService(deps)),
			Request: controllers.NewRequestController(requests),
		},
		relay:    services.NewOutboxRelay(deps),
		unpaid:   services.NewUnpaidLeave(deps),
		holidays: deps.Repos.Holidays,
		plus:     parity,
		cal:      cal,
	}
}

// RegisterModels lists Module 4 models for AutoMigrate (dev path; SQL 000032–000037 in prod).
func (m *Module) RegisterModels() []interface{} {
	return append([]interface{}{&models.LeaveType{}, &models.Holiday{}, &models.Balance{}, &models.Request{},
		&models.LedgerEntry{}, &models.OutboxEvent{}}, m.plus.RegisterModels()...)
}

// RouteDeps is the Module 0 infrastructure the routes need (connections C4).
type RouteDeps struct {
	Identity identityport.HTTP
}

// RegisterRoutes mounts Module 4 under rg using Module 0 tenant/auth/RBAC middleware.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{
		TenantResolver: d.Identity.TenantResolver(),
		Authenticate:   d.Identity.Authenticate(),
		Require: func(action string) gin.HandlerFunc {
			return d.Identity.Require("leave", action)
		},
	}, m.controllers)
	m.plus.RegisterRoutes(rg, d.Identity)
}

// UnpaidLeave exposes the read-only loss-of-pay port for Module 5 (D5-03).
func (m *Module) UnpaidLeave() services.UnpaidLeave { return m.unpaid }

// Relay exposes the outbox relay; cmd/main.go runs it until shutdown.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

// Holidays is the holiday-calendar port other HR contexts use (leave/api).
func (m *Module) Holidays() api.Holidays { return holidayPort{repo: m.holidays, cal: m.cal} }

type holidayPort struct {
	repo repositories.HolidayRepository
	cal  *plus.Calendar
}

func (h holidayPort) IsHolidayFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (bool, *bool, error) {
	return h.cal.DayKind(ctx, db, tenantID, employeeID, date)
}

func (h holidayPort) IsHoliday(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (bool, error) {
	row, err := h.repo.FindByDate(ctx, db, tenantID, date)
	return err == nil && row != nil && !row.IsOptional, err
}

// SetLocations installs the employee-location source once HR/Core exists (LP-05).
func (m *Module) SetLocations(l plus.Locations) { m.cal.SetLocations(l) }

// RunCompOffExpiry runs the daily comp-off expiry sweep until ctx is cancelled (LP-10).
func (m *Module) RunCompOffExpiry(ctx context.Context, onError func(error)) {
	m.plus.RunCompOffExpiry(ctx, onError)
}

// Settlement is the port payroll final settlement uses to read encashable balances and mark encashments paid.
func (m *Module) Settlement() api.Settlement { return m.plus.Settlement() }
