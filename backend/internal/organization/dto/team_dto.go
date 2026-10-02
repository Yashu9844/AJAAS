package dto

import "time"

// CreateTeamRequest creates a team inside a department. FR-T001.
type CreateTeamRequest struct {
	Name         string  `json:"name" binding:"required,min=1,max=100"`
	Code         *string `json:"code" binding:"omitempty,min=2,max=32"`
	Description  *string `json:"description" binding:"omitempty,max=500"`
	DepartmentID string  `json:"department_id" binding:"required,uuid"`
	LeadUserID   *string `json:"lead_user_id" binding:"omitempty,uuid"`
}

// UpdateTeamRequest patches a team; code is immutable and ignored. FR-T005.
type UpdateTeamRequest struct {
	Name         *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description  *string `json:"description" binding:"omitempty,max=500"`
	DepartmentID *string `json:"department_id" binding:"omitempty,uuid"`
	LeadUserID   *string `json:"lead_user_id" binding:"omitempty,uuid"`
}

// TeamResponse serializes a team with member count.
type TeamResponse struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	DepartmentID string    `json:"department_id"`
	Name         string    `json:"name"`
	Code         *string   `json:"code,omitempty"`
	Description  *string   `json:"description,omitempty"`
	LeadUserID   *string   `json:"lead_user_id,omitempty"`
	Status       string    `json:"status"`
	MemberCount  int64     `json:"member_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TeamListResponse is a paginated team list.
type TeamListResponse struct {
	Data []TeamResponse `json:"data"`
	Meta PaginationMeta `json:"meta"`
}
