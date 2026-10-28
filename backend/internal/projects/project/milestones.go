package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// MilestoneInput creates or edits a milestone; nil fields are unchanged on edit.
type MilestoneInput struct {
	Name   *string      `json:"name"`
	DueOn  *kernel.Date `json:"due_on"`
	Status *string      `json:"status"`
}

// Milestones lists a project milestones with progress counts (PR-08).
func (s *Service) Milestones(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Milestone, error) {
	if _, err := s.Access(ctx, nil, a, projectID); err != nil {
		return nil, err
	}
	var rows []Milestone
	err := s.db.WithContext(ctx).Raw(`SELECT m.*,
		(SELECT count(*) FROM tasks t WHERE t.milestone_id = m.id AND t.deleted_at IS NULL) AS total,
		(SELECT count(*) FROM tasks t JOIN board_columns c ON c.id = t.column_id
			WHERE t.milestone_id = m.id AND t.deleted_at IS NULL AND c.category = 'done') AS done
		FROM milestones m WHERE m.project_id = ? AND m.tenant_id = ? ORDER BY m.due_on NULLS LAST, lower(m.name)`,
		projectID, a.TenantID).Scan(&rows).Error
	return rows, err
}

func (in MilestoneInput) apply(m *Milestone) error {
	if in.Name != nil {
		name, err := text("name", *in.Name, 1, maxName)
		if err != nil {
			return err
		}
		m.Name = name
	}
	if in.DueOn != nil {
		m.DueOn = in.DueOn
	}
	if in.Status != nil {
		if *in.Status != "open" && *in.Status != "reached" {
			return kernel.Invalid("status", "must be open or reached")
		}
		m.Status = *in.Status
	}
	return nil
}

// CreateMilestone adds a milestone (milestone.manage).
func (s *Service) CreateMilestone(ctx context.Context, a api.Actor, projectID uuid.UUID, in MilestoneInput) (*Milestone, error) {
	if err := s.require(ctx, a, projectID, api.MilestoneManage); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	m := Milestone{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Status: "open", CreatedAt: now, UpdatedAt: now}
	if err := in.apply(&m); err != nil {
		return nil, err
	}
	if m.Name == "" {
		return nil, kernel.Invalid("name", "is required")
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "milestone.created", "milestone", m.ID, map[string]any{"project_id": projectID})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("MILESTONE_NAME_TAKEN", "a milestone with this name already exists")
	}
	return &m, err
}

// UpdateMilestone edits a milestone.
func (s *Service) UpdateMilestone(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in MilestoneInput) (*Milestone, error) {
	if err := s.require(ctx, a, projectID, api.MilestoneManage); err != nil {
		return nil, err
	}
	var m Milestone
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&m).Error; err != nil {
			return mapMissing(err, "milestone")
		}
		if err := in.apply(&m); err != nil {
			return err
		}
		m.UpdatedAt = s.now().UTC()
		return tx.Save(&m).Error
	})
	if isDup(err) {
		return nil, kernel.Conflict("MILESTONE_NAME_TAKEN", "a milestone with this name already exists")
	}
	return &m, err
}

// DeleteMilestone removes a milestone; its tasks keep existing without one.
func (s *Service) DeleteMilestone(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.MilestoneManage); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE tasks SET milestone_id = NULL, version = version + 1 WHERE milestone_id = ?`, id).Error; err != nil {
			return err
		}
		res := tx.Exec(`DELETE FROM milestones WHERE id = ? AND project_id = ? AND tenant_id = ?`, id, projectID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("milestone")
		}
		s.audit(ctx, tx, a, "milestone.deleted", "milestone", id, nil)
		return nil
	})
}
