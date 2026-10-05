package dto

import "time"

// CreateDesignationRequest creates a job title/level. FR-DG001.
type CreateDesignationRequest struct {
	Title       string  `json:"title" binding:"required,min=1,max=100"`
	Code        *string `json:"code" binding:"omitempty,min=2,max=32"`
	Level       *int    `json:"level" binding:"omitempty,min=1"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

// UpdateDesignationRequest patches a designation; code is immutable and ignored.
type UpdateDesignationRequest struct {
	Title       *string `json:"title" binding:"omitempty,min=1,max=100"`
	Level       *int    `json:"level" binding:"omitempty,min=1"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

// DesignationResponse serializes a designation.
type DesignationResponse struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Title       string    `json:"title"`
	Code        *string   `json:"code,omitempty"`
	Level       *int      `json:"level,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DesignationListResponse is a paginated designation list.
type DesignationListResponse struct {
	Data []DesignationResponse `json:"data"`
	Meta PaginationMeta        `json:"meta"`
}
