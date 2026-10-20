package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmployeeDocument holds metadata and verification state of employee documents.
type EmployeeDocument struct {
	database.TenantBaseModel
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;index" json:"employee_profile_id"`
	DocumentType      string     `gorm:"type:varchar(50);not null" json:"document_type"`
	FileName          string     `gorm:"type:varchar(255);not null" json:"file_name"`
	FileURL           string     `gorm:"type:text;not null" json:"file_url"`
	FileSize          int64      `gorm:"type:bigint;not null" json:"file_size"`
	MimeType          string     `gorm:"type:varchar(100);not null" json:"mime_type"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	VerifiedBy        *uuid.UUID `gorm:"type:uuid" json:"verified_by,omitempty"`
}

func (EmployeeDocument) TableName() string {
	return "employee_documents"
}
