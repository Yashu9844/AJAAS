package project

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/ports"
	"gorm.io/gorm"
)

// Group rule kinds (PR-04).
const (
	KindDepartment = "department"
	KindTeam       = "team"
	KindReportsOf  = "reports_of"
)

// GroupRule gives every member of a department or team, or a manager direct reports, a role in the project.
type GroupRule struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid" json:"-"`
	ProjectID   uuid.UUID  `gorm:"type:uuid" json:"project_id"`
	Kind        string     `json:"kind"`
	RefID       uuid.UUID  `gorm:"type:uuid" json:"ref_id"`
	RoleKey     string     `json:"role_key"`
	CreatedBy   uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	RefreshedAt *time.Time `json:"refreshed_at"`
}

// TableName maps to project_group_rules.
func (GroupRule) TableName() string { return "project_group_rules" }

// GroupRuleInput adds a rule.
type GroupRuleInput struct {
	Kind  string    `json:"kind"`
	RefID uuid.UUID `json:"ref_id"`
	Role  string    `json:"role"`
}

func (in GroupRuleInput) validate() error {
	if in.Kind != KindDepartment && in.Kind != KindTeam && in.Kind != KindReportsOf {
		return kernel.Invalid("kind", "must be department, team or reports_of")
	}
	if in.RefID == uuid.Nil {
		return kernel.Invalid("ref_id", "is required")
	}
	if in.Role != api.RoleManager && in.Role != api.RoleMember && in.Role != api.RoleViewer {
		return kernel.Invalid("role", "must be manager, member or viewer")
	}
	return nil
}

func (s *Service) requirePeople() error {
	if s.p.People == nil {
		return kernel.Err(503, "PEOPLE_UNAVAILABLE", "organisation data is not available")
	}
	return nil
}

// GroupRules lists the project rules.
func (s *Service) GroupRules(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]GroupRule, error) {
	if err := s.require(ctx, a, projectID, api.ManageMembers); err != nil {
		return nil, err
	}
	var rows []GroupRule
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Order("created_at").Find(&rows).Error
	return rows, err
}

// AddGroupRule creates a rule and expands it into members at once.
func (s *Service) AddGroupRule(ctx context.Context, a api.Actor, projectID uuid.UUID, in GroupRuleInput) (*GroupRule, error) {
	if err := s.require(ctx, a, projectID, api.ManageMembers); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	if err := s.requirePeople(); err != nil {
		return nil, err
	}
	rule := GroupRule{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Kind: in.Kind, RefID: in.RefID, RoleKey: in.Role,
		CreatedBy: a.UserID, CreatedAt: s.now().UTC()}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.group_rule.added", ref{"project", projectID, map[string]any{"rule_id": rule.ID, "kind": in.Kind}})
		return kernel.Emit(ctx, tx, a.TenantID, "project.member.changed", map[string]any{"project_id": projectID, "rule_id": rule.ID})
	})
	if isDup(err) {
		return nil, kernel.Conflict("RULE_EXISTS", "this rule already exists")
	}
	if err != nil {
		return nil, err
	}
	return &rule, s.RefreshGroups(ctx, a.TenantID, projectID)
}

// RemoveGroupRule deletes a rule; members only that rule granted are removed.
func (s *Service) RemoveGroupRule(ctx context.Context, a api.Actor, projectID, ruleID uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ManageMembers); err != nil {
		return err
	}
	if err := s.requirePeople(); err != nil {
		return err
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`DELETE FROM project_group_rules WHERE id = ? AND project_id = ? AND tenant_id = ?`, ruleID, projectID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("rule")
		}
		s.audit(ctx, tx, a, "project.group_rule.removed", ref{"project", projectID, map[string]any{"rule_id": ruleID}})
		return nil
	})
	if err != nil {
		return err
	}
	return s.RefreshGroups(ctx, a.TenantID, projectID)
}

// RefreshProject re-resolves a project rules through the people port (manage_members).
func (s *Service) RefreshProject(ctx context.Context, a api.Actor, projectID uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ManageMembers); err != nil {
		return err
	}
	if err := s.requirePeople(); err != nil {
		return err
	}
	return s.RefreshGroups(ctx, a.TenantID, projectID)
}

var rank = map[string]int{api.RoleViewer: 1, api.RoleMember: 2, api.RoleManager: 3}

// desiredMembers resolves every rule to users, keeping the highest role per user. Network-free of the DB transaction.
func (s *Service) desiredMembers(ctx context.Context, rules []GroupRule, tenantID uuid.UUID) (map[uuid.UUID]string, error) {
	want := map[uuid.UUID]string{}
	for _, r := range rules {
		ids, err := s.resolve(ctx, s.p.People, tenantID, r)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if rank[r.RoleKey] > rank[want[id]] {
				want[id] = r.RoleKey
			}
		}
	}
	return want, nil
}

func (s *Service) resolve(ctx context.Context, p ports.People, tenantID uuid.UUID, r GroupRule) ([]uuid.UUID, error) {
	switch r.Kind {
	case KindDepartment:
		return p.DepartmentMembers(ctx, tenantID, r.RefID)
	case KindTeam:
		return p.TeamMembers(ctx, tenantID, r.RefID)
	default:
		return p.DirectReports(ctx, tenantID, r.RefID)
	}
}
