package project

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/shared/identityport"
)

// roleLabel is the short role name shown to users: the built-in key, or the custom role name.
func roleLabel(r identityport.ResourceRole) (key, label string) {
	if len(r.TemplateKey) > len(templatePrefix) && r.TemplateKey[:len(templatePrefix)] == templatePrefix {
		k := r.TemplateKey[len(templatePrefix):]
		return k, k
	}
	return "", r.RoleName
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
	roles, err := s.p.Resources.AvailableRoles(ctx, s.db, projectRef(a.TenantID, uuid.Nil))
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
