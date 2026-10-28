package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// LabelInput creates or edits a label.
type LabelInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (in LabelInput) validate() (string, error) {
	name, err := text("name", in.Name, 1, 60)
	if err != nil {
		return "", err
	}
	if !colorPattern.MatchString(in.Color) {
		return "", kernel.Invalid("color", "must be #RRGGBB")
	}
	return name, nil
}

// Labels lists the labels usable in a project: its own and the tenant-wide ones (PR-07).
func (s *Service) Labels(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Label, error) {
	if _, err := s.Access(ctx, nil, a, projectID); err != nil {
		return nil, err
	}
	var rows []Label
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND (project_id = ? OR project_id IS NULL)", a.TenantID, projectID).
		Order("project_id NULLS FIRST, lower(name)").Find(&rows).Error
	return rows, err
}

// TenantLabels lists the tenant-wide labels.
func (s *Service) TenantLabels(ctx context.Context, a api.Actor) ([]Label, error) {
	var rows []Label
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND project_id IS NULL", a.TenantID).Order("lower(name)").Find(&rows).Error
	return rows, err
}

// CreateLabel adds a project label (label.manage) or, with a nil projectID, a tenant-wide label (tenant administrators).
func (s *Service) CreateLabel(ctx context.Context, a api.Actor, projectID *uuid.UUID, in LabelInput) (*Label, error) {
	if err := s.labelGuard(ctx, a, projectID); err != nil {
		return nil, err
	}
	name, err := in.validate()
	if err != nil {
		return nil, err
	}
	l := Label{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Name: name, Color: in.Color, CreatedAt: s.now().UTC()}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&l).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "label.created", "label", l.ID, nil)
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("LABEL_NAME_TAKEN", "a label with this name already exists")
	}
	return &l, err
}

func (s *Service) labelGuard(ctx context.Context, a api.Actor, projectID *uuid.UUID) error {
	if projectID == nil {
		if !a.ManageAll {
			return kernel.Forbidden("projects:manage_all")
		}
		return nil
	}
	return s.require(ctx, a, *projectID, api.LabelManage)
}

// loadLabel finds a label in the given scope (a project, or the tenant when projectID is nil).
func (s *Service) loadLabel(tx *gorm.DB, a api.Actor, projectID *uuid.UUID, id uuid.UUID) (*Label, error) {
	var l Label
	q := tx.Where("id = ? AND tenant_id = ?", id, a.TenantID)
	if projectID == nil {
		q = q.Where("project_id IS NULL")
	} else {
		q = q.Where("project_id = ?", *projectID)
	}
	if err := q.Take(&l).Error; err != nil {
		return nil, mapMissing(err, "label")
	}
	return &l, nil
}

// UpdateLabel renames or recolors a label.
func (s *Service) UpdateLabel(ctx context.Context, a api.Actor, projectID *uuid.UUID, id uuid.UUID, in LabelInput) (*Label, error) {
	if err := s.labelGuard(ctx, a, projectID); err != nil {
		return nil, err
	}
	name, err := in.validate()
	if err != nil {
		return nil, err
	}
	var l *Label
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		var lerr error
		if l, lerr = s.loadLabel(tx, a, projectID, id); lerr != nil {
			return lerr
		}
		l.Name, l.Color = name, in.Color
		return tx.Save(l).Error
	})
	if isDup(err) {
		return nil, kernel.Conflict("LABEL_NAME_TAKEN", "a label with this name already exists")
	}
	return l, err
}

// DeleteLabel removes a label from every task and deletes it.
func (s *Service) DeleteLabel(ctx context.Context, a api.Actor, projectID *uuid.UUID, id uuid.UUID) error {
	if err := s.labelGuard(ctx, a, projectID); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := s.loadLabel(tx, a, projectID, id); err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM task_labels WHERE label_id = ?`, id).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "label.deleted", "label", id, nil)
		return tx.Exec(`DELETE FROM labels WHERE id = ?`, id).Error
	})
}
