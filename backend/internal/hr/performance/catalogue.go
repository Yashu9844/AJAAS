package performance

import (
	"github.com/jaas/jaas/internal/hr/performance/routes"
	"github.com/jaas/jaas/internal/identity/access"
)

// Catalogue is HR/Performance's permission catalogue with the scopes predefined roles receive by default
// (docs/ARCHITECTURE.md §17). Self, manager and reviewer access needs no permission (PM-008).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "performance", Action: routes.ActionRead, Description: "View every goal, review and the 9-box", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "performance", Action: routes.ActionManage, Description: "Run appraisal cycles, manage any goal and assign reviewers", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "performance", Action: routes.ActionCalibrate, Description: "Calibrate final ratings (score and potential) with a reason", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
	}
}
