package models

import (
	"testing"

	"github.com/jaas/jaas/internal/identity/access"
)

func TestIdentityCatalogueValidAndComplete(t *testing.T) {
	if err := access.ValidateDefs(IdentityCatalogue); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range IdentityCatalogue {
		seen[d.Key()] = true
		if d.Description == "" {
			t.Errorf("%s has no description", d.Key())
		}
	}
	// every permission the M0 routes check must be in the catalogue
	for _, need := range []string{
		"users:create", "users:read", "users:update", "users:delete",
		"roles:create", "roles:read", "roles:update", "roles:delete", "permissions:read", "audit:read",
		"access:read", "access:manage",
	} {
		if !seen[need] {
			t.Errorf("catalogue is missing %s", need)
		}
	}
}
