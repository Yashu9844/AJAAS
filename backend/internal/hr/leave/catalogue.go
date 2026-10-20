package leave

import (
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/hr/leave/routes"
)

// Catalogue is HR/Leave's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md
// §17). The app registers it with M0 at boot.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "leave", Action: routes.ActionRead, Description: "View leave requests, balances and ledgers of other employees", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateManager: access.ScopeAllReports, access.TemplateHRManager: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "leave", Action: routes.ActionManage, Description: "Manage leave types, holidays and balance adjustments", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "leave", Action: routes.ActionApprove, Description: "Approve or reject leave requests", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateManager: access.ScopeDirectReports, access.TemplateHRManager: access.ScopeTenant}},
	}
}
