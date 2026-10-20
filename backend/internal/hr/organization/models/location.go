package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Location is a physical work site of a tenant: address, state (professional tax, holidays) and time zone.
// FR-L001..FR-L007; SQL twin: migrations/hr/000047.
type Location struct {
	database.BaseModel
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index:idx_locations_tenant_status,priority:1;uniqueIndex:uq_locations_tenant_code,priority:1" json:"tenant_id"`
	Code         string    `gorm:"type:varchar(32);not null;uniqueIndex:uq_locations_tenant_code,priority:2" json:"code"`
	Name         string    `gorm:"type:varchar(150);not null" json:"name"`
	AddressLine1 *string   `gorm:"type:varchar(200)" json:"address_line1,omitempty"`
	AddressLine2 *string   `gorm:"type:varchar(200)" json:"address_line2,omitempty"`
	City         string    `gorm:"type:varchar(100);not null" json:"city"`
	StateCode    string    `gorm:"type:varchar(10);not null" json:"state_code"`
	CountryCode  string    `gorm:"type:char(2);not null;default:'IN'" json:"country_code"`
	PostalCode   *string   `gorm:"type:varchar(20)" json:"postal_code,omitempty"`
	TimeZone     string    `gorm:"type:varchar(64);not null;default:'Asia/Kolkata'" json:"time_zone"`
	Status       string    `gorm:"type:varchar(20);not null;default:'active';index:idx_locations_tenant_status,priority:2" json:"status"`
}

// BeforeCreate generates a UUID v4 key when unset.
func (l *Location) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}
