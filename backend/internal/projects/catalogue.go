package projects

import (
	"strings"

	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/projects/api"
)

// projectTemplates are the M0 project role templates, from the least to the most privileged, with the built-in role each
// one carries (DPJ-49).
var projectTemplates = []struct{ template, role string }{
	{access.TemplateProjectViewer, api.RoleViewer},
	{access.TemplateProjectMember, api.RoleMember},
	{access.TemplateProjectManager, api.RoleManager},
	{access.TemplateProjectOwner, api.RoleOwner},
}

// projectPermissionDefs registers every project-level permission in the M0 catalogue as "resource:action" (project.read is
// project:read) and gives each predefined project role the permissions of the built-in role it replaces.
func projectPermissionDefs() []access.PermissionDef {
	defs := make([]access.PermissionDef, 0, len(api.AllPerms))
	for _, p := range api.AllPerms {
		resource, action, _ := strings.Cut(string(p), ".")
		d := access.PermissionDef{Module: "projects", Resource: resource, Action: action, AppliesTo: access.AppliesProject,
			Description: "Project permission " + string(p), Defaults: map[string]access.Scope{}}
		for _, t := range projectTemplates {
			if api.BuiltinPerms(t.role)[p] {
				d.Defaults[t.template] = access.ScopeTenant
			}
		}
		defs = append(defs, d)
	}
	return defs
}
