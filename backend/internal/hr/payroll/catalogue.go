package payroll

import (
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/hr/payroll/routes"
)

// Catalogue is HR/Payroll's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md
// §17). The app registers it with M0 at boot.
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "payroll", Action: routes.ActionRead, Description: "View salary structures, assignments, payroll runs and payslips", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplatePayrollAdmin: access.ScopeTenant, access.TemplatePayrollApprover: access.ScopeTenant, access.TemplateFinance: access.ScopeTenant, access.TemplateHRAdmin: access.ScopeTenant, access.TemplateAuditor: access.ScopeTenant}},
		{Module: "hr", Resource: "payroll", Action: routes.ActionManage, Description: "Manage salary structures and assignments; create and calculate payroll runs", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplatePayrollAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "payroll", Action: routes.ActionApprove, Description: "Approve and finalize payroll runs", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplatePayrollApprover: access.ScopeTenant}},
	}
}
