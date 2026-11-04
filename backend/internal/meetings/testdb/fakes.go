package testdb

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// ErrFault is the error injected by the stand-ins.
var ErrFault = errors.New("injected fault")

// Resources is an in-memory stand-in for M0 role assignments on meeting series, with the four predefined meeting roles.
// The real M0 is covered by its own tests and by the end-to-end journeys in tests/integration.
type Resources struct {
	// Fail makes the named method ("PermissionsOn", "AssignRole", ...) return ErrFault (fault-injection tests).
	Fail   map[string]bool
	mu     sync.Mutex
	roles  map[uuid.UUID]fakeRole
	byKey  map[string]uuid.UUID
	assign map[[3]uuid.UUID]identityport.ResourceRole // tenant, resource, user
}

type fakeRole struct {
	name, template string
	perms          []string
}

// NewResources returns the stand-in.
func NewResources() *Resources {
	r := &Resources{Fail: map[string]bool{}, roles: map[uuid.UUID]fakeRole{}, byKey: map[string]uuid.UUID{}, assign: map[[3]uuid.UUID]identityport.ResourceRole{}}
	for _, k := range []string{api.RoleViewer, api.RoleAttendee, api.RoleNoteTaker, api.RoleOrganizer} {
		var perms []string
		for p := range api.TemplatePerms(k) {
			perms = append(perms, string(p))
		}
		id := uuid.New()
		r.roles[id], r.byKey[k] = fakeRole{name: k, template: k, perms: perms}, id
	}
	return r
}

func (r *Resources) view(id uuid.UUID, a identityport.ResourceRole) identityport.ResourceRole {
	role := r.roles[id]
	a.RoleID, a.RoleName, a.TemplateKey = id, role.name, role.template
	return a
}

// PermissionsOn implements identityport.ResourceAccess.
func (r *Resources) PermissionsOn(_ context.Context, _ *gorm.DB, ref identityport.ResourceRef, userID uuid.UUID) (identityport.ResourceRole, []string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["PermissionsOn"] {
		return identityport.ResourceRole{}, nil, ErrFault
	}
	a, ok := r.assign[[3]uuid.UUID{ref.TenantID, ref.ID, userID}]
	if !ok {
		return identityport.ResourceRole{}, nil, nil
	}
	return a, append([]string(nil), r.roles[a.RoleID].perms...), nil
}

// RolesFor implements identityport.ResourceAccess.
func (r *Resources) RolesFor(_ context.Context, _ *gorm.DB, ref identityport.ResourceRef, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]identityport.ResourceRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["RolesFor"] {
		return nil, ErrFault
	}
	out := map[uuid.UUID]identityport.ResourceRole{}
	for _, id := range ids {
		if a, ok := r.assign[[3]uuid.UUID{ref.TenantID, id, userID}]; ok {
			out[id] = a
		}
	}
	return out, nil
}

// AssignRole implements identityport.ResourceAccess.
func (r *Resources) AssignRole(_ context.Context, _ *gorm.DB, ref identityport.ResourceRef, in identityport.RoleAssignment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["AssignRole"] {
		return ErrFault
	}
	if ref.Type != access.AppliesMeetingSeries {
		return access.ErrResourceType
	}
	if _, ok := r.roles[in.RoleID]; !ok {
		return access.ErrRoleNotFound
	}
	r.assign[[3]uuid.UUID{ref.TenantID, ref.ID, in.UserID}] = r.view(in.RoleID, identityport.ResourceRole{UserID: in.UserID, Source: in.Source})
	return nil
}

// RevokeRole implements identityport.ResourceAccess.
func (r *Resources) RevokeRole(_ context.Context, _ *gorm.DB, ref identityport.ResourceRef, _, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["RevokeRole"] {
		return ErrFault
	}
	delete(r.assign, [3]uuid.UUID{ref.TenantID, ref.ID, userID})
	return nil
}

// RolesOn implements identityport.ResourceAccess.
func (r *Resources) RolesOn(_ context.Context, _ *gorm.DB, ref identityport.ResourceRef) ([]identityport.ResourceRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["RolesOn"] {
		return nil, ErrFault
	}
	var out []identityport.ResourceRole
	for k, a := range r.assign {
		if k[0] == ref.TenantID && k[1] == ref.ID {
			out = append(out, a)
		}
	}
	return out, nil
}

// AvailableRoles implements identityport.ResourceAccess.
func (r *Resources) AvailableRoles(context.Context, *gorm.DB, identityport.ResourceRef) ([]identityport.ResourceRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["AvailableRoles"] {
		return nil, ErrFault
	}
	var out []identityport.ResourceRole
	for id := range r.roles {
		out = append(out, r.view(id, identityport.ResourceRole{}))
	}
	return out, nil
}

// RoleByTemplate implements identityport.ResourceAccess.
func (r *Resources) RoleByTemplate(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string) (uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["RoleByTemplate"] {
		return uuid.Nil, ErrFault
	}
	if id, ok := r.byKey[key]; ok {
		return id, nil
	}
	return uuid.Nil, fmt.Errorf("predefined role %q is not provisioned", key)
}

// CountWithTemplate implements identityport.ResourceAccess.
func (r *Resources) CountWithTemplate(_ context.Context, _ *gorm.DB, ref identityport.ResourceRef, key string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail["CountWithTemplate"] {
		return 0, ErrFault
	}
	var n int64
	for k, a := range r.assign {
		if k[0] == ref.TenantID && k[1] == ref.ID && a.TemplateKey == key {
			n++
		}
	}
	return n, nil
}

// Users is a stand-in user directory: every id is an active user unless marked inactive.
type Users struct {
	Fail     bool // GetByID returns ErrFault
	mu       sync.Mutex
	Inactive map[uuid.UUID]bool
}

// GetByID implements identityport.Users.
func (u *Users) GetByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*identityport.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.Fail {
		return nil, ErrFault
	}
	status := "active"
	if u.Inactive[id] {
		status = "inactive"
	}
	return &identityport.User{ID: id, TenantID: tenantID, Status: status}, nil
}

// DeactivateUser implements identityport.Users.
func (u *Users) DeactivateUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

// ActivateUser implements identityport.Users.
func (u *Users) ActivateUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return nil }

// InviteUser implements identityport.Users.
func (u *Users) InviteUser(context.Context, *gorm.DB, uuid.UUID, identityport.Invite, uuid.UUID) (*identityport.User, error) {
	return nil, nil
}

// Audit records audit actions in memory.
type Audit struct {
	Fail    bool // every Log returns ErrFault
	mu      sync.Mutex
	Actions []string
}

// Log implements identityport.Auditor.
func (a *Audit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, _ interface{}, _, _ string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Fail {
		return ErrFault
	}
	a.Actions = append(a.Actions, action)
	return nil
}
