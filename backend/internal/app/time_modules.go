package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance"
	"github.com/jaas/jaas/internal/employee"
	"github.com/jaas/jaas/internal/identity"
	"github.com/jaas/jaas/internal/leave"
	"github.com/jaas/jaas/internal/payroll"
)

// peopleModules are the people-operations modules built on Modules 0–2: Attendance (3), Leave (4), Payroll (5).
type peopleModules struct {
	attendance *attendance.Module
	leave      *leave.Module
	payroll    *payroll.Module
}

// seeder is a module that seeds its own permissions (D3-10).
type seeder interface {
	SeedPermissions(ctx context.Context) error
}

// buildPeopleModules wires Modules 3–5: schema (AutoMigrate only in legacy mode; SQL migrations 000026–000044
// otherwise), module-owned permission seeds and the optional outbox relays.
func buildPeopleModules(o Options, id *identity.Module, emp *employee.Module) (*peopleModules, error) {
	att := attendance.NewModule(o.DB, o.Publisher, emp.EmployeeService(), id.AuditService())
	lv := leave.NewModule(o.DB, leave.Ports{Employees: emp.EmployeeService(), Audit: id.AuditService(),
		Attendance: att.LeaveSync(), Publisher: o.Publisher})
	pay := payroll.NewModule(o.DB, payroll.Ports{Employees: emp.EmployeeService(), Leave: lv.UnpaidLeave(),
		Audit: id.AuditService(), Publisher: o.Publisher})
	if o.Config.Database.AutoMigrate {
		all := append(append(att.RegisterModels(), lv.RegisterModels()...), pay.RegisterModels()...)
		if err := o.DB.AutoMigrate(all...); err != nil {
			return nil, fmt.Errorf("people modules auto-migration failed: %w", err)
		}
	}
	for name, s := range map[string]seeder{"attendance": att, "leave": lv, "payroll": pay} {
		if err := s.SeedPermissions(context.Background()); err != nil {
			return nil, fmt.Errorf("%s permission seed failed: %w", name, err)
		}
	}
	if o.Background != nil {
		go att.Relay().Run(o.Background)
		go lv.Relay().Run(o.Background)
		go pay.Relay().Run(o.Background)
	}
	return &peopleModules{attendance: att, leave: lv, payroll: pay}, nil
}

// registerPeopleRoutes mounts Modules 3–5 behind Module 0 tenant/auth/RBAC middleware.
func registerPeopleRoutes(v1 *gin.RouterGroup, o Options, id *identity.Module, m *peopleModules) {
	m.attendance.RegisterRoutes(v1, attendance.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository(), Redis: o.Redis})
	m.leave.RegisterRoutes(v1, leave.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository()})
	m.payroll.RegisterRoutes(v1, payroll.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository()})
}
