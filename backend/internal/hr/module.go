// Package hr is the composition root of M1 HR Management: it builds every bounded context, wires the contexts to
// each other through their ports, registers HR permissions and automatic-role facts with M0 through identityport,
// starts the background relays and mounts every HR route under one group (/api/v1/hr).
// Design: docs/modules/m1-hr-management/design.md.
package hr

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/attendance"
	"github.com/jaas/jaas/internal/hr/employee"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/leave"
	"github.com/jaas/jaas/internal/hr/organization"
	"github.com/jaas/jaas/internal/hr/payroll"
	"github.com/jaas/jaas/internal/hr/recruitment"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/queue"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// Options are the infrastructure handles HR needs.
type Options struct {
	DB          *gorm.DB
	Redis       *cache.RedisClient
	Publisher   queue.EventPublisher
	Logger      *zerolog.Logger
	Identity    identityport.Port
	AutoMigrate bool            // legacy/dev only; SQL migrations are the schema authority
	Background  context.Context // nil: no relays (tests)
}

// Module is the HR service.
type Module struct {
	o           Options
	org         *organization.Module
	employee    *employee.Module
	attendance  *attendance.Module
	leave       *leave.Module
	payroll     *payroll.Module
	recruitment *recruitment.Module
}

// New builds and prepares every HR context.
func New(ctx context.Context, o Options) (*Module, error) {
	m := &Module{o: o}
	m.org = organization.NewModule(o.DB, o.Redis, o.Publisher, o.Logger, o.Identity)
	m.employee = employee.NewModule(o.DB, o.Redis, o.Publisher, o.Logger, o.Identity)
	emp := m.employee.Directory()
	m.attendance = attendance.NewModule(o.DB, o.Publisher, emp, o.Identity.Audit())
	m.leave = leave.NewModule(o.DB, leave.Ports{Employees: emp, Audit: o.Identity.Audit(), Attendance: m.attendance.LeaveSync(), Publisher: o.Publisher})
	m.payroll = payroll.NewModule(o.DB, payroll.Ports{Employees: emp, Leave: m.leave.UnpaidLeave(), Audit: o.Identity.Audit(), Publisher: o.Publisher})
	m.recruitment = recruitment.NewModule(o.DB, recruitment.Ports{Employees: emp, Users: o.Identity.Users(),
		Departments: m.org.DepartmentService(), Designations: m.org.DesignationService(), Audit: o.Identity.Audit(), Publisher: o.Publisher})

	// FR-M005 / FR-EV001: deactivating a login converges org mappings and the employee profile in the same transaction.
	o.Identity.OnUserDeactivated(m.org.UserDeactivationConverger())
	o.Identity.OnUserDeactivated(m.employee.UserDeactivationConverger())
	o.Identity.ProvideRoleFacts(roleFacts{employees: m.employee, org: m.org})
	if err := m.migrate(); err != nil {
		return nil, err
	}
	if err := m.registerPermissions(ctx); err != nil {
		return nil, err
	}
	m.startRelays()
	return m, nil
}

func (m *Module) migrate() error {
	if !m.o.AutoMigrate {
		return nil
	}
	var all []interface{}
	for _, ms := range [][]interface{}{m.org.RegisterModels(), m.employee.RegisterModels(), m.attendance.RegisterModels(),
		m.leave.RegisterModels(), m.payroll.RegisterModels(), m.recruitment.RegisterModels()} {
		all = append(all, ms...)
	}
	if err := m.o.DB.AutoMigrate(all...); err != nil {
		return fmt.Errorf("hr auto-migration failed: %w", err)
	}
	return nil
}

// registerPermissions registers every context's catalogue (with predefined-role defaults) in M0.
func (m *Module) registerPermissions(ctx context.Context) error {
	for name, defs := range map[string][]access.PermissionDef{
		"organization": organization.Catalogue(), "employee": employee.Catalogue(), "attendance": attendance.Catalogue(),
		"leave": leave.Catalogue(), "payroll": payroll.Catalogue(), "recruitment": recruitment.Catalogue(),
	} {
		if err := m.o.Identity.RegisterPermissions(ctx, defs); err != nil {
			return fmt.Errorf("%s permission catalogue: %w", name, err)
		}
	}
	return nil
}

func (m *Module) startRelays() {
	if m.o.Background == nil {
		return
	}
	go m.attendance.Relay().Run(m.o.Background)
	go m.leave.Relay().Run(m.o.Background)
	go m.payroll.Relay().Run(m.o.Background)
	go m.recruitment.Relay().Run(m.o.Background)
}

// RegisterRoutes mounts every HR context under rg (the /api/v1/hr group).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	idp := m.o.Identity
	m.org.RegisterRoutes(rg, idp)
	m.employee.RegisterRoutes(rg, idp)
	m.attendance.RegisterRoutes(rg, attendance.RouteDeps{Identity: idp, Redis: m.o.Redis})
	m.leave.RegisterRoutes(rg, leave.RouteDeps{Identity: idp})
	m.payroll.RegisterRoutes(rg, payroll.RouteDeps{Identity: idp})
	m.recruitment.RegisterRoutes(rg, recruitment.RouteDeps{Identity: idp})
}

// roleFacts adapts HR/Employee and HR/Organization to identityport.RoleFacts: employees hold the automatic Employee
// role, people with active direct reports the Manager role.
type roleFacts struct {
	employees *employee.Module
	org       *organization.Module
}

func (f roleFacts) Members(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	emps, err := f.employees.EmployedUserIDs(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	mgrs, err := f.org.ManagerUserIDs(ctx, tenantID)
	return emps, mgrs, err
}

func (f roleFacts) MemberOf(ctx context.Context, tenantID, userID uuid.UUID) (bool, bool, error) {
	emp, err := f.employees.IsEmployed(ctx, tenantID, userID)
	if err != nil {
		return false, false, err
	}
	mgr, err := f.org.HasDirectReports(ctx, tenantID, userID)
	return emp, mgr, err
}
