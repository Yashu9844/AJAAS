package recruitment

import (
	"github.com/jaas/jaas/internal/hr/recruitment/routes"
	"github.com/jaas/jaas/internal/identity/access"
)

// Catalogue is HR/Recruitment's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md
// §17). The app registers it with M0 at boot.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "recruitment", Action: routes.ActionRead, Description: "View job openings, candidates, interviews and offers", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateRecruiter: access.ScopeTenant, access.TemplateHRManager: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "recruitment", Action: routes.ActionManage, Description: "Manage job openings, candidates, interviews and offers", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateRecruiter: access.ScopeTenant, access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "recruitment", Action: routes.ActionHire, Description: "Hire candidates (creates a user invite and an employee profile)", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateRecruiter: access.ScopeTenant, access.TemplateHRAdmin: access.ScopeTenant}},
	}
}
