package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance"
	"github.com/jaas/jaas/internal/employee"
	"github.com/jaas/jaas/internal/identity"
	"github.com/jaas/jaas/internal/leave"
)

// buildTimeModules wires Module 3 (Attendance) and Module 4 (Leave): schema (AutoMigrate only in legacy mode;
// SQL migrations 000026–000037 otherwise), module-owned permission seeds (D3-10) and the optional outbox relays.
func buildTimeModules(o Options, id *identity.Module, emp *employee.Module) (*attendance.Module, *leave.Module, error) {
	att := attendance.NewModule(o.DB, o.Publisher, emp.EmployeeService(), id.AuditService())
	lv := leave.NewModule(o.DB, leave.Ports{Employees: emp.EmployeeService(), Audit: id.AuditService(),
		Attendance: att.LeaveSync(), Publisher: o.Publisher})
	if o.Config.Database.AutoMigrate {
		if err := o.DB.AutoMigrate(append(att.RegisterModels(), lv.RegisterModels()...)...); err != nil {
			return nil, nil, fmt.Errorf("attendance/leave auto-migration failed: %w", err)
		}
	}
	ctx := context.Background()
	if err := att.SeedPermissions(ctx); err != nil {
		return nil, nil, fmt.Errorf("attendance permission seed failed: %w", err)
	}
	if err := lv.SeedPermissions(ctx); err != nil {
		return nil, nil, fmt.Errorf("leave permission seed failed: %w", err)
	}
	if o.Background != nil {
		go att.Relay().Run(o.Background)
		go lv.Relay().Run(o.Background)
	}
	return att, lv, nil
}

// registerTimeRoutes mounts Modules 3 and 4 behind Module 0 tenant/auth/RBAC middleware.
func registerTimeRoutes(v1 *gin.RouterGroup, o Options, id *identity.Module, att *attendance.Module, lv *leave.Module) {
	att.RegisterRoutes(v1, attendance.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository(), Redis: o.Redis})
	lv.RegisterRoutes(v1, leave.RouteDeps{TenantResolver: id.TenantResolver(), Authenticate: id.AuthMiddleware(),
		UserRoles: id.UserRoleRepository(), RolePerms: id.RolePermissionRepository()})
}
