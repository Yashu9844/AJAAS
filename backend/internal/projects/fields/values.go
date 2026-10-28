package fields

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// SetInput sets values by field key; a null value clears the field. Version, when given, must match the task version.
type SetInput struct {
	Values  map[string]any `json:"values"`
	Version *int           `json:"version"`
}

// change is one validated edit: Value nil clears the field.
type change struct {
	def   Definition
	value any
}

// TaskFields lists every field of the task project with the task value (nil when unset).
func (s *Service) TaskFields(ctx context.Context, a api.Actor, taskID uuid.UUID) ([]Value, error) {
	ref, acc, err := s.tasks.Task(ctx, s.db, a, taskID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, err
	}
	return s.valuesOf(ctx, s.db, a.TenantID, ref.ProjectID, taskID)
}

func (s *Service) valuesOf(ctx context.Context, db *gorm.DB, tenantID, projectID, taskID uuid.UUID) ([]Value, error) {
	var rows []struct {
		Definition
		Raw []byte `gorm:"column:raw"`
	}
	err := db.WithContext(ctx).Raw(`SELECT d.*, v.value::text AS raw FROM custom_field_definitions d
		LEFT JOIN custom_field_values v ON v.field_id = d.id AND v.task_id = ?
		WHERE d.project_id = ? AND d.tenant_id = ? ORDER BY d.position, d.created_at`, taskID, projectID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Value, 0, len(rows))
	for _, r := range rows {
		var v any
		if len(r.Raw) > 0 {
			if err := json.Unmarshal(r.Raw, &v); err != nil {
				return nil, err
			}
		}
		out = append(out, Value{FieldID: r.ID, Key: r.Key, Name: r.Name, Type: r.Type, Value: v})
	}
	return out, nil
}

// SetTaskFields validates and stores values for a task (task.update), bumping the task version (TK-03).
func (s *Service) SetTaskFields(ctx context.Context, a api.Actor, taskID uuid.UUID, in SetInput) ([]Value, error) {
	ref, acc, err := s.tasks.Task(ctx, s.db, a, taskID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskUpdate); err != nil {
		return nil, err
	}
	if len(in.Values) == 0 || len(in.Values) > maxDefinitions {
		return nil, kernel.Invalid("values", "give between 1 and 30 field values")
	}
	changes, err := s.validate(ctx, a, ref.ProjectID, in.Values)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error { return s.apply(ctx, tx, a, ref, in.Version, changes) })
	if err != nil {
		return nil, err
	}
	return s.valuesOf(ctx, s.db, a.TenantID, ref.ProjectID, taskID)
}

// validate resolves every key to a definition of the project and checks its value.
func (s *Service) validate(ctx context.Context, a api.Actor, projectID uuid.UUID, values map[string]any) ([]change, error) {
	var defs []Definition
	if err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Find(&defs).Error; err != nil {
		return nil, err
	}
	byKey := map[string]Definition{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	member := func(id uuid.UUID) (bool, error) {
		n, err := countRows(s.db.WithContext(ctx), "project_members", "project_id = ? AND user_id = ? AND active", projectID, id)
		return n > 0, err
	}
	out := make([]change, 0, len(values))
	for key, v := range values {
		d, ok := byKey[key]
		if !ok {
			return nil, kernel.Invalid(key, "is not a custom field of this project")
		}
		if v == nil {
			out = append(out, change{def: d})
			continue
		}
		clean, err := checkValue(d, v, member)
		if err != nil {
			return nil, err
		}
		out = append(out, change{def: d, value: clean})
	}
	return out, nil
}
