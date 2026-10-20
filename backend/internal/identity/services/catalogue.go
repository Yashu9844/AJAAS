package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"gorm.io/gorm"
)

// Transactor runs fn in one database transaction.
type Transactor func(ctx context.Context, fn func(tx *gorm.DB) error) error

func dbTransactor(db *gorm.DB) Transactor {
	return func(ctx context.Context, fn func(tx *gorm.DB) error) error { return db.WithContext(ctx).Transaction(fn) }
}

// Catalogue registers the permission catalogue and keeps every tenant's predefined roles in sync
// (docs/ARCHITECTURE.md §17).
type Catalogue interface {
	// Register validates and upserts permission definitions and remembers their predefined-role defaults.
	Register(ctx context.Context, defs []access.PermissionDef) error
	// ProvisionTenant creates missing predefined roles and adds default grants to roles still template-managed.
	ProvisionTenant(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error
	// SyncAllTenants provisions every tenant (boot).
	SyncAllTenants(ctx context.Context) error
}

// CatalogueDeps are the catalogue's collaborators.
type CatalogueDeps struct {
	DB        *gorm.DB
	Transact  Transactor // defaults to DB.Transaction
	Roles     repositories.RoleRepository
	RolePerms repositories.RolePermissionRepository
	Access    repositories.AccessRepository
}

type catalogue struct {
	d    CatalogueDeps
	mu   sync.RWMutex
	defs map[string]access.PermissionDef
}

// NewCatalogue builds a Catalogue.
func NewCatalogue(d CatalogueDeps) Catalogue {
	if d.Transact == nil {
		d.Transact = dbTransactor(d.DB)
	}
	return &catalogue{d: d, defs: map[string]access.PermissionDef{}}
}

func (c *catalogue) Register(ctx context.Context, defs []access.PermissionDef) error {
	if err := access.ValidateDefs(defs); err != nil {
		return err
	}
	for _, def := range defs {
		desc := def.Description
		row := &models.Permission{Resource: def.Resource, Action: def.Action, Description: &desc, Module: def.Module, IsSensitive: def.Sensitive}
		if err := c.d.Access.UpsertPermission(ctx, c.d.DB, row); err != nil {
			return fmt.Errorf("register %s: %w", def.Key(), err)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, def := range defs {
		c.defs[def.Key()] = def
	}
	return nil
}

func (c *catalogue) registered() []access.PermissionDef {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]access.PermissionDef, 0, len(c.defs))
	for _, d := range c.defs {
		out = append(out, d)
	}
	return out
}

func (c *catalogue) SyncAllTenants(ctx context.Context) error {
	ids, err := c.d.Access.TenantIDs(ctx, c.d.DB, false)
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := c.d.Transact(ctx, func(tx *gorm.DB) error { return c.ProvisionTenant(ctx, tx, id) })
		if err != nil {
			return fmt.Errorf("provision predefined roles for tenant %s: %w", id, err)
		}
	}
	return nil
}

func (c *catalogue) ProvisionTenant(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error {
	roles, err := c.d.Access.TenantRoles(ctx, tx, tenantID)
	if err != nil {
		return err
	}
	byKey, names := map[string]*models.Role{}, map[string]bool{}
	for i := range roles {
		names[roles[i].Name] = true
		if roles[i].TemplateKey != nil {
			byKey[*roles[i].TemplateKey] = &roles[i]
		}
	}
	permIDs, err := c.permissionIDs(ctx, tx)
	if err != nil {
		return err
	}
	defs := c.registered()
	for _, tpl := range access.Templates {
		role := byKey[tpl.Key]
		if role == nil {
			if role, err = c.createTemplateRole(ctx, tx, tenantID, tpl, byKey, names); err != nil {
				return err
			}
			byKey[tpl.Key] = role
		}
		if role.TemplateManaged {
			if err := c.syncDefaults(ctx, tx, role, access.DefaultGrants(defs, tpl.Key), permIDs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *catalogue) createTemplateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, tpl access.Template, byKey map[string]*models.Role, names map[string]bool) (*models.Role, error) {
	name := tpl.Name
	if names[name] {
		name = tpl.Name + " (predefined)"
	}
	key, desc := tpl.Key, tpl.Description
	role := &models.Role{TenantID: tenantID, Name: name, Description: &desc, TemplateKey: &key, IsAutomatic: tpl.Automatic, TemplateManaged: true}
	if parent := byKey[tpl.Parent]; parent != nil {
		role.ParentRoleID = &parent.ID
	}
	if err := c.d.Roles.Create(ctx, tx, role); err != nil {
		return nil, fmt.Errorf("create predefined role %s: %w", tpl.Key, err)
	}
	names[name] = true
	return role, nil
}

// syncDefaults adds missing default grants and resets drifted scopes on a template-managed role.
func (c *catalogue) syncDefaults(ctx context.Context, tx *gorm.DB, role *models.Role, want []access.Grant, permIDs map[string]uuid.UUID) error {
	have, err := c.d.RolePerms.FindByRoleID(ctx, tx, role.TenantID, role.ID)
	if err != nil {
		return err
	}
	current := map[uuid.UUID]string{}
	for _, rp := range have {
		current[rp.PermissionID] = rp.Scope
	}
	for _, g := range want {
		pid, ok := permIDs[g.Permission]
		if !ok {
			continue
		}
		scope, held := current[pid]
		switch {
		case !held:
			err = c.d.RolePerms.Create(ctx, tx, &models.RolePermission{RoleID: role.ID, PermissionID: pid, TenantID: role.TenantID, Scope: string(g.Scope)})
		case scope != string(g.Scope):
			err = c.d.Access.SetRolePermissionScope(ctx, tx, role.TenantID, role.ID, pid, string(g.Scope))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *catalogue) permissionIDs(ctx context.Context, db *gorm.DB) (map[string]uuid.UUID, error) {
	perms, err := c.d.Access.AllPermissions(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make(map[string]uuid.UUID, len(perms))
	for _, p := range perms {
		out[access.Key(p.Resource, p.Action)] = p.ID
	}
	return out, nil
}
