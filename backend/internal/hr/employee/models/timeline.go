package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmployeeTimeline records chronological career, status, and org transitions.
type EmployeeTimeline struct {
	database.TenantBaseModel
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null;index" json:"employee_profile_id"`
	EventType         string    `gorm:"type:varchar(50);not null" json:"event_type"`
	EffectiveDate     time.Time `gorm:"type:date;not null" json:"effective_date"`
	Notes             string    `gorm:"type:text" json:"notes"`
	Metadata          string    `gorm:"type:jsonb" json:"metadata"`
}

func (EmployeeTimeline) TableName() string {
	return "employee_timelines"
}
