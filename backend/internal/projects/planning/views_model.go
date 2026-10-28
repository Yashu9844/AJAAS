package planning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// RawJSON is a validated JSON document stored in a JSONB column and written to responses as-is.
type RawJSON []byte

// Value stores the document.
func (r RawJSON) Value() (driver.Value, error) {
	if len(r) == 0 {
		return "null", nil
	}
	return string(r), nil
}

// Scan reads a JSONB value.
func (r *RawJSON) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		*r = append((*r)[:0], v...)
	case string:
		*r = RawJSON(v)
	case nil:
		*r = nil
	default:
		return fmt.Errorf("planning: cannot scan %T into RawJSON", src)
	}
	return nil
}

// MarshalJSON writes the stored document without re-encoding it.
func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}

// SavedView is a named filter/sort/grouping/layout combination, personal or shared with a project (VW-06).
type SavedView struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid" json:"-"`
	ProjectID   *uuid.UUID `gorm:"type:uuid" json:"project_id"`
	OwnerUserID uuid.UUID  `gorm:"type:uuid" json:"owner_user_id"`
	Name        string     `json:"name"`
	Layout      string     `json:"layout"`
	Filters     RawJSON    `gorm:"type:jsonb" json:"filters"`
	Sort        RawJSON    `gorm:"type:jsonb" json:"sort"`
	Grouping    string     `json:"grouping"`
	Shared      bool       `json:"shared"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName maps to saved_views.
func (SavedView) TableName() string { return "saved_views" }

// ViewInput creates or edits a saved view; on edit nil/absent fields are unchanged and Version, when set, must match.
// ProjectID is read on create only.
type ViewInput struct {
	Name      *string         `json:"name"`
	Layout    *string         `json:"layout"`
	ProjectID *uuid.UUID      `json:"project_id"`
	Filters   json.RawMessage `json:"filters"`
	Sort      json.RawMessage `json:"sort"`
	Grouping  *string         `json:"grouping"`
	Shared    *bool           `json:"shared"`
	Version   *int            `json:"version"`
}

// ViewFilters is the closed set of filters a saved view may hold. It is decoded with unknown fields refused and stored
// re-encoded, so no free-form fragment can be persisted and none is ever interpolated into SQL.
type ViewFilters struct {
	ColumnIDs   []uuid.UUID  `json:"column_ids,omitempty"`
	Categories  []string     `json:"categories,omitempty"`
	Priorities  []string     `json:"priorities,omitempty"`
	AssigneeIDs []uuid.UUID  `json:"assignee_ids,omitempty"`
	Mine        bool         `json:"mine,omitempty"`
	LabelIDs    []uuid.UUID  `json:"label_ids,omitempty"`
	MilestoneID *uuid.UUID   `json:"milestone_id,omitempty"`
	Sprint      string       `json:"sprint,omitempty"`
	DueAfter    *kernel.Date `json:"due_after,omitempty"`
	DueBefore   *kernel.Date `json:"due_before,omitempty"`
	Overdue     bool         `json:"overdue,omitempty"`
	Query       string       `json:"q,omitempty"`
}

// SortKey orders a view by one allow-listed field.
type SortKey struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

// ViewFilter narrows the saved view list.
type ViewFilter struct {
	ProjectID *uuid.UUID
	Cursor    string
	Limit     int
}
