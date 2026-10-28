package planning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Sprint states (SP-01).
const (
	SprintPlanned   = "planned"
	SprintActive    = "active"
	SprintCompleted = "completed"
)

// Report is the commitment summary stored when a sprint completes (SP-02). MovedTo is "backlog" or the next sprint id.
type Report struct {
	CommittedCount           int    `json:"committed_count"`
	CompletedCount           int    `json:"completed_count"`
	CommittedEstimateMinutes int    `json:"committed_estimate_minutes"`
	CompletedEstimateMinutes int    `json:"completed_estimate_minutes"`
	MovedUnfinishedCount     int    `json:"moved_unfinished_count"`
	MovedTo                  string `json:"moved_to,omitempty"`
}

// Value stores the report as JSONB.
func (r Report) Value() (driver.Value, error) {
	raw, err := json.Marshal(r)
	return string(raw), err
}

// Scan reads the JSONB report.
func (r *Report) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, r)
	case string:
		return json.Unmarshal([]byte(v), r)
	case nil:
		*r = Report{}
		return nil
	}
	return fmt.Errorf("planning: cannot scan %T into Report", src)
}

// Sprint is a time-boxed batch of tasks in a scrum project (SP-01).
type Sprint struct {
	ID          uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID    `gorm:"type:uuid" json:"-"`
	ProjectID   uuid.UUID    `gorm:"type:uuid" json:"project_id"`
	Name        string       `json:"name"`
	Goal        string       `json:"goal"`
	StartOn     *kernel.Date `gorm:"type:date" json:"start_on"`
	EndOn       *kernel.Date `gorm:"type:date" json:"end_on"`
	Status      string       `json:"status"`
	StartedAt   *time.Time   `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	Report      *Report      `gorm:"type:jsonb" json:"report"`
	Version     int          `json:"version"`
	CreatedBy   uuid.UUID    `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	TaskCount   int          `gorm:"->;-:migration" json:"task_count"`
}

// TableName maps to sprints.
func (Sprint) TableName() string { return "sprints" }

// SprintInput creates or edits a sprint; on edit nil fields are unchanged and Version, when set, must match.
type SprintInput struct {
	Name    *string      `json:"name"`
	Goal    *string      `json:"goal"`
	StartOn *kernel.Date `json:"start_on"`
	EndOn   *kernel.Date `json:"end_on"`
	Version *int         `json:"version"`
}

// CompleteInput closes a sprint; MoveUnfinishedTo is "backlog" or the id of the next planned sprint (SP-02).
type CompleteInput struct {
	MoveUnfinishedTo string `json:"move_unfinished_to"`
}

// SprintFilter narrows the sprint list.
type SprintFilter struct {
	Status string
	Cursor string
	Limit  int
}

// ReportView is the report endpoint payload: stored for a completed sprint, computed live otherwise.
type ReportView struct {
	SprintID uuid.UUID `json:"sprint_id"`
	Status   string    `json:"status"`
	Live     bool      `json:"live"`
	Report   Report    `json:"report"`
}
