package dto

import "time"

// CreateMappingRequest maps a user into org structure. FR-M001.
type CreateMappingRequest struct {
	UserID        string  `json:"user_id" binding:"required,uuid"`
	DepartmentID  *string `json:"department_id" binding:"omitempty,uuid"`
	TeamID        *string `json:"team_id" binding:"omitempty,uuid"`
	DesignationID *string `json:"designation_id" binding:"omitempty,uuid"`
	IsPrimary     bool    `json:"is_primary"`
	ManagerUserID *string `json:"manager_user_id" binding:"omitempty,uuid"`
}

// UpdateMappingRequest patches a mapping (reassignment, primary swap, manager change).
type UpdateMappingRequest struct {
	DepartmentID  *string `json:"department_id" binding:"omitempty,uuid"`
	TeamID        *string `json:"team_id" binding:"omitempty,uuid"`
	DesignationID *string `json:"designation_id" binding:"omitempty,uuid"`
	IsPrimary     *bool   `json:"is_primary"`
	ManagerUserID *string `json:"manager_user_id" binding:"omitempty,uuid"`
}

// DeactivateMappingRequest deactivates with a reason (append-only, FR-M007).
type DeactivateMappingRequest struct {
	Reason *string `json:"reason" binding:"omitempty,max=500"`
}

// MappingResponse serializes a user→org mapping (IDs only, no PII).
type MappingResponse struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	UserID        string    `json:"user_id"`
	DepartmentID  *string   `json:"department_id,omitempty"`
	TeamID        *string   `json:"team_id,omitempty"`
	DesignationID *string   `json:"designation_id,omitempty"`
	IsPrimary     bool      `json:"is_primary"`
	ManagerUserID *string   `json:"manager_user_id,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MappingListResponse is a paginated mapping list.
type MappingListResponse struct {
	Data []MappingResponse `json:"data"`
	Meta PaginationMeta    `json:"meta"`
}
