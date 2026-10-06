package models

// PermissionSeed is one row of the global permission catalogue.
type PermissionSeed struct {
	Resource    string
	Action      string
	Description string
}

// DefaultPermissions is the catalogue enforced by the routes of Modules 0-2. It is seeded idempotently at startup
// (the permissions table is global and has no create API).
var DefaultPermissions = []PermissionSeed{
	{"users", "create", "Create users"},
	{"users", "read", "View users"},
	{"users", "update", "Update users, assign/remove roles, activate"},
	{"users", "delete", "Deactivate users"},
	{"roles", "create", "Create roles"},
	{"roles", "read", "View roles"},
	{"roles", "update", "Update roles and their permissions"},
	{"roles", "delete", "Delete roles"},
	{"permissions", "read", "View the permission catalogue"},
	{"audit", "read", "View the audit trail"},
	{"organization", "read", "View departments, teams, designations, mappings, org chart"},
	{"organization", "update", "Manage departments, teams, designations, mappings"},
	{"employee", "read", "View employee directory and profiles (masked PII)"},
	{"employee", "create", "Create employee profiles"},
	{"employee", "update", "Update employee profiles, status and documents"},
	{"employee", "read_sensitive", "View unmasked statutory / bank details"},
	{"employee", "update_sensitive", "Edit statutory / bank details"},
	{"employee", "admin", "Verify employee documents and administer employees"},
}
