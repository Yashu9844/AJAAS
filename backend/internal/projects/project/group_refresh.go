package project

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefreshGroups reconciles the group-sourced members of one project with its rules. Direct members are never touched or
// overridden. The people lookups happen before the transaction so no external call is made while rows are locked.
func (s *Service) RefreshGroups(ctx context.Context, tenantID, projectID uuid.UUID) error {
	if s.p.People == nil {
		return nil
	}
	var rules []GroupRule
	if err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, tenantID).Find(&rules).Error; err != nil {
		return err
	}
	want, err := s.desiredMembers(ctx, rules, tenantID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, tenantID, projectID); err != nil {
			return err
		}
		return s.reconcile(tx, tenantID, projectID, want)
	})
}

func (s *Service) reconcile(tx *gorm.DB, tenantID, projectID uuid.UUID, want map[uuid.UUID]string) error {
	var have []Member
	if err := tx.Where("project_id = ? AND source = 'group'", projectID).Find(&have).Error; err != nil {
		return err
	}
	current := map[uuid.UUID]Member{}
	for _, m := range have {
		current[m.UserID] = m
	}
	for uid, role := range want {
		if err := s.upsertGroupMember(tx, groupScope{tenantID, projectID, current}, uid, role); err != nil {
			return err
		}
	}
	for uid := range current {
		if _, keep := want[uid]; !keep {
			if err := tx.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ? AND source = 'group'`, projectID, uid).Error; err != nil {
				return err
			}
			if err := dropWatches(tx, tenantID, projectID, uid); err != nil {
				return err
			}
		}
	}
	return tx.Exec(`UPDATE project_group_rules SET refreshed_at = ? WHERE project_id = ?`, s.now().UTC(), projectID).Error
}

// groupScope is the tenant, project and current group rows a reconcile works on.
type groupScope struct {
	tenantID, projectID uuid.UUID
	current             map[uuid.UUID]Member
}

// upsertGroupMember inserts or updates a group member; a user who is already a direct member keeps the direct row.
func (s *Service) upsertGroupMember(tx *gorm.DB, scope groupScope, uid uuid.UUID, role string) error {
	tenantID, projectID, current := scope.tenantID, scope.projectID, scope.current
	if cur, ok := current[uid]; ok {
		if cur.RoleKey != nil && *cur.RoleKey == role && cur.Active {
			return nil
		}
		return tx.Exec(`UPDATE project_members SET role_key = ?, active = TRUE WHERE project_id = ? AND user_id = ? AND source = 'group'`, role, projectID, uid).Error
	}
	return tx.Exec(`INSERT INTO project_members (project_id, user_id, tenant_id, role_key, active, added_by, added_at, source)
		VALUES (?, ?, ?, ?, TRUE, ?, ?, 'group') ON CONFLICT (project_id, user_id) DO NOTHING`,
		projectID, uid, tenantID, role, uid, s.now().UTC()).Error
}

// RefreshAll refreshes every project of the tenant that has rules; a periodic sweep keeps group membership aligned with HR.
func (s *Service) RefreshAll(ctx context.Context) error {
	var rows []struct{ TenantID, ProjectID uuid.UUID }
	if err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT r.tenant_id, r.project_id FROM project_group_rules r
		JOIN projects p ON p.id = r.project_id WHERE p.deleted_at IS NULL AND p.status = 'active'`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		if err := s.RefreshGroups(ctx, r.TenantID, r.ProjectID); err != nil {
			return err
		}
	}
	return nil
}

// RunGroupRefresh refreshes all group rules every interval until ctx ends; failures go to onErr and are retried.
func (s *Service) RunGroupRefresh(ctx context.Context, interval time.Duration, onErr func(error)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.RefreshAll(ctx); err != nil && onErr != nil {
				onErr(err)
			}
		}
	}
}
