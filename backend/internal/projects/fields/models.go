// Package fields is the custom-fields context of M2 (specification section 9, phase 4): typed, project-scoped field
// definitions and per-task values. It reaches tasks and project access only through the api contracts.
package fields

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Field types.
const (
	TypeText        = "text"
	TypeNumber      = "number"
	TypeCheckbox    = "checkbox"
	TypeDate        = "date"
	TypeURL         = "url"
	TypeSelect      = "select"
	TypeMultiselect = "multiselect"
	TypeUser        = "user"
)

// Definition is one custom field of a project.
type Definition struct {
	ID        uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID  uuid.UUID         `gorm:"type:uuid" json:"-"`
	ProjectID uuid.UUID         `gorm:"type:uuid" json:"project_id"`
	Name      string            `json:"name"`
	Key       string            `json:"key"`
	Type      string            `json:"type"`
	Options   kernel.StringList `gorm:"type:jsonb" json:"options"`
	Position  int               `json:"position"`
	CreatedBy uuid.UUID         `gorm:"type:uuid" json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// TableName maps to custom_field_definitions.
func (Definition) TableName() string { return "custom_field_definitions" }

// Value is a task's value for one field, as returned by the API (nil Value means unset).
type Value struct {
	FieldID uuid.UUID `json:"field_id"`
	Key     string    `json:"key"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   any       `json:"value"`
}
