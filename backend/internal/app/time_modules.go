package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance"
	"github.com/jaas/jaas/internal/employee"
	"github.com/jaas/jaas/internal/identity"
	"github.com/jaas/jaas/internal/leave"
	"github.com/jaas/jaas/internal/organization"
	"github.com/jaas/jaas/internal/payroll"
	"github.com/jaas/jaas/internal/recruitment"
)

// peopleModules are the people-operations modules built on Modules 0–2: Attendance (3), Leave (4), Payroll (5),
// Recruitment (6).
type peopleModules struct {
	attendance  *attendance.Module
	leave       *leave.Module
	payroll     *payroll.Module
	recruitment *recruitment.Module
}

// coreModules are the Modules 0–2 the people modules consume.
type coreModules struct {
	identity *identity.Module
	org      *organization.Module
	employee *employee.Module
}

// seeder is a module that seeds its own permissions (D3-10).
type seeder interface {
	SeedPermissions(ctx context.Context) error
}

// buildPeopleModules wires Modules 3–6: schema (AutoMigrate only in legacy mode; SQL migrations 000026–000050
// otherwise), module-owned permission seeds and the optional outbox relays.
func buildPeopleModules(o Options, core coreModules) (*peopleModules, error) {
	id, emp := core.identity, core.employee
	att := attendance.NewModule(o.DB, o.Publisher, emp.EmployeeService(), id.AuditService())
	lv := leave.NewModule(o.DB, leave.Ports{Employees: emp.EmployeeService(), Audit: id.AuditService(),
		Attendance: att.LeaveSync(), Publisher: o.Publisher})
	pay := payroll.NewModule(o.DB, payroll.Ports{Employees: emp.EmployeeService(), Leave: lv.UnpaidLeave(),
		Audit: id.AuditService(), Publisher: o.Publisher})
	rec := recruitment.NewModule(o.DB, recruitment.Ports{Employees: emp.EmployeeService(), Users: id.UserService(),
		Departments: core.org.DepartmentService(), Designations: core.org.DesignationService(), Audit: id.AuditService(), Publisher: o.Publisher})
	m := &peopleModules{attendance: att, leave: lv, payroll: pay, recruitment: rec}
	if err := m.prepare(o); err != nil {
		return nil, err
	}
	return m, nil
}

// prepare migrates (legacy mode only), seeds permissions and starts the relays.
func (m *peopleModules) prepare(o Options) error {
	if o.Config.Database.AutoMigrate {
		var all []interface{}
		for _, models := range [][]interface{}{m.attendance.RegisterModels(), m.leave.RegisterModels(), m.payroll.RegisterModels(), m.recruitment.RegisterModels()} {
			all = append(all, models...)
		}
		if err := o.DB.AutoMigrate(all...); err != nil {
			return fmt.Errorf("people modules auto-migration failed: %w", err)
		}
	}
	for name, s := range map[string]seeder{"attendance": m.attendance, "leave": m.leave, "payroll": m.payroll, "recruitment": m.recruitment} {
		if err := s.SeedPermissions(context.Background()); err != nil {
			return fmt.Errorf("%s permission seed failed: %w", name, err)
		}
	}
	if o.Background != nil {
		go m.attendance.Relay().Run(o.Background)
		go m.leave.Relay().Run(o.Background)
		go m.payroll.Relay().Run(o.Background)
		go m.recruitment.Relay().Run(o.Background)
	}
	return nil
}

// registerPeopleRoutes mounts Modules 3–6 behind Module 0 tenant/auth/RBAC middleware.
func registerPeopleRoutes(v1 *gin.RouterGroup, o Options, id *identity.Module, m *peopleModules) {
	m.attendance.RegisterRoutes(v1, attendance.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository(), Redis: o.Redis})
	m.leave.RegisterRoutes(v1, leave.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository()})
	m.payroll.RegisterRoutes(v1, payroll.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository()})
	m.recruitment.RegisterRoutes(v1, recruitment.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository()})
}
