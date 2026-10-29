// Package publiclink is the public project links context of M2 (specification section 9): a revocable secret URL that
// shows a strict allow-list of a project (name, columns, tasks without people) to anyone who holds it, read-only and
// without a login. The tenant and the project come from the token row only, never from the client.
package publiclink

import (
	"time"

	"github.com/google/uuid"
)

const (
	maxActiveLinks = 3
	// tokenLen is the length of 32 random bytes in unpadded base64url.
	tokenLen = 43
	// MaxDescription caps a description shown on a public page.
	MaxDescription = 2000
)

// Link is one public link. Token and Path are set once, in the response to creation; only the hash is stored.
type Link struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid" json:"-"`
	ProjectID        uuid.UUID  `gorm:"type:uuid" json:"project_id"`
	TokenHash        string     `json:"-"`
	ShowDescriptions bool       `json:"show_descriptions"`
	CreatedBy        uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
	Token            string     `gorm:"-" json:"token,omitempty"`
	Path             string     `gorm:"-" json:"path,omitempty"`
}

// TableName maps to public_links.
func (Link) TableName() string { return "public_links" }

// Input creates a link.
type Input struct {
	ShowDescriptions bool `json:"show_descriptions"`
}

// Project is the public identity of a project.
type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Column is a board column as the public sees it.
type Column struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Color    string `json:"color"`
}

// Task is a task as the public sees it: no people, comments, files, time or history.
type Task struct {
	Key         string     `json:"key"`
	Title       string     `json:"title"`
	Column      string     `json:"column"`
	Priority    string     `json:"priority"`
	DueAt       *time.Time `json:"due_at"`
	Labels      []string   `json:"labels"`
	Progress    int        `json:"progress"`
	Description *string    `json:"description,omitempty"`
}

// Page is one page of the public view; the project and columns repeat on every page.
type Page struct {
	Project Project  `json:"project"`
	Columns []Column `json:"columns"`
	Tasks   []Task   `json:"tasks"`
	Limit   int      `json:"-"`
	Next    string   `json:"-"`
}
