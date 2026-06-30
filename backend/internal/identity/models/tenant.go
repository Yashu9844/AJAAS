package models

import (
	"github.com/jaas/jaas/internal/shared/database"
)

// Tenant represents an isolated customer organization.
type Tenant struct {
	database.BaseModel
	Name    string `gorm:"type:varchar(255);not null" json:"name"`
	Slug    string `gorm:"type:varchar(64);uniqueIndex:idx_tenants_slug;not null" json:"slug"`
	Domain  *string `gorm:"type:varchar(255);uniqueIndex:idx_tenants_domain" json:"domain,omitempty"`
	Status  string `gorm:"type:varchar(20);not null;default:'active';index:idx_tenants_status" json:"status"`
	Plan    string `gorm:"type:varchar(50);not null;default:'free'" json:"plan"`

	// Relationships
	Settings []TenantSettings `gorm:"foreignKey:TenantID" json:"settings,omitempty"`
	Users    []User           `gorm:"foreignKey:TenantID" json:"users,omitempty"`
	Roles    []Role           `gorm:"foreignKey:TenantID" json:"roles,omitempty"`
	Sessions []Session        `gorm:"foreignKey:TenantID" json:"sessions,omitempty"`
	Logs     []AuditLog       `gorm:"foreignKey:TenantID" json:"logs,omitempty"`
}
