package project

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// projectResource is the M0 resource type of a project; project role templates are named project_<key>.
const (
	projectResource = "project"
	templatePrefix  = "project_"
)

// MemberInput adds or changes a member. Exactly one of Role (built-in key) and RoleID (an M0 project role) is set.
type MemberInput struct {
	UserID uuid.UUID  `json:"user_id"`
	Role   string     `json:"role"`
	RoleID *uuid.UUID `json:"role_id"`
}

// MemberView is a member with the M0 role resolved.
type MemberView struct {
	Member
	RoleID      uuid.UUID `gorm:"-" json:"role_id"`
	RoleKey     string    `gorm:"-" json:"role_key,omitempty"`
	RoleName    string    `gorm:"-" json:"role_name"`
	DisplayName string    `gorm:"-" json:"display_name,omitempty"`
}

// roleLabel is the short role name shown to users: the built-in key, or the custom role name.
func roleLabel(r identityport.ResourceRole) (key, label string) {
	if len(r.TemplateKey) > len(templatePrefix) && r.TemplateKey[:len(templatePrefix)] == templatePrefix {
		k := r.TemplateKey[len(templatePrefix):]
		return k, k
	}
	return "", r.RoleName
}

// Members lists the active members of a project.
func (s *Service) Members(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]MemberView, error) {
	if _, err := s.Access(ctx, nil, a, projectID); err != nil {
		return nil, err
	}
	var members []Member
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ? AND active", projectID, a.TenantID).
		Order("added_at, user_id").Limit(1000).Find(&members).Error
	if err != nil {
		return nil, err
	}
	roles, err := s.p.Resources.RolesOn(ctx, s.db, a.TenantID, projectResource, projectID)
	if err != nil {
		return nil, err
	}
	byUser := map[uuid.UUID]identityport.ResourceRole{}
	for _, r := range roles {
		byUser[r.UserID] = r
	}
	rows := make([]MemberView, len(members))
	for i, m := range members {
		r := byUser[m.UserID]
		rows[i] = MemberView{Member: m, RoleID: r.RoleID}
		rows[i].RoleKey, rows[i].RoleName = roleLabel(r)
	}
	return rows, s.addNames(ctx, a.TenantID, rows)
}

func (in MemberInput) validate() error {
	if (in.Role == "") == (in.RoleID == nil) {
		return kernel.Invalid("role", "give either role or role_id")
	}
	if in.Role != "" && api.BuiltinPerms(in.Role) == nil {
		return kernel.Invalid("role", "must be owner, manager, member or viewer")
	}
	return nil
}

// roleID resolves the M0 role the input names.
func (s *Service) roleID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, in MemberInput) (uuid.UUID, error) {
	if in.RoleID != nil {
		return *in.RoleID, nil
	}
	return s.p.Resources.RoleByTemplate(ctx, db, tenantID, templatePrefix+in.Role)
}

// assign gives the user the role on the project in M0, inside tx, translating M0 refusals into request errors.
func (s *Service) assign(ctx context.Context, tx *gorm.DB, a api.Actor, userID, roleID, projectID uuid.UUID, source string) error {
	err := s.p.Resources.AssignRole(ctx, tx, a.TenantID, a.UserID, userID, roleID, projectResource, projectID, source)
	if errors.Is(err, identityport.ErrRoleNotFound) || errors.Is(err, identityport.ErrRoleNotApplicable) {
		return kernel.Invalid("role_id", "must be a project role of this tenant")
	}
	return err
}

// isOwnerRole reports whether the user currently holds the owner role on the project.
func (s *Service) isOwnerRole(ctx context.Context, db *gorm.DB, tenantID, userID, projectID uuid.UUID) (bool, error) {
	roles, err := s.p.Resources.RolesFor(ctx, db, tenantID, userID, projectResource, []uuid.UUID{projectID})
	return roles[projectID].TemplateKey == templatePrefix+api.RoleOwner, err
}

// AddMember adds a user to a project (PR-04).
func (s *Service) AddMember(ctx context.Context, a api.Actor, projectID uuid.UUID, in MemberInput) (*MemberView, error) {
	acc, err := s.Access(ctx, nil, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ManageMembers); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	if in.Role == api.RoleOwner && !ownerLevel(acc, a) {
		return nil, kernel.Forbidden("owner")
	}
	if u, uerr := s.p.Users.GetByID(ctx, s.db, a.TenantID, in.UserID); uerr != nil || u == nil || !u.Active() {
		return nil, kernel.Invalid("user_id", "must be an active user of the tenant")
	}
	return s.upsertMember(ctx, a, projectID, in)
}

func ownerLevel(acc api.Access, a api.Actor) bool { return a.ManageAll || acc.Role == api.RoleOwner }

func (s *Service) upsertMember(ctx context.Context, a api.Actor, projectID uuid.UUID, in MemberInput) (*MemberView, error) {
	m := Member{ProjectID: projectID, UserID: in.UserID, TenantID: a.TenantID, Active: true, AddedBy: a.UserID, AddedAt: s.now().UTC()}
	out := MemberView{Member: m}
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
		rid, err := s.roleID(ctx, tx, a.TenantID, in)
		if err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, in.UserID).Error; err != nil {
			return err
		}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		if err := s.assign(ctx, tx, a, in.UserID, rid, projectID, "manual"); err != nil {
			return err
		}
		out = MemberView{Member: m, RoleID: rid, RoleKey: in.Role}
		s.audit(ctx, tx, a, "project.member.added", ref{"project", projectID, map[string]any{"user_id": in.UserID}})
		return kernel.Emit(ctx, tx, a.TenantID, "project.member.added", map[string]any{"project_id": projectID, "user_id": in.UserID})
	})
	return &out, err
}

// ChangeMember changes a member role; the last owner cannot be demoted (PR-04).
func (s *Service) ChangeMember(ctx context.Context, a api.Actor, projectID, userID uuid.UUID, in MemberInput) (*MemberView, error) {
	acc, err := s.Access(ctx, nil, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ManageMembers); err != nil {
		return nil, err
	}
	in.UserID = userID
	if err := in.validate(); err != nil {
		return nil, err
	}
	var out MemberView
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		if err := tx.Where("project_id = ? AND user_id = ? AND active", projectID, userID).Take(&out.Member).Error; err != nil {
			return mapMissing(err, "member")
		}
		wasOwner, err := s.isOwnerRole(ctx, tx, a.TenantID, userID, projectID)
		if err != nil {
			return err
		}
		if (in.Role == api.RoleOwner || wasOwner) && !ownerLevel(acc, a) {
			return kernel.Forbidden("owner")
		}
		rid, err := s.roleID(ctx, tx, a.TenantID, in)
		if err != nil {
			return err
		}
		out.Source, out.RoleID, out.RoleKey = "direct", rid, in.Role // an explicit role change pins a group member as a direct one
		if err := tx.Model(&Member{}).Where("project_id = ? AND user_id = ?", projectID, userID).Update("source", "direct").Error; err != nil {
			return err
		}
		if err := s.assign(ctx, tx, a, userID, rid, projectID, "manual"); err != nil {
			return err
		}
		if err := s.ensureOwner(ctx, tx, a.TenantID, projectID); err != nil {
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
		if err := s.p.Resources.RevokeRole(ctx, tx, a.TenantID, a.UserID, userID, projectResource, projectID); err != nil {
			return err
		}
		if err := s.ensureOwner(ctx, tx, a.TenantID, projectID); err != nil {
			return err
		}
		if err := dropWatches(tx, a.TenantID, projectID, userID); err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.member.removed", ref{"project", projectID, map[string]any{"user_id": userID}})
		return kernel.Emit(ctx, tx, a.TenantID, "project.member.removed", map[string]any{"project_id": projectID, "user_id": userID})
	})
}

// ensureOwner refuses a change that would leave the project without an active owner.
func (s *Service) ensureOwner(ctx context.Context, tx *gorm.DB, tenantID, projectID uuid.UUID) error {
	roles, err := s.p.Resources.RolesOn(ctx, tx, tenantID, projectResource, projectID)
	if err != nil {
		return err
	}
	var owners []uuid.UUID
	for _, r := range roles {
		if r.TemplateKey == templatePrefix+api.RoleOwner {
			owners = append(owners, r.UserID)
		}
	}
	n := int64(0)
	if len(owners) > 0 {
		if n, err = countRows(tx, "project_members", "project_id = ? AND active AND user_id IN ?", projectID, owners); err != nil {
			return err
		}
	}
	if n == 0 {
		return kernel.Conflict("LAST_OWNER", "a project needs at least one owner")
	}
	return nil
}

// addNames fills display names for the members; a failing lookup degrades to ids only rather than failing the list.
func (s *Service) addNames(ctx context.Context, tenantID uuid.UUID, rows []MemberView) error {
	if s.p.Names == nil || len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	names, err := s.p.Names.DisplayNames(ctx, tenantID, ids)
	if err != nil {
		return nil
	}
	for i := range rows {
		rows[i].DisplayName = names[rows[i].UserID]
	}
	return nil
}

// RoleOption is a project role that can be given to a member.
type RoleOption struct {
	RoleID uuid.UUID `json:"role_id"`
	Name   string    `json:"name"`
	Key    string    `json:"key,omitempty"` // owner, manager, member or viewer for the built-in roles
}

// AssignableRoles lists the M0 roles that can be given to members of the project (manage_members).
func (s *Service) AssignableRoles(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]RoleOption, error) {
	if err := s.require(ctx, a, projectID, api.ManageMembers); err != nil {
		return nil, err
	}
	roles, err := s.p.Resources.AvailableRoles(ctx, s.db, a.TenantID, projectResource)
	if err != nil {
		return nil, err
	}
	out := make([]RoleOption, 0, len(roles))
	for _, r := range roles {
		key, label := roleLabel(r)
		out = append(out, RoleOption{RoleID: r.RoleID, Name: label, Key: key})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
