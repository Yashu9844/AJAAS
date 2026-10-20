package access

import (
	"errors"
	"fmt"
	"strings"
)

// Predefined role template keys (docs/ARCHITECTURE.md §17.1).
const (
	TemplateExecutive       = "executive"
	TemplateHRAdmin         = "hr_admin"
	TemplateHRManager       = "hr_manager"
	TemplateManager         = "manager"
	TemplateEmployee        = "employee"
	TemplatePayrollAdmin    = "payroll_admin"
	TemplatePayrollApprover = "payroll_approver"
	TemplateFinance         = "finance"
	TemplateRecruiter       = "recruiter"
	TemplateInterviewer     = "interviewer"
	TemplateITAdmin         = "it_admin"
	TemplateAuditor         = "auditor"
)

// Template describes a predefined role seeded into every tenant.
type Template struct {
	Key         string
	Name        string
	Description string
	Parent      string // template key of the parent role; "" for none
	Automatic   bool   // held automatically from HR facts; never assigned by hand
}

// Templates lists the predefined roles, parents before children.
var Templates = []Template{
	{TemplateEmployee, "Employee", "Every employee: own records and self-service", "", true},
	{TemplateManager, "Manager", "Anyone with direct reports: access to their team", TemplateEmployee, true},
	{TemplateHRManager, "HR Manager", "HR operations across the company", TemplateManager, false},
	{TemplateHRAdmin, "HR Admin", "All of HR, including sensitive data", TemplateHRManager, false},
	{TemplateExecutive, "Executive", "Company-wide visibility and approvals (CEO and leadership)", TemplateHRAdmin, false},
	{TemplatePayrollAdmin, "Payroll Admin", "Prepares and calculates payroll", TemplateEmployee, false},
	{TemplatePayrollApprover, "Payroll Approver", "Approves and finalizes payroll", TemplateEmployee, false},
	{TemplateFinance, "Finance", "Finance team: payroll and cost visibility", TemplateEmployee, false},
	{TemplateRecruiter, "Recruiter", "Runs hiring: jobs, candidates, offers, hires", TemplateEmployee, false},
	{TemplateInterviewer, "Interviewer", "Interviews candidates assigned to them", TemplateEmployee, false},
	{TemplateITAdmin, "IT Admin", "Users, roles and access administration", TemplateEmployee, false},
	{TemplateAuditor, "Auditor", "Read-only access to records and audit trails", TemplateEmployee, false},
}

// TemplateByKey finds a template.
func TemplateByKey(key string) (Template, bool) {
	for _, t := range Templates {
		if t.Key == key {
			return t, true
		}
	}
	return Template{}, false
}

// PermissionDef is one catalogue entry a service registers with M0, including the scope each predefined role
// receives by default.
type PermissionDef struct {
	Module      string
	Resource    string
	Action      string
	Description string
	Sensitive   bool
	Defaults    map[string]Scope // template key → scope
}

// Key returns "resource:action".
func (d PermissionDef) Key() string { return Key(d.Resource, d.Action) }

// ErrInvalidDefinition rejects malformed catalogue entries.
var ErrInvalidDefinition = errors.New("invalid permission definition")

// ValidateDefs checks names, unknown templates and invalid scopes, and duplicate keys.
func ValidateDefs(defs []PermissionDef) error {
	seen := map[string]bool{}
	for _, d := range defs {
		if strings.TrimSpace(d.Module) == "" || strings.TrimSpace(d.Resource) == "" || strings.TrimSpace(d.Action) == "" {
			return fmt.Errorf("%w: module, resource and action are required (%q)", ErrInvalidDefinition, d.Key())
		}
		if seen[d.Key()] {
			return fmt.Errorf("%w: duplicate %s", ErrInvalidDefinition, d.Key())
		}
		seen[d.Key()] = true
		for tpl, s := range d.Defaults {
			if _, ok := TemplateByKey(tpl); !ok || !s.Valid() {
				return fmt.Errorf("%w: %s default %s=%q", ErrInvalidDefinition, d.Key(), tpl, s)
			}
		}
	}
	return nil
}

// DefaultGrants returns the grants a template receives from defs (its own, not inherited).
func DefaultGrants(defs []PermissionDef, templateKey string) []Grant {
	var out []Grant
	for _, d := range defs {
		if s, ok := d.Defaults[templateKey]; ok {
			out = append(out, Grant{Permission: d.Key(), Scope: s})
		}
	}
	return out
}
