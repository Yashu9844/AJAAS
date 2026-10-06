package models

import "testing"

func TestDefaultPermissionsUniqueAndComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range DefaultPermissions {
		k := p.Resource + ":" + p.Action
		if seen[k] {
			t.Fatalf("duplicate permission %s", k)
		}
		seen[k] = true
		if p.Description == "" {
			t.Errorf("%s has no description", k)
		}
	}
	// every permission the Module 0-2 routes check must be in the catalogue
	for _, need := range []string{
		"users:create", "users:read", "users:update", "users:delete",
		"roles:create", "roles:read", "roles:update", "roles:delete", "permissions:read", "audit:read",
		"organization:read", "organization:update",
		"employee:read", "employee:create", "employee:update", "employee:read_sensitive", "employee:update_sensitive", "employee:admin",
	} {
		if !seen[need] {
			t.Errorf("catalogue is missing %s", need)
		}
	}
}
