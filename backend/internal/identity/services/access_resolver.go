package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"gorm.io/gorm"
)

// accessCacheTTL bounds how long a resolved access set is reused; the tenant access version invalidates it sooner.
const accessCacheTTL = 5 * time.Minute

// AccessCache stores resolved access sets; implementations must treat errors as misses.
type AccessCache interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// AccessResolver computes a user's effective access (docs/ARCHITECTURE.md §17).
type AccessResolver interface {
	// Effective resolves through db (pass the transaction when deciding inside one; it bypasses the cache).
	Effective(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Effective, error)
	// ScopeFor returns the widest scope held for resource:action, using the cache.
	ScopeFor(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (access.Scope, error)
	// RoleGrants resolves what holding roleIDs (with their ancestors) grants, for escalation checks.
	RoleGrants(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) (access.Effective, error)
}

// AccessResolverDeps are the resolver's collaborators.
type AccessResolverDeps struct {
	DB        *gorm.DB
	Access    repositories.AccessRepository // nil: flat roles only (no inheritance, user grants or blocks)
	UserRoles repositories.UserRoleRepository
	RolePerms repositories.RolePermissionRepository
	Cache     AccessCache // optional
	Now       func() time.Time
}

type accessResolver struct{ d AccessResolverDeps }

// NewAccessResolver builds an AccessResolver.
func NewAccessResolver(d AccessResolverDeps) AccessResolver {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &accessResolver{d: d}
}

func (r *accessResolver) ScopeFor(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (access.Scope, error) {
	eff, err := r.cached(ctx, tenantID, userID)
	if err != nil {
		return access.ScopeNone, err
	}
	return eff.ScopeFor(access.Key(resource, action)), nil
}

func (r *accessResolver) cached(ctx context.Context, tenantID, userID uuid.UUID) (access.Effective, error) {
	if r.d.Cache == nil || r.d.Access == nil {
		eff, _, err := r.resolve(ctx, r.d.DB, tenantID, userID)
		return eff, err
	}
	version, err := r.d.Access.AccessVersion(ctx, r.d.DB, tenantID)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("access:%s:%d:%s", tenantID, version, userID)
	if raw, ok, gerr := r.d.Cache.Get(ctx, key); gerr == nil && ok {
		var eff access.Effective
		if json.Unmarshal([]byte(raw), &eff) == nil {
			return eff, nil
		}
	}
	eff, ttl, err := r.resolve(ctx, r.d.DB, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if raw, merr := json.Marshal(eff); merr == nil {
		_ = r.d.Cache.Set(ctx, key, string(raw), ttl)
	}
	return eff, nil
}

func (r *accessResolver) Effective(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Effective, error) {
	eff, _, err := r.resolve(ctx, db, tenantID, userID)
	return eff, err
}

// resolve loads the rows, resolves them, and returns how long the result stays valid (earliest grant expiry).
func (r *accessResolver) resolve(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Effective, time.Duration, error) {
	in, err := r.input(ctx, db, tenantID, userID)
	if err != nil {
		return nil, 0, err
	}
	eff, err := access.Resolve(in)
	if err != nil {
		return nil, 0, err
	}
	ttl := accessCacheTTL
	for _, g := range in.UserGrants {
		if g.ExpiresAt != nil && g.ExpiresAt.After(in.Now) && g.ExpiresAt.Sub(in.Now) < ttl {
			ttl = g.ExpiresAt.Sub(in.Now)
		}
	}
	return eff, ttl, nil
}

func (r *accessResolver) input(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Input, error) {
	in := access.Input{Now: r.d.Now()}
	urs, err := r.d.UserRoles.FindByUserID(ctx, db, tenantID, userID)
	if err != nil {
		return in, err
	}
	for _, ur := range urs {
		in.Assigned = append(in.Assigned, access.Assignment{RoleID: ur.RoleID, Source: access.AssignmentSource(ur.Source)})
	}
	if in.Roles, err = r.roleNodes(ctx, db, tenantID, roleIDsOf(urs)); err != nil {
		return in, err
	}
	if r.d.Access == nil {
		return in, nil
	}
	grants, err := r.d.Access.UserGrants(ctx, db, tenantID, userID)
	if err != nil {
		return in, err
	}
	for _, g := range grants {
		if g.Permission != nil {
			in.UserGrants = append(in.UserGrants, access.UserGrant{Grant: grantOf(g.Permission, g.Scope), ExpiresAt: g.ExpiresAt})
		}
	}
	blocks, err := r.d.Access.UserBlocks(ctx, db, tenantID, userID)
	if err != nil {
		return in, err
	}
	for _, b := range blocks {
		if b.Permission != nil {
			in.Blocks = append(in.Blocks, access.Key(b.Permission.Resource, b.Permission.Action))
		}
	}
	return in, nil
}

func roleIDsOf(urs []models.UserRole) []uuid.UUID {
	ids := make([]uuid.UUID, len(urs))
	for i, ur := range urs {
		ids[i] = ur.RoleID
	}
	return ids
}

func (r *accessResolver) RoleGrants(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) (access.Effective, error) {
	nodes, err := r.roleNodes(ctx, db, tenantID, roleIDs)
	if err != nil {
		return nil, err
	}
	in := access.Input{Now: r.d.Now(), Roles: nodes}
	for _, id := range roleIDs {
		in.Assigned = append(in.Assigned, access.Assignment{RoleID: id, Source: access.SourceManual})
	}
	return access.Resolve(in)
}

// roleNodes loads every live role of the tenant with its own grants (two queries). Without an access repository
// only the given roles are known (no inheritance).
func (r *accessResolver) roleNodes(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, fallback []uuid.UUID) ([]access.RoleNode, error) {
	var roles []models.Role
	if r.d.Access == nil {
		for _, id := range fallback {
			role := models.Role{}
			role.ID = id
			roles = append(roles, role)
		}
	} else {
		var err error
		if roles, err = r.d.Access.TenantRoles(ctx, db, tenantID); err != nil {
			return nil, err
		}
	}
	ids := make([]uuid.UUID, len(roles))
	for i, role := range roles {
		ids[i] = role.ID
	}
	rps, err := r.d.RolePerms.FindByRoleIDs(ctx, db, tenantID, ids)
	if err != nil {
		return nil, err
	}
	grants := map[uuid.UUID][]access.Grant{}
	for _, rp := range rps {
		if rp.Permission != nil {
			grants[rp.RoleID] = append(grants[rp.RoleID], grantOf(rp.Permission, rp.Scope))
		}
	}
	nodes := make([]access.RoleNode, len(roles))
	for i, role := range roles {
		nodes[i] = access.RoleNode{ID: role.ID, Name: role.Name, ParentID: role.ParentRoleID, Automatic: role.IsAutomatic, Grants: grants[role.ID]}
	}
	return nodes, nil
}

func grantOf(p *models.Permission, scope string) access.Grant {
	s, err := access.ParseScope(scope)
	if err != nil {
		s = access.ScopeNone
	}
	return access.Grant{Permission: access.Key(p.Resource, p.Action), Scope: s}
}
