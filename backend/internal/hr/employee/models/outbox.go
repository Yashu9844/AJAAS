package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmployeeEventsOutbox provides durable outbox persistence for domain events.
type EmployeeEventsOutbox struct {
	database.TenantBaseModel
	EventID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"event_id"`
	EventType   string     `gorm:"type:varchar(100);not null" json:"event_type"`
	RoutingKey  string     `gorm:"type:varchar(100);not null" json:"routing_key"`
	Payload     string     `gorm:"type:jsonb;not null" json:"payload"`
	Published   bool       `gorm:"not null;default:false;index" json:"published"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	RetryCount  int        `gorm:"not null;default:0" json:"retry_count"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
}

func (EmployeeEventsOutbox) TableName() string {
	return "employee_events_outbox"
}
