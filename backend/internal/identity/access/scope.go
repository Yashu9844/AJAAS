// Package access is the pure access model of M0 (docs/ARCHITECTURE.md §17): scopes, grants, role inheritance and
// the resolution of a user's effective access. It has no I/O; services feed it rows and cache its result.
package access

import "errors"

// Scope is how far a granted permission reaches.
type Scope string

// Scopes in increasing reach. The order is total so escalation checks can compare any two scopes; department is
// ranked above all_reports by convention (docs/ARCHITECTURE.md §17.1).
const (
	ScopeNone          Scope = ""
	ScopeOwn           Scope = "own"
	ScopeDirectReports Scope = "direct_reports"
	ScopeAllReports    Scope = "all_reports"
	ScopeDepartment    Scope = "department"
	ScopeTenant        Scope = "tenant"
)

// ErrInvalidScope rejects unknown scope names.
var ErrInvalidScope = errors.New("scope must be one of own, direct_reports, all_reports, department, tenant")

var ranks = map[Scope]int{ScopeOwn: 1, ScopeDirectReports: 2, ScopeAllReports: 3, ScopeDepartment: 4, ScopeTenant: 5}

// AllScopes lists the valid scopes in increasing reach.
var AllScopes = []Scope{ScopeOwn, ScopeDirectReports, ScopeAllReports, ScopeDepartment, ScopeTenant}

// Valid reports whether s is a known, non-empty scope.
func (s Scope) Valid() bool { return ranks[s] > 0 }

// ParseScope validates a scope; empty means tenant (the meaning of legacy grants without a scope).
func ParseScope(raw string) (Scope, error) {
	if raw == "" {
		return ScopeTenant, nil
	}
	s := Scope(raw)
	if !s.Valid() {
		return ScopeNone, ErrInvalidScope
	}
	return s, nil
}

// Covers reports whether holding scope held is enough for scope want.
func Covers(held, want Scope) bool {
	return held.Valid() && want.Valid() && ranks[held] >= ranks[want]
}

// Wider returns the scope with the larger reach.
func Wider(a, b Scope) Scope {
	if ranks[b] > ranks[a] {
		return b
	}
	return a
}

// Key formats a permission key "resource:action".
func Key(resource, action string) string { return resource + ":" + action }

// Grant is a permission key held at a scope.
type Grant struct {
	Permission string
	Scope      Scope
}
