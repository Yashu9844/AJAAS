package dto

import "time"

// CreateRoleRequest contains payload to create a new role.
type CreateRoleRequest struct {
	Name          string   `json:"name" binding:"required,min=1,max=100"`
	Description   string   `json:"description" binding:"omitempty,max=500"`
	PermissionIDs []string `json:"permission_ids" binding:"omitempty,dive,uuid"`
}

// UpdateRoleRequest is a partial payload to update role metadata.
type UpdateRoleRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

// AssignRoleRequest holds roles to assign to a user.
type AssignRoleRequest struct {
	RoleIDs []string `json:"role_ids" binding:"required,min=1,dive,uuid"`
}

// AssignPermissionsRequest holds permissions to assign to a role.
type AssignPermissionsRequest struct {
	PermissionIDs []string `json:"permission_ids" binding:"required,min=1,dive,uuid"`
}

// PermissionSummary maps minimal permission metadata inside nested responses.
type PermissionSummary struct {
	ID       string `json:"id"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// RoleResponse represents the serialization structure for role output.
type RoleResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description *string             `gorm:"type:varchar(500)" json:"description,omitempty"`
	IsSystem    bool                `json:"is_system"`
	Permissions []PermissionSummary `json:"permissions,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// RoleListResponse holds paginated RoleResponses.
type RoleListResponse struct {
	Data []RoleResponse `json:"data"`
	Meta PaginationMeta `json:"meta"`
}
