package dto

import (
	"time"

	"github.com/google/uuid"
)

type TimelineResponse struct {
	ID                uuid.UUID `json:"id"`
	EmployeeProfileID uuid.UUID `json:"employee_profile_id"`
	EventType         string    `json:"event_type"`
	EffectiveDate     time.Time `json:"effective_date"`
	Notes             string    `json:"notes,omitempty"`
	Metadata          string    `json:"metadata,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}
