package automation

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Rules lists the rules of a project, oldest first (project.read); a project has at most 50, so the list is not paged.
func (s *Service) Rules(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Rule, error) {
	if err := s.require(ctx, a, projectID, api.ProjectRead); err != nil {
		return nil, err
	}
	var rows []Rule
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// CreateRule adds a rule that runs as the caller (project.update); a project has at most 50.
func (s *Service) CreateRule(ctx context.Context, a api.Actor, projectID uuid.UUID, in RuleInput) (*Rule, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	in, err := (scope{s, a.TenantID, projectID}).check(ctx, in)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	r := Rule{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Name: in.Name, Trigger: in.Trigger, Conditions: in.Conditions,
		Actions: in.Actions, Active: in.Active == nil || *in.Active, CreatedBy: a.UserID, Version: 1, CreatedAt: now, UpdatedAt: now}
	err = s.inTx(ctx, func(tx *gorm.DB) error { return s.insertRule(ctx, tx, a, &r) })
	if isDup(err) {
		return nil, kernel.Conflict("RULE_NAME_TAKEN", "a rule with this name already exists")
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) insertRule(ctx context.Context, tx *gorm.DB, a api.Actor, r *Rule) error {
	if err := lockProject(tx, a.TenantID, r.ProjectID); err != nil {
		return err
	}
	n, err := countRows(tx, "workflow_rules", "project_id = ? AND tenant_id = ?", r.ProjectID, a.TenantID)
	if err != nil {
		return err
	}
	if n >= maxRules {
		return kernel.Conflict("RULE_LIMIT", "a project has at most 50 rules")
	}
	if err := tx.Create(r).Error; err != nil {
		return err
	}
	s.audit(ctx, tx, a, "workflow_rule.created", ref{"workflow_rule", r.ID, map[string]any{"project_id": r.ProjectID, "trigger": r.Trigger}})
	return nil
}

// lockProject serialises writers of one project so the rule count limit holds under concurrency.
func lockProject(tx *gorm.DB, tenantID, projectID uuid.UUID) error {
	return tx.Exec(`SELECT id FROM projects WHERE id = ? AND tenant_id = ? FOR UPDATE`, projectID, tenantID).Error
}

// UpdateRule edits a rule (project.update). Any edit clears the failure flag and counter; Active true re-enables it.
func (s *Service) UpdateRule(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in RuleUpdate) (*Rule, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var r Rule
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := lockRule(tx, ruleKey{a.TenantID, projectID, id}, &r); err != nil {
			return err
		}
		if in.Version != nil && *in.Version != r.Version {
			return &kernel.VersionConflict{Current: r.Version}
		}
		def, err := (scope{s, a.TenantID, projectID}).check(ctx, in.merge(r))
		if err != nil {
			return err
		}
		r.Name, r.Trigger, r.Conditions, r.Actions = def.Name, def.Trigger, def.Conditions, def.Actions
		r.Active, r.ConsecutiveFailures, r.FlaggedCode = def.Active == nil || *def.Active, 0, nil
		r.Version, r.UpdatedAt = r.Version+1, s.now().UTC()
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "workflow_rule.updated", ref{"workflow_rule", r.ID, map[string]any{"project_id": projectID, "active": r.Active}})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("RULE_NAME_TAKEN", "a rule with this name already exists")
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// merge lays the given members over the stored rule as a full definition to validate.
func (in RuleUpdate) merge(r Rule) RuleInput {
	out := RuleInput{Name: r.Name, Trigger: r.Trigger, Conditions: r.Conditions, Actions: r.Actions, Active: &r.Active}
	if in.Name != nil {
		out.Name = *in.Name
	}
	if in.Trigger != nil {
		out.Trigger = *in.Trigger
	}
	if in.Conditions != nil {
		out.Conditions = *in.Conditions
	}
	if in.Actions != nil {
		out.Actions = *in.Actions
	}
	if in.Active != nil {
		out.Active = in.Active
	}
	return out
}

// ruleKey identifies one rule inside its project and tenant.
type ruleKey struct{ tenantID, projectID, id uuid.UUID }

func lockRule(tx *gorm.DB, k ruleKey, dst *Rule) error {
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND tenant_id = ?", k.id, k.projectID, k.tenantID).Take(dst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return kernel.NotFound("rule")
	}
	return err
}

// DeleteRule removes a rule and its run history (project.update).
func (s *Service) DeleteRule(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`DELETE FROM workflow_rules WHERE id = ? AND project_id = ? AND tenant_id = ?`, id, projectID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("rule")
		}
		s.audit(ctx, tx, a, "workflow_rule.deleted", ref{"workflow_rule", id, map[string]any{"project_id": projectID}})
		return nil
	})
}

// load reads one rule of a project.
func (s *Service) load(ctx context.Context, tenantID, projectID, id uuid.UUID) (*Rule, error) {
	var r Rule
	err := s.db.WithContext(ctx).Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, tenantID).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("rule")
	}
	return &r, err
}
