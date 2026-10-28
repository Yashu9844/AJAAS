package fields

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// DefinitionInput creates a field. Type is fixed for the life of the field.
type DefinitionInput struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Options []string `json:"options"`
}

// DefinitionUpdate edits a field; nil Options leaves the options unchanged.
type DefinitionUpdate struct {
	Name    *string   `json:"name"`
	Options *[]string `json:"options"`
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 60 {
		return "", kernel.Invalid("name", "length must be between 1 and 60")
	}
	return name, nil
}

func (s *Service) require(ctx context.Context, a api.Actor, projectID uuid.UUID, p api.Perm) error {
	acc, err := s.authz.Access(ctx, nil, a, projectID)
	if err != nil {
		return err
	}
	return acc.Require(p)
}

// Definitions lists the fields of a project in display order.
func (s *Service) Definitions(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Definition, error) {
	if err := s.require(ctx, a, projectID, api.ProjectRead); err != nil {
		return nil, err
	}
	var rows []Definition
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Order("position, created_at").Find(&rows).Error
	return rows, err
}

// CreateDefinition adds a field to a project (project.update); a project has at most 30.
func (s *Service) CreateDefinition(ctx context.Context, a api.Actor, projectID uuid.UUID, in DefinitionInput) (*Definition, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	name, err := cleanName(in.Name)
	if err != nil {
		return nil, err
	}
	if !validType(in.Type) {
		return nil, kernel.Invalid("type", "must be text, number, checkbox, date, url, select, multiselect or user")
	}
	opts, err := cleanOptions(in.Type, in.Options)
	if err != nil {
		return nil, err
	}
	d := Definition{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Name: name, Key: slugKey(name), Type: in.Type,
		Options: opts, CreatedBy: a.UserID, CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC()}
	err = s.inTx(ctx, func(tx *gorm.DB) error { return s.insertDefinition(ctx, tx, a, &d) })
	if isDup(err) {
		return nil, kernel.Conflict("FIELD_NAME_TAKEN", "a field with this name already exists")
	}
	return &d, err
}

func (s *Service) insertDefinition(ctx context.Context, tx *gorm.DB, a api.Actor, d *Definition) error {
	n, err := countRows(tx, "custom_field_definitions", "project_id = ?", d.ProjectID)
	if err != nil {
		return err
	}
	if n >= maxDefinitions {
		return kernel.Conflict("FIELD_LIMIT", "a project has at most 30 custom fields")
	}
	d.Position = int(n)
	if err := tx.Create(d).Error; err != nil {
		return err
	}
	s.audit(ctx, tx, a, "custom_field.created", ref{"custom_field", d.ID, map[string]any{"project_id": d.ProjectID, "type": d.Type}})
	return kernel.Emit(ctx, tx, a.TenantID, "project.updated", map[string]any{"project_id": d.ProjectID, "custom_field_id": d.ID})
}

// UpdateDefinition renames a field or changes its options; values that use a removed option are cleared.
func (s *Service) UpdateDefinition(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in DefinitionUpdate) (*Definition, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var d Definition
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&d).Error; err != nil {
			return mapMissing(err)
		}
		removed, err := in.apply(&d)
		if err != nil {
			return err
		}
		d.UpdatedAt = s.now().UTC()
		if err := tx.Save(&d).Error; err != nil {
			return err
		}
		if err := dropRemovedOptions(tx, d, removed); err != nil {
			return err
		}
		s.audit(ctx, tx, a, "custom_field.updated", ref{"custom_field", d.ID, map[string]any{"project_id": projectID}})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("FIELD_NAME_TAKEN", "a field with this name already exists")
	}
	return &d, err
}

func mapMissing(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return kernel.NotFound("custom field")
	}
	return err
}

// apply edits d and returns the options that disappeared.
func (in DefinitionUpdate) apply(d *Definition) ([]string, error) {
	if in.Name != nil {
		name, err := cleanName(*in.Name)
		if err != nil {
			return nil, err
		}
		d.Name = name
	}
	if in.Options == nil {
		return nil, nil
	}
	opts, err := cleanOptions(d.Type, *in.Options)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, o := range d.Options {
		if !contains(opts, o) {
			removed = append(removed, o)
		}
	}
	d.Options = opts
	return removed, nil
}

// dropRemovedOptions clears the values that still point at an option that was removed.
func dropRemovedOptions(tx *gorm.DB, d Definition, removed []string) error {
	for _, o := range removed {
		var err error
		if d.Type == TypeSelect {
			err = tx.Exec(`DELETE FROM custom_field_values WHERE field_id = ? AND value = to_jsonb(?::text)`, d.ID, o).Error
		} else {
			err = tx.Exec(`UPDATE custom_field_values SET value = value - ?::text WHERE field_id = ? AND jsonb_typeof(value) = 'array'`, o, d.ID).Error
		}
		if err != nil {
			return err
		}
	}
	return tx.Exec(`DELETE FROM custom_field_values WHERE field_id = ? AND value = '[]'::jsonb`, d.ID).Error
}

// DeleteDefinition removes a field and every value stored for it.
func (s *Service) DeleteDefinition(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`DELETE FROM custom_field_definitions WHERE id = ? AND project_id = ? AND tenant_id = ?`, id, projectID, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("custom field")
		}
		s.audit(ctx, tx, a, "custom_field.deleted", ref{"custom_field", id, map[string]any{"project_id": projectID}})
		return nil
	})
}
