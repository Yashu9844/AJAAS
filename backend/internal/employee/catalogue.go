package employee

import (
	"github.com/jaas/jaas/internal/identity/access"
)

// Catalogue is HR/Employee's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md
// §17). The app registers it with M0 at boot.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "employee", Action: "read", Description: "View employee directory and profiles (masked PII)", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateManager: access.ScopeAllReports, access.TemplateHRManager: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "employee", Action: "create", Description: "Create employee profiles", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "employee", Action: "update", Description: "Update employee profiles, status and documents", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant}},
		{Module: "hr", Resource: "employee", Action: "read_sensitive", Description: "View unmasked statutory / bank details", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "employee", Action: "update_sensitive", Description: "Edit statutory / bank details", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "employee", Action: "admin", Description: "Verify employee documents and administer employees", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
	}
}
