package webhooks

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Webhook formats.
const (
	FormatGeneric = "generic"
	FormatSlack   = "slack"
	FormatDiscord = "discord"
)

// Delivery statuses.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	StatusDead      = "dead"
)

// Providers and link kinds.
const (
	ProviderGitHub = "github"
	ProviderGitea  = "gitea"
	KindPR         = "pr"
	KindBranch     = "branch"
	KindCommit     = "commit"
	StateOpen      = "open"
	StateMerged    = "merged"
	StateClosed    = "closed"
)

// Webhook is one outgoing endpoint of a project. The signing secret is never part of the model's JSON.
type Webhook struct {
	ID           uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID         `gorm:"type:uuid" json:"-"`
	ProjectID    uuid.UUID         `gorm:"type:uuid" json:"project_id"`
	Name         string            `json:"name"`
	URL          string            `gorm:"column:url" json:"url"`
	Format       string            `json:"format"`
	EventTypes   kernel.StringList `gorm:"type:jsonb" json:"event_types"`
	Active       bool              `json:"active"`
	SecretEnc    []byte            `json:"-"`
	SecretHint   string            `json:"secret_hint"`
	FailureCount int               `json:"failure_count"`
	DisabledAt   *time.Time        `json:"disabled_at"`
	CreatedBy    uuid.UUID         `gorm:"type:uuid" json:"created_by"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// TableName maps to webhooks.
func (Webhook) TableName() string { return "webhooks" }

// WebhookWithSecret is returned once, when a secret is created or rotated.
type WebhookWithSecret struct {
	Webhook
	Secret string `json:"secret"`
}

// Delivery is one event sent (or to be sent) to one webhook. It never holds the body or the response.
type Delivery struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid" json:"-"`
	WebhookID      uuid.UUID  `gorm:"type:uuid" json:"webhook_id"`
	EventID        uuid.UUID  `gorm:"type:uuid" json:"event_id"`
	EventType      string     `json:"event_type"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  *time.Time `json:"next_attempt_at"`
	ResponseStatus *int       `json:"response_status"`
	LastError      string     `json:"last_error"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`
}

// TableName maps to webhook_deliveries.
func (Delivery) TableName() string { return "webhook_deliveries" }

// Integration is a git provider hooked to one project; its incoming URL is /integrations/{provider}/{id}.
type Integration struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid" json:"-"`
	ProjectID  uuid.UUID `gorm:"type:uuid" json:"project_id"`
	Provider   string    `json:"provider"`
	Name       string    `json:"name"`
	SecretEnc  []byte    `json:"-"`
	SecretHint string    `json:"secret_hint"`
	Active     bool      `json:"active"`
	CreatedBy  uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName maps to integrations.
func (Integration) TableName() string { return "integrations" }

// IntegrationWithSecret is returned once, when a secret is created or rotated.
type IntegrationWithSecret struct {
	Integration
	Secret string `json:"secret"`
	Path   string `json:"path"`
}

// ExternalLink ties a task to a pull request, branch or commit of a git provider. No titles or messages are stored.
type ExternalLink struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid" json:"-"`
	TaskID     uuid.UUID `gorm:"type:uuid" json:"task_id"`
	Provider   string    `json:"provider"`
	Kind       string    `json:"kind"`
	ExternalID string    `json:"external_id"`
	URL        string    `gorm:"column:url" json:"url"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName maps to external_links.
func (ExternalLink) TableName() string { return "external_links" }
