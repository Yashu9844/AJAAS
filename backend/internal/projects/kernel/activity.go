package kernel

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Activity is one append-only history entry (CM-04). Data carries ids and field values; no free-text personal data.
type Activity struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	TaskID    *uuid.UUID
	ActorID   *uuid.UUID
	Type      string // created, updated, moved, assigned, commented, attached, related, time_logged, ...
	Data      map[string]any
	Source    string // user (default), system, integration
}

// LogActivity appends an entry in the caller transaction and bumps the project recent-activity time.
func LogActivity(ctx context.Context, tx *gorm.DB, a Activity) error {
	if a.Source == "" {
		a.Source = "user"
	}
	if a.Data == nil {
		a.Data = map[string]any{}
	}
	raw, err := json.Marshal(a.Data)
	if err != nil {
		return err
	}
	db := tx.WithContext(ctx)
	if err := db.Exec(`INSERT INTO activity (id, tenant_id, project_id, task_id, actor_user_id, type, data, source)
		VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?)`, uuid.New(), a.TenantID, a.ProjectID, a.TaskID, a.ActorID, a.Type, string(raw), a.Source).Error; err != nil {
		return err
	}
	return db.Exec(`UPDATE projects SET last_activity_at = now() WHERE id = ?`, a.ProjectID).Error
}
