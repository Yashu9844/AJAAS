package project

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Access resolves what the caller may do in the project (specification section 8): tenant-level permissions, then the
// effective member role, then tenant visibility for reads. A caller who cannot read the project gets 404 (PG4).
func (s *Service) Access(ctx context.Context, db *gorm.DB, a api.Actor, projectID uuid.UUID) (api.Access, error) {
	db = s.conn(db).WithContext(ctx)
	var p Project
	err := db.Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", projectID, a.TenantID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.Access{}, kernel.NotFound("project")
	}
	if err != nil {
		return api.Access{}, err
	}
	held, role, member, err := s.heldPerms(db, a, p)
	if err != nil {
		return api.Access{}, err
	}
	if !held[api.ProjectRead] {
		return api.Access{}, kernel.NotFound("project")
	}
	eff := held
	if p.Status == api.StatusArchived {
		eff = api.Archived(held)
	}
	info := api.ProjectInfo{ID: p.ID, Key: p.Key, Name: p.Name, Status: p.Status, Visibility: p.Visibility, Mode: p.Mode,
		TimeTracking: p.TimeTrackingEnabled, OwnerUserID: p.OwnerUserID}
	return api.Access{Project: info, Role: role, Member: member, Perms: eff, Held: held}, nil
}

// heldPerms merges the permission sources before the archive filter.
func (s *Service) heldPerms(db *gorm.DB, a api.Actor, p Project) (map[api.Perm]bool, string, bool, error) {
	held := map[api.Perm]bool{}
	role, member := "", false
	var m Member
	err := db.Where("project_id = ? AND user_id = ? AND active", p.ID, a.UserID).Take(&m).Error
	switch {
	case err == nil:
		member = true
		perms, name, rerr := s.memberPerms(db, m)
		if rerr != nil {
			return nil, "", false, rerr
		}
		role = name
		for k := range perms {
			held[k] = true
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, "", false, err
	}
	if a.ManageAll {
		for _, k := range api.AllPerms {
			held[k] = true
		}
		if role == "" {
			role = "tenant_admin"
		}
	}
	if a.ReadAll || p.Visibility == api.VisibilityTenant {
		held[api.ProjectRead], held[api.TaskRead] = true, true
	}
	return held, role, member, nil
}

// memberPerms returns the permission set and display name of a member role, built-in or custom.
func (s *Service) memberPerms(db *gorm.DB, m Member) (map[api.Perm]bool, string, error) {
	if m.RoleKey != nil {
		return api.BuiltinPerms(*m.RoleKey), *m.RoleKey, nil
	}
	var r CustomRole
	if m.CustomRoleID == nil {
		return nil, "", nil
	}
	if err := db.Where("id = ?", *m.CustomRoleID).Take(&r).Error; err != nil {
		return nil, "", err
	}
	perms := map[api.Perm]bool{}
	for _, k := range r.Permissions {
		perms[api.Perm(k)] = true
	}
	return perms, r.Name, nil
}

// ReadableProjectIDs lists the projects the caller can read (used by search and My work).
func (s *Service) ReadableProjectIDs(ctx context.Context, db *gorm.DB, a api.Actor) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.conn(db).WithContext(ctx).Raw(`SELECT p.id FROM projects p WHERE p.tenant_id = ? AND p.deleted_at IS NULL
		AND (p.visibility = 'tenant' OR ? OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = ? AND m.active))`,
		a.TenantID, a.ReadAll || a.ManageAll, a.UserID).Scan(&ids).Error
	return ids, err
}
