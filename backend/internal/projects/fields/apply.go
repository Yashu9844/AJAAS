package fields

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// bumpVersion increments the task version when it still matches (or always when no version was sent).
func bumpVersion(tx *gorm.DB, tenantID, taskID uuid.UUID, expected *int) (int, error) {
	var v int
	res := tx.Raw(`UPDATE tasks SET version = version + 1, updated_at = now() WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL
		AND (?::int IS NULL OR version = ?) RETURNING version`, taskID, tenantID, expected, expected).Scan(&v)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 1 {
		return v, nil
	}
	var cur int
	if err := tx.Raw(`SELECT version FROM tasks WHERE id = ? AND tenant_id = ?`, taskID, tenantID).Scan(&cur).Error; err != nil {
		return 0, err
	}
	return 0, &kernel.VersionConflict{Current: cur}
}

// apply stores the changes in one transaction together with the version bump, the activity entry and the event.
// edit is a validated set of changes with the version the caller expects.
type edit struct {
	expected *int
	changes  []change
}

func (s *Service) apply(ctx context.Context, tx *gorm.DB, a api.Actor, t api.TaskRef, e edit) error {
	changes := e.changes
	version, err := bumpVersion(tx, a.TenantID, t.ID, e.expected)
	if err != nil {
		return err
	}
	entries := make([]map[string]any, 0, len(changes))
	ids := make([]uuid.UUID, 0, len(changes))
	for _, c := range changes {
		old, err := writeValue(tx, a, t.ID, c)
		if err != nil {
			return err
		}
		entries = append(entries, entry(c, old))
		ids = append(ids, c.def.ID)
	}
	actor := a.UserID
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: t.ProjectID, TaskID: &t.ID, ActorID: &actor,
		Type: "custom_fields.updated", Data: map[string]any{"changes": entries}}); err != nil {
		return err
	}
	s.audit(ctx, tx, a, "task.custom_fields.updated", ref{"task", t.ID, map[string]any{"project_id": t.ProjectID, "field_ids": ids}})
	return kernel.Emit(ctx, tx, a.TenantID, "task.updated", map[string]any{"task_id": t.ID, "project_id": t.ProjectID, "version": version, "custom_field_ids": ids})
}

// writeValue upserts or clears one value and returns the previous one.
func writeValue(tx *gorm.DB, a api.Actor, taskID uuid.UUID, c change) (any, error) {
	var raws []string
	if err := tx.Raw(`SELECT value::text FROM custom_field_values WHERE task_id = ? AND field_id = ?`, taskID, c.def.ID).Scan(&raws).Error; err != nil {
		return nil, err
	}
	var old any
	if len(raws) == 1 {
		if err := json.Unmarshal([]byte(raws[0]), &old); err != nil {
			return nil, err
		}
	}
	if c.value == nil {
		return old, tx.Exec(`DELETE FROM custom_field_values WHERE task_id = ? AND field_id = ?`, taskID, c.def.ID).Error
	}
	enc, err := json.Marshal(c.value)
	if err != nil {
		return nil, err
	}
	return old, tx.Exec(`INSERT INTO custom_field_values (task_id, field_id, tenant_id, value, updated_by) VALUES (?, ?, ?, ?::jsonb, ?)
		ON CONFLICT (task_id, field_id) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		taskID, c.def.ID, a.TenantID, string(enc), a.UserID).Error
}

// entry is the activity record of one change; free text and URLs are recorded by field only (like descriptions).
func entry(c change, old any) map[string]any {
	e := map[string]any{"field_id": c.def.ID, "key": c.def.Key}
	if c.def.Type != TypeText && c.def.Type != TypeURL {
		e["old"], e["new"] = old, c.value
	}
	return e
}
