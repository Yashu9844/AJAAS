package core

import (
	"github.com/jaas/jaas/internal/hr/core/routes"
	"github.com/jaas/jaas/internal/identity/access"
)

// Catalogue is HR/Core's permission catalogue with the scopes predefined roles receive by default
// (docs/ARCHITECTURE.md §17). Approving is separate from initiating (DC-02).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "core", Action: routes.ActionRead, Description: "View job architecture, positions, assignments, HR actions and headcount", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "core", Action: routes.ActionManage, Description: "Manage job architecture, positions and headcount plans; initiate HR actions", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "core", Action: routes.ActionApprove, Description: "Approve, reject or rescind HR actions (never one's own)", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
	}
}
