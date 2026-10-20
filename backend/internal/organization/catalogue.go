package organization

import (
	"github.com/jaas/jaas/internal/identity/access"
)

// Catalogue is HR/Organization's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md
// §17). The app registers it with M0 at boot.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "organization", Action: "read", Description: "View departments, teams, designations, mappings, org chart", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateEmployee: access.ScopeTenant}},
		{Module: "hr", Resource: "organization", Action: "update", Description: "Manage departments, teams, designations, mappings", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
	}
}
