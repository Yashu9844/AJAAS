package dto

import (
	"time"

	"github.com/google/uuid"
)

type UploadDocumentRequest struct {
	DocumentType string `json:"document_type" binding:"required,min=2,max=50"`
	FileName     string `json:"file_name" binding:"required,min=1,max=255"`
	FileURL      string `json:"file_url" binding:"required,url"`
	FileSize     int64  `json:"file_size" binding:"required,gt=0"`
	MimeType     string `json:"mime_type" binding:"required,max=100"`
}

type DocumentResponse struct {
	ID                uuid.UUID  `json:"id"`
	EmployeeProfileID uuid.UUID  `json:"employee_profile_id"`
	DocumentType      string     `json:"document_type"`
	FileName          string     `json:"file_name"`
	FileURL           string     `json:"file_url"`
	FileSize          int64      `json:"file_size"`
	MimeType          string     `json:"mime_type"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	VerifiedBy        *uuid.UUID `json:"verified_by,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}
