package project

import (
	"context"
	"errors"
	"strings"

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
	held, role, member, err := s.heldPerms(ctx, db, a, p)
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
func (s *Service) heldPerms(ctx context.Context, db *gorm.DB, a api.Actor, p Project) (map[api.Perm]bool, string, bool, error) {
	held := map[api.Perm]bool{}
	role, member := "", false
	var m Member
	err := db.Where("project_id = ? AND user_id = ? AND active", p.ID, a.UserID).Take(&m).Error
	switch {
	case err == nil:
		member = true
		rr, keys, rerr := s.p.Resources.PermissionsOn(ctx, db, projectRef(a.TenantID, p.ID), m.UserID)
		if rerr != nil {
			return nil, "", false, rerr
		}
		_, role = roleLabel(rr)
		for _, k := range keys {
			if perm := strings.Replace(k, ":", ".", 1); api.ValidPerm(perm) {
				held[api.Perm(perm)] = true
			}
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

// ReadableProjectIDs lists the projects the caller can read (used by search and My work).
func (s *Service) ReadableProjectIDs(ctx context.Context, db *gorm.DB, a api.Actor) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.conn(db).WithContext(ctx).Raw(`SELECT p.id FROM projects p WHERE p.tenant_id = ? AND p.deleted_at IS NULL
		AND (p.visibility = 'tenant' OR ? OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = ? AND m.active))`,
		a.TenantID, a.ReadAll || a.ManageAll, a.UserID).Scan(&ids).Error
	return ids, err
}
