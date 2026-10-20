package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// fakeAccess is an in-memory AccessRepository plus the role/assignment/permission rows the access services read.
type fakeAccess struct {
	tenant      uuid.UUID
	version     int64
	roles       map[uuid.UUID]*models.Role
	rolePerms   []models.RolePermission
	userRoles   []models.UserRole
	grants      []models.UserPermissionGrant
	blocks      []models.UserPermissionBlock
	perms       map[uuid.UUID]*models.Permission
	users       map[uuid.UUID]bool
	tenants     []uuid.UUID
	err         error // returned by every call when set
	scopeWrites int
}

func newFakeAccess() *fakeAccess {
	return &fakeAccess{tenant: uuid.New(), roles: map[uuid.UUID]*models.Role{}, perms: map[uuid.UUID]*models.Permission{}, users: map[uuid.UUID]bool{}}
}

func (f *fakeAccess) bump() { f.version++ }

func (f *fakeAccess) perm(resource, action string) *models.Permission {
	for _, p := range f.perms {
		if p.Resource == resource && p.Action == action {
			return p
		}
	}
	p := &models.Permission{ID: uuid.New(), Resource: resource, Action: action}
	f.perms[p.ID] = p
	return p
}

func (f *fakeAccess) role(name string, parent *uuid.UUID, grants map[string]string) *models.Role {
	r := &models.Role{TenantID: f.tenant, Name: name, ParentRoleID: parent}
	r.ID = uuid.New()
	f.roles[r.ID] = r
	for key, scope := range grants {
		p := f.permByKey(key)
		f.rolePerms = append(f.rolePerms, models.RolePermission{ID: uuid.New(), RoleID: r.ID, PermissionID: p.ID, TenantID: f.tenant, Scope: scope, Permission: p})
	}
	return r
}

func (f *fakeAccess) permByKey(key string) *models.Permission {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return f.perm(key[:i], key[i+1:])
		}
	}
	return f.perm(key, "")
}

func (f *fakeAccess) assign(user uuid.UUID, r *models.Role, source string) {
	f.users[user] = true
	f.userRoles = append(f.userRoles, models.UserRole{ID: uuid.New(), UserID: user, RoleID: r.ID, TenantID: f.tenant, Source: source, Role: r})
}

// ---- repositories.AccessRepository

func (f *fakeAccess) AccessVersion(context.Context, *gorm.DB, uuid.UUID) (int64, error) {
	return f.version, f.err
}
func (f *fakeAccess) TenantRoles(context.Context, *gorm.DB, uuid.UUID) ([]models.Role, error) {
	out := make([]models.Role, 0, len(f.roles))
	for _, r := range f.roles {
		out = append(out, *r)
	}
	return out, f.err
}
func (f *fakeAccess) RoleByTemplateKey(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string) (*models.Role, error) {
	for _, r := range f.roles {
		if r.TemplateKey != nil && *r.TemplateKey == key {
			return r, f.err
		}
	}
	return nil, f.err
}
func (f *fakeAccess) SetRolePermissionScope(_ context.Context, _ *gorm.DB, _, roleID, permID uuid.UUID, scope string) error {
	for i := range f.rolePerms {
		if f.rolePerms[i].RoleID == roleID && f.rolePerms[i].PermissionID == permID {
			f.rolePerms[i].Scope = scope
		}
	}
	f.scopeWrites++
	f.bump()
	return f.err
}
func (f *fakeAccess) UserGrants(_ context.Context, _ *gorm.DB, _, user uuid.UUID) ([]models.UserPermissionGrant, error) {
	var out []models.UserPermissionGrant
	for _, g := range f.grants {
		if g.UserID == user {
			g.Permission = f.perms[g.PermissionID]
			out = append(out, g)
		}
	}
	return out, f.err
}
func (f *fakeAccess) UpsertUserGrant(_ context.Context, _ *gorm.DB, g *models.UserPermissionGrant) error {
	if f.err != nil {
		return f.err
	}
	f.DeleteUserGrant(context.Background(), nil, g.TenantID, g.UserID, g.PermissionID)
	f.grants = append(f.grants, *g)
	f.bump()
	return nil
}
func (f *fakeAccess) DeleteUserGrant(_ context.Context, _ *gorm.DB, _, user, perm uuid.UUID) (bool, error) {
	for i, g := range f.grants {
		if g.UserID == user && g.PermissionID == perm {
			f.grants = append(f.grants[:i], f.grants[i+1:]...)
			f.bump()
			return true, f.err
		}
	}
	return false, f.err
}
func (f *fakeAccess) UserBlocks(_ context.Context, _ *gorm.DB, _, user uuid.UUID) ([]models.UserPermissionBlock, error) {
	var out []models.UserPermissionBlock
	for _, b := range f.blocks {
		if b.UserID == user {
			b.Permission = f.perms[b.PermissionID]
			out = append(out, b)
		}
	}
	return out, f.err
}
func (f *fakeAccess) UpsertUserBlock(_ context.Context, _ *gorm.DB, b *models.UserPermissionBlock) error {
	if f.err != nil {
		return f.err
	}
	f.blocks = append(f.blocks, *b)
	f.bump()
	return nil
}
func (f *fakeAccess) DeleteUserBlock(_ context.Context, _ *gorm.DB, _, user, perm uuid.UUID) (bool, error) {
	for i, b := range f.blocks {
		if b.UserID == user && b.PermissionID == perm {
			f.blocks = append(f.blocks[:i], f.blocks[i+1:]...)
			return true, f.err
		}
	}
	return false, f.err
}
func (f *fakeAccess) AutomaticAssignments(context.Context, *gorm.DB, uuid.UUID) ([]models.UserRole, error) {
	var out []models.UserRole
	for _, ur := range f.userRoles {
		if ur.Source == string(access.SourceAutomatic) {
			out = append(out, ur)
		}
	}
	return out, f.err
}
func (f *fakeAccess) UpsertPermission(_ context.Context, _ *gorm.DB, p *models.Permission) error {
	if f.err != nil {
		return f.err
	}
	existing := f.perm(p.Resource, p.Action)
	existing.Module, existing.IsSensitive, existing.Description = p.Module, p.IsSensitive, p.Description
	return nil
}
func (f *fakeAccess) AllPermissions(context.Context, *gorm.DB) ([]models.Permission, error) {
	out := make([]models.Permission, 0, len(f.perms))
	for _, p := range f.perms {
		out = append(out, *p)
	}
	return out, f.err
}
func (f *fakeAccess) TenantIDs(context.Context, *gorm.DB, bool) ([]uuid.UUID, error) {
	return f.tenants, f.err
}

// ---- role / assignment / permission repositories backed by the same state

func (f *fakeAccess) roleRepo() *MockRoleRepository {
	return &MockRoleRepository{
		CreateFunc: func(_ context.Context, _ *gorm.DB, r *models.Role) error {
			if r.ID == uuid.Nil {
				r.ID = uuid.New()
			}
			cp := *r
			f.roles[r.ID] = &cp
			f.bump()
			return f.err
		},
		FindByIDFunc: func(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*models.Role, error) {
			if r, ok := f.roles[id]; ok {
				cp := *r
				return &cp, f.err
			}
			return nil, f.err
		},
		FindByNameFunc: func(_ context.Context, _ *gorm.DB, _ uuid.UUID, name string) (*models.Role, error) {
			for _, r := range f.roles {
				if r.Name == name {
					return r, f.err
				}
			}
			return nil, f.err
		},
		UpdateFunc: func(_ context.Context, _ *gorm.DB, r *models.Role) error {
			cp := *r
			f.roles[r.ID] = &cp
			return f.err
		},
		DeleteFunc: func(_ context.Context, _ *gorm.DB, _, id uuid.UUID) error { delete(f.roles, id); return f.err },
	}
}

func (f *fakeAccess) userRoleRepo() *MockUserRoleRepository {
	return &MockUserRoleRepository{
		FindByUserIDFunc: func(_ context.Context, _ *gorm.DB, _, user uuid.UUID) ([]models.UserRole, error) {
			var out []models.UserRole
			for _, ur := range f.userRoles {
				if ur.UserID == user {
					ur.Role = f.roles[ur.RoleID]
					out = append(out, ur)
				}
			}
			return out, f.err
		},
		CreateFunc: func(_ context.Context, _ *gorm.DB, ur *models.UserRole) error {
			f.userRoles = append(f.userRoles, *ur)
			f.bump()
			return f.err
		},
		DeleteFunc: func(_ context.Context, _ *gorm.DB, _, user, role uuid.UUID) error {
			for i, ur := range f.userRoles {
				if ur.UserID == user && ur.RoleID == role {
					f.userRoles = append(f.userRoles[:i], f.userRoles[i+1:]...)
					break
				}
			}
			f.bump()
			return f.err
		},
		CountByRoleIDFunc: func(_ context.Context, _ *gorm.DB, _, role uuid.UUID) (int64, error) {
			var n int64
			for _, ur := range f.userRoles {
				if ur.RoleID == role {
					n++
				}
			}
			return n, f.err
		},
	}
}

func (f *fakeAccess) rolePermRepo() *MockRolePermissionRepository {
	return &MockRolePermissionRepository{
		FindByRoleIDFunc: func(_ context.Context, _ *gorm.DB, _, role uuid.UUID) ([]models.RolePermission, error) {
			var out []models.RolePermission
			for _, rp := range f.rolePerms {
				if rp.RoleID == role {
					rp.Permission = f.perms[rp.PermissionID]
					out = append(out, rp)
				}
			}
			return out, f.err
		},
		CreateFunc: func(_ context.Context, _ *gorm.DB, rp *models.RolePermission) error {
			if rp.Scope == "" {
				rp.Scope = "tenant"
			}
			f.rolePerms = append(f.rolePerms, *rp)
			f.bump()
			return f.err
		},
		DeleteFunc: func(_ context.Context, _ *gorm.DB, _, role, perm uuid.UUID) error {
			for i, rp := range f.rolePerms {
				if rp.RoleID == role && rp.PermissionID == perm {
					f.rolePerms = append(f.rolePerms[:i], f.rolePerms[i+1:]...)
					break
				}
			}
			return f.err
		},
		DeleteByRoleIDFunc: func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return f.err },
	}
}

func (f *fakeAccess) permRepo() *MockPermissionRepository {
	return &MockPermissionRepository{
		FindByIDFunc: func(_ context.Context, _ *gorm.DB, id uuid.UUID) (*models.Permission, error) {
			return f.perms[id], f.err
		},
		FindByIDsFunc: func(_ context.Context, _ *gorm.DB, ids []uuid.UUID) ([]models.Permission, error) {
			var out []models.Permission
			for _, id := range ids {
				if p, ok := f.perms[id]; ok {
					out = append(out, *p)
				}
			}
			return out, f.err
		},
		FindAllFunc: func(context.Context, *gorm.DB, int, int) ([]models.Permission, int64, error) {
			ps, err := f.AllPermissions(context.Background(), nil)
			return ps, int64(len(ps)), err
		},
	}
}

func (f *fakeAccess) userRepo() *MockUserRepository {
	return &MockUserRepository{FindByIDFunc: func(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*models.User, error) {
		if !f.users[id] {
			return nil, f.err
		}
		u := &models.User{Status: "active"}
		u.ID = id
		return u, f.err
	}}
}

func (f *fakeAccess) resolver(cache AccessCache, now time.Time) AccessResolver {
	return NewAccessResolver(AccessResolverDeps{Access: f, UserRoles: f.userRoleRepo(), RolePerms: f.rolePermRepo(), Cache: cache,
		Now: func() time.Time { return now }})
}

// passthrough runs the transaction body without a database.
func passthrough(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }

// memCache is an in-memory AccessCache.
type memCache struct {
	m    map[string]string
	ttls []time.Duration
	err  error
}

func (c *memCache) Get(_ context.Context, k string) (string, bool, error) {
	v, ok := c.m[k]
	return v, ok, c.err
}
func (c *memCache) Set(_ context.Context, k, v string, ttl time.Duration) error {
	c.m[k] = v
	c.ttls = append(c.ttls, ttl)
	return c.err
}
