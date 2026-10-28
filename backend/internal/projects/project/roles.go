package project

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// RoleInput creates or edits a custom project role (PR-05).
type RoleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// BuiltinRole describes one fixed role in the catalogue.
type BuiltinRole struct {
	Key         string   `json:"key"`
	Permissions []string `json:"permissions"`
}

// Catalogue lists the project permissions and the built-in roles.
type Catalogue struct {
	Permissions []string      `json:"permissions"`
	Roles       []BuiltinRole `json:"builtin_roles"`
}

// PermissionCatalogue returns what a custom role can be built from.
func PermissionCatalogue() Catalogue {
	c := Catalogue{}
	for _, p := range api.AllPerms {
		c.Permissions = append(c.Permissions, string(p))
	}
	for _, k := range api.BuiltinRoles() {
		c.Roles = append(c.Roles, BuiltinRole{Key: k, Permissions: api.SortedPerms(api.BuiltinPerms(k))})
	}
	return c
}

func (in RoleInput) validate() (kernel.StringList, string, error) {
	name, err := text("name", in.Name, 1, 60)
	if err != nil {
		return nil, "", err
	}
	if api.BuiltinPerms(strings.ToLower(name)) != nil {
		return nil, "", kernel.Invalid("name", "is reserved for a built-in role")
	}
	if tooLong(in.Description, 300) {
		return nil, "", kernel.Invalid("description", "is too long")
	}
	seen, perms := map[string]bool{}, kernel.StringList{}
	for _, p := range in.Permissions {
		if !api.ValidPerm(p) {
			return nil, "", kernel.Invalid("permissions", "unknown permission "+p)
		}
		if !seen[p] {
			seen[p] = true
			perms = append(perms, p)
		}
	}
	if !seen[string(api.ProjectRead)] || !seen[string(api.TaskRead)] {
		return nil, "", kernel.Invalid("permissions", "a role must include project.read and task.read")
	}
	return perms, name, nil
}

func (s *Service) requireRoles(a api.Actor) error {
	if a.ManageRoles || a.ManageAll {
		return nil
	}
	return kernel.Forbidden("project_roles:manage")
}

// Roles lists the tenant custom roles.
func (s *Service) Roles(ctx context.Context, a api.Actor) ([]CustomRole, error) {
	var rows []CustomRole
	err := s.db.WithContext(ctx).Where("tenant_id = ?", a.TenantID).Order("lower(name)").Find(&rows).Error
	return rows, err
}

// CreateRole defines a custom role.
func (s *Service) CreateRole(ctx context.Context, a api.Actor, in RoleInput) (*CustomRole, error) {
	if err := s.requireRoles(a); err != nil {
		return nil, err
	}
	perms, name, err := in.validate()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	r := CustomRole{ID: uuid.New(), TenantID: a.TenantID, Name: name, Description: in.Description, Permissions: perms, CreatedBy: a.UserID, CreatedAt: now, UpdatedAt: now}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.role.created", "project_role", r.ID, nil)
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("ROLE_NAME_TAKEN", "a role with this name already exists")
	}
	return &r, err
}

// UpdateRole edits a custom role; members pick up the new permissions at once.
func (s *Service) UpdateRole(ctx context.Context, a api.Actor, id uuid.UUID, in RoleInput) (*CustomRole, error) {
	if err := s.requireRoles(a); err != nil {
		return nil, err
	}
	perms, name, err := in.validate()
	if err != nil {
		return nil, err
	}
	var r CustomRole
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&r).Error; err != nil {
			return mapMissing(err, "role")
		}
		r.Name, r.Description, r.Permissions, r.UpdatedAt = name, in.Description, perms, s.now().UTC()
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.role.updated", "project_role", id, nil)
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("ROLE_NAME_TAKEN", "a role with this name already exists")
	}
	return &r, err
}

// DeleteRole removes a custom role that no member uses.
func (s *Service) DeleteRole(ctx context.Context, a api.Actor, id uuid.UUID) error {
	if err := s.requireRoles(a); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		var used int64
		tx.Table("project_members").Where("custom_role_id = ?", id).Count(&used)
		if used > 0 {
			return kernel.Conflict("ROLE_IN_USE", "members still hold this role")
		}
		res := tx.Exec(`DELETE FROM project_roles WHERE id = ? AND tenant_id = ?`, id, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("role")
		}
		s.audit(ctx, tx, a, "project.role.deleted", "project_role", id, nil)
		return nil
	})
}
