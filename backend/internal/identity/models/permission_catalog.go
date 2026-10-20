package models

import "github.com/jaas/jaas/internal/identity/access"

const identityModule = "identity"

func tenantWide(templates ...string) map[string]access.Scope {
	out := make(map[string]access.Scope, len(templates))
	for _, t := range templates {
		out[t] = access.ScopeTenant
	}
	return out
}

// IdentityCatalogue is M0's own permission catalogue with the scopes predefined roles receive by default
// (docs/ARCHITECTURE.md §17). Other services register theirs through identity.Module.RegisterCatalogue.
var IdentityCatalogue = []access.PermissionDef{
	{Module: identityModule, Resource: "users", Action: "create", Description: "Create users",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin)},
	{Module: identityModule, Resource: "users", Action: "read", Description: "View users",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRManager, access.TemplateAuditor)},
	{Module: identityModule, Resource: "users", Action: "update", Description: "Update users, assign/remove roles, activate",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin)},
	{Module: identityModule, Resource: "users", Action: "delete", Description: "Deactivate users",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin)},
	{Module: identityModule, Resource: "roles", Action: "create", Description: "Create roles",
		Defaults: tenantWide(access.TemplateITAdmin)},
	{Module: identityModule, Resource: "roles", Action: "read", Description: "View roles",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin, access.TemplateAuditor)},
	{Module: identityModule, Resource: "roles", Action: "update", Description: "Update roles, their grants and inheritance",
		Defaults: tenantWide(access.TemplateITAdmin)},
	{Module: identityModule, Resource: "roles", Action: "delete", Description: "Delete roles",
		Defaults: tenantWide(access.TemplateITAdmin)},
	{Module: identityModule, Resource: "permissions", Action: "read", Description: "View the permission catalogue",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin, access.TemplateAuditor)},
	{Module: identityModule, Resource: "audit", Action: "read", Description: "View the audit trail",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateAuditor, access.TemplateExecutive)},
	{Module: identityModule, Resource: "access", Action: "read", Description: "View any user's effective access and its sources",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin, access.TemplateAuditor)},
	{Module: identityModule, Resource: "access", Action: "manage", Description: "Grant, block and revoke individual user permissions; sync automatic roles",
		Defaults: tenantWide(access.TemplateITAdmin, access.TemplateHRAdmin)},
}
