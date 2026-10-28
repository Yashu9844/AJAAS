package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// MemberInput adds or changes a member. Exactly one of Role (built-in key) and CustomRoleID is set.
type MemberInput struct {
	UserID       uuid.UUID  `json:"user_id"`
	Role         string     `json:"role"`
	CustomRoleID *uuid.UUID `json:"custom_role_id"`
}

// MemberView is a member with the role name resolved.
type MemberView struct {
	Member
	RoleName string `gorm:"column:role_name" json:"role_name"`
}

// Members lists the active members of a project.
func (s *Service) Members(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]MemberView, error) {
	if _, err := s.Access(ctx, nil, a, projectID); err != nil {
		return nil, err
	}
	var rows []MemberView
	err := s.db.WithContext(ctx).Raw(`SELECT m.*, COALESCE(m.role_key, cr.name) AS role_name FROM project_members m
		LEFT JOIN project_roles cr ON cr.id = m.custom_role_id WHERE m.project_id = ? AND m.tenant_id = ? AND m.active
		ORDER BY m.added_at, m.user_id LIMIT 1000`, projectID, a.TenantID).Scan(&rows).Error
	return rows, err
}

func (in MemberInput) validate(db *gorm.DB, tenantID uuid.UUID) error {
	if (in.Role == "") == (in.CustomRoleID == nil) {
		return kernel.Invalid("role", "give either role or custom_role_id")
	}
	if in.Role != "" && api.BuiltinPerms(in.Role) == nil {
		return kernel.Invalid("role", "must be owner, manager, member or viewer")
	}
	if in.CustomRoleID != nil {
		n, err := countRows(db, "project_roles", "id = ? AND tenant_id = ?", *in.CustomRoleID, tenantID)
		if err != nil {
			return err
		}
		if n == 0 {
			return kernel.Invalid("custom_role_id", "unknown role")
		}
	}
	return nil
}

func (in MemberInput) roleCols() (*string, *uuid.UUID) {
	if in.Role != "" {
		r := in.Role
		return &r, nil
	}
	return nil, in.CustomRoleID
}

// grantsOwner reports whether the change needs the caller to be an owner (or a tenant administrator).
func grantsOwner(in MemberInput, current *Member) bool {
	if in.Role == api.RoleOwner {
		return true
	}
	return current != nil && current.RoleKey != nil && *current.RoleKey == api.RoleOwner
}

// AddMember adds a user to a project (PR-04).
func (s *Service) AddMember(ctx context.Context, a api.Actor, projectID uuid.UUID, in MemberInput) (*Member, error) {
	acc, err := s.Access(ctx, nil, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ManageMembers); err != nil {
		return nil, err
	}
	if err := in.validate(s.db.WithContext(ctx), a.TenantID); err != nil {
		return nil, err
	}
	if grantsOwner(in, nil) && !ownerLevel(acc, a) {
		return nil, kernel.Forbidden("owner")
	}
	if u, uerr := s.p.Users.GetByID(ctx, s.db, a.TenantID, in.UserID); uerr != nil || u == nil || !u.Active() {
		return nil, kernel.Invalid("user_id", "must be an active user of the tenant")
	}
	return s.upsertMember(ctx, a, projectID, in)
}

func ownerLevel(acc api.Access, a api.Actor) bool { return a.ManageAll || acc.Role == api.RoleOwner }

func (s *Service) upsertMember(ctx context.Context, a api.Actor, projectID uuid.UUID, in MemberInput) (*Member, error) {
	rk, cr := in.roleCols()
	m := Member{ProjectID: projectID, UserID: in.UserID, TenantID: a.TenantID, RoleKey: rk, CustomRoleID: cr, Active: true, AddedBy: a.UserID, AddedAt: s.now().UTC()}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		have, err := countRows(tx, "project_members", "project_id = ? AND user_id = ? AND active", projectID, in.UserID)
		if err != nil {
			return err
		}
		if have > 0 {
			return kernel.Conflict("ALREADY_MEMBER", "the user is already a member")
		}
		if err := tx.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, in.UserID).Error; err != nil {
			return err
		}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.member.added", ref{"project", projectID, map[string]any{"user_id": in.UserID}})
		return kernel.Emit(ctx, tx, a.TenantID, "project.member.added", map[string]any{"project_id": projectID, "user_id": in.UserID})
	})
	return &m, err
}

// ChangeMember changes a member role; the last owner cannot be demoted (PR-04).
func (s *Service) ChangeMember(ctx context.Context, a api.Actor, projectID, userID uuid.UUID, in MemberInput) (*Member, error) {
	acc, err := s.Access(ctx, nil, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ManageMembers); err != nil {
		return nil, err
	}
	in.UserID = userID
	if err := in.validate(s.db.WithContext(ctx), a.TenantID); err != nil {
		return nil, err
	}
	var out Member
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		if err := tx.Where("project_id = ? AND user_id = ? AND active", projectID, userID).Take(&out).Error; err != nil {
			return mapMissing(err, "member")
		}
		out.Source = "direct" // an explicit role change pins a group member as a direct one
		if grantsOwner(in, &out) && !ownerLevel(acc, a) {
			return kernel.Forbidden("owner")
		}
		out.RoleKey, out.CustomRoleID = in.roleCols()
		if err := tx.Model(&Member{}).Where("project_id = ? AND user_id = ?", projectID, userID).
			Updates(map[string]any{"role_key": out.RoleKey, "custom_role_id": out.CustomRoleID, "source": "direct"}).Error; err != nil {
			return err
		}
		if err := ensureOwner(tx, projectID); err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.member.changed", ref{"project", projectID, map[string]any{"user_id": userID}})
		return kernel.Emit(ctx, tx, a.TenantID, "project.member.changed", map[string]any{"project_id": projectID, "user_id": userID})
	})
	return &out, err
}

// RemoveMember removes a member; anyone may remove themselves. The last owner stays (PR-04).
func (s *Service) RemoveMember(ctx context.Context, a api.Actor, projectID, userID uuid.UUID) error {
	acc, err := s.Access(ctx, nil, a, projectID)
	if err != nil {
		return err
	}
	if userID != a.UserID {
		if err := acc.Require(api.ManageMembers); err != nil {
			return err
		}
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		group, cerr := countRows(tx, "project_members", "project_id = ? AND user_id = ? AND source = 'group'", projectID, userID)
		if cerr != nil {
			return cerr
		}
		if group > 0 {
			return kernel.Conflict("GROUP_MEMBER", "this member comes from a group rule; remove the rule or change the role to pin them")
		}
		res := tx.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ? AND tenant_id = ?`, projectID, userID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("member")
		}
		if err := ensureOwner(tx, projectID); err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.member.removed", ref{"project", projectID, map[string]any{"user_id": userID}})
		return kernel.Emit(ctx, tx, a.TenantID, "project.member.removed", map[string]any{"project_id": projectID, "user_id": userID})
	})
}

// ensureOwner refuses a change that would leave the project without an owner.
func ensureOwner(tx *gorm.DB, projectID uuid.UUID) error {
	n, err := countRows(tx, "project_members", "project_id = ? AND role_key = 'owner' AND active", projectID)
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.Conflict("LAST_OWNER", "a project needs at least one owner")
	}
	return nil
}
