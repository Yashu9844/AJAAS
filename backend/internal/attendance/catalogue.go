package attendance

import (
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/attendance/routes"
)

// Catalogue is HR/Attendance's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md
// §17). The app registers it with M0 at boot.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "attendance", Action: routes.ActionRead, Description: "View attendance records, summaries, shifts and regularizations", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateManager: access.ScopeAllReports, access.TemplateHRManager: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "attendance", Action: routes.ActionManage, Description: "Create and change shifts and shift assignments", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "attendance", Action: routes.ActionApprove, Description: "Approve or reject attendance regularizations", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateManager: access.ScopeDirectReports, access.TemplateHRManager: access.ScopeTenant}},
	}
}
