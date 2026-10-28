// Package project is the project context of M2: projects, members, roles, columns, labels, milestones and favourites,
// and the Authorizer every other M2 context uses (specification sections 1 and 8).
package project

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Project is a unit of work with its own members, columns and tasks (PR-01).
type Project struct {
	ID                  uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID            uuid.UUID    `gorm:"type:uuid" json:"-"`
	Key                 string       `json:"key"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Status              string       `json:"status"`
	Visibility          string       `json:"visibility"`
	Mode                string       `json:"mode"`
	OwnerUserID         uuid.UUID    `gorm:"type:uuid" json:"owner_user_id"`
	StartOn             *kernel.Date `gorm:"type:date" json:"start_on"`
	DueOn               *kernel.Date `gorm:"type:date" json:"due_on"`
	Icon                string       `json:"icon"`
	Color               string       `json:"color"`
	TaskCounter         int          `json:"task_counter"`
	TimeTrackingEnabled bool         `json:"time_tracking_enabled"`
	Version             int          `json:"version"`
	CreatedBy           uuid.UUID    `gorm:"type:uuid" json:"created_by"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	LastActivityAt      time.Time    `json:"last_activity_at"`
	ArchivedAt          *time.Time   `json:"archived_at"`
	DeletedAt           *time.Time   `json:"-"`
	Favorite            bool         `gorm:"->;-:migration" json:"favorite"`
	MyRole              string       `gorm:"->;-:migration" json:"my_role"`
}

// TableName maps to projects.projects.
func (Project) TableName() string { return "projects" }

// Member links a user to a project with a built-in or custom role (PR-04, PR-05).
type Member struct {
	ProjectID    uuid.UUID  `gorm:"type:uuid;primaryKey" json:"project_id"`
	UserID       uuid.UUID  `gorm:"type:uuid;primaryKey" json:"user_id"`
	TenantID     uuid.UUID  `gorm:"type:uuid" json:"-"`
	RoleKey      *string    `json:"role_key"`
	CustomRoleID *uuid.UUID `gorm:"type:uuid" json:"custom_role_id"`
	Active       bool       `json:"active"`
	AddedBy      uuid.UUID  `gorm:"type:uuid" json:"added_by"`
	AddedAt      time.Time  `json:"added_at"`
}

// TableName maps to project_members.
func (Member) TableName() string { return "project_members" }

// CustomRole is a tenant-defined named set of project permissions (PR-05).
type CustomRole struct {
	ID          uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID         `gorm:"type:uuid" json:"-"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Permissions kernel.StringList `gorm:"type:jsonb" json:"permissions"`
	CreatedBy   uuid.UUID         `gorm:"type:uuid" json:"created_by"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// TableName maps to project_roles.
func (CustomRole) TableName() string { return "project_roles" }

// Column is a board status of a project (PR-06).
type Column struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid" json:"-"`
	ProjectID uuid.UUID `gorm:"type:uuid" json:"project_id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Position  int       `json:"position"`
	Color     string    `json:"color"`
	Category  string    `json:"category"`
	WipLimit  *int      `json:"wip_limit"`
	WipHard   bool      `json:"wip_hard"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TaskCount int       `gorm:"->;-:migration" json:"task_count"`
}

// TableName maps to board_columns.
func (Column) TableName() string { return "board_columns" }

// Label tags tasks; ProjectID is nil for a tenant-wide label (PR-07).
type Label struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid" json:"-"`
	ProjectID *uuid.UUID `gorm:"type:uuid" json:"project_id"`
	Name      string     `json:"name"`
	Color     string     `json:"color"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName maps to labels.
func (Label) TableName() string { return "labels" }

// Milestone groups tasks toward a date (PR-08).
type Milestone struct {
	ID        uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID  uuid.UUID    `gorm:"type:uuid" json:"-"`
	ProjectID uuid.UUID    `gorm:"type:uuid" json:"project_id"`
	Name      string       `json:"name"`
	DueOn     *kernel.Date `gorm:"type:date" json:"due_on"`
	Status    string       `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Total     int          `gorm:"->;-:migration" json:"total_tasks"`
	Done      int          `gorm:"->;-:migration" json:"done_tasks"`
}

// TableName maps to milestones.
func (Milestone) TableName() string { return "milestones" }
