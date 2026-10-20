package dto

import "time"

// GrantInput is a permission granted at a scope (docs/ARCHITECTURE.md §17); an empty scope means tenant.
type GrantInput struct {
	PermissionID string `json:"permission_id" binding:"required,uuid"`
	Scope        string `json:"scope" binding:"omitempty,oneof=own direct_reports all_reports department tenant"`
}

// CreateRoleRequest contains payload to create a new role. permission_ids are granted at tenant scope; grants carry
// explicit scopes. parent_role_id makes the role inherit every grant of the parent.
type CreateRoleRequest struct {
	Name          string       `json:"name" binding:"required,min=1,max=100"`
	Description   string       `json:"description" binding:"omitempty,max=500"`
	PermissionIDs []string     `json:"permission_ids" binding:"omitempty,dive,uuid"`
	Grants        []GrantInput `json:"grants" binding:"omitempty,dive"`
	ParentRoleID  *string      `json:"parent_role_id" binding:"omitempty,uuid"`
}

// UpdateRoleRequest is a partial payload to update role metadata. parent_role_id "" removes the parent.
type UpdateRoleRequest struct {
	Name         *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description  *string `json:"description" binding:"omitempty,max=500"`
	ParentRoleID *string `json:"parent_role_id" binding:"omitempty,max=36"`
}

// CloneRoleRequest names the copy of a role.
type CloneRoleRequest struct {
	Name string `json:"name" binding:"required,min=1,max=100"`
}

// AssignRoleRequest holds roles to assign to a user.
type AssignRoleRequest struct {
	RoleIDs []string `json:"role_ids" binding:"required,min=1,dive,uuid"`
}

// AssignPermissionsRequest holds permissions to assign to a role (at least one of permission_ids or grants).
// Assigning a permission the role already holds changes its scope.
type AssignPermissionsRequest struct {
	PermissionIDs []string     `json:"permission_ids" binding:"omitempty,dive,uuid"`
	Grants        []GrantInput `json:"grants" binding:"omitempty,dive"`
}

// PermissionSummary maps minimal permission metadata inside nested responses.
type PermissionSummary struct {
	ID       string `json:"id"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Scope    string `json:"scope,omitempty"`
}

// RoleResponse represents the serialization structure for role output.
type RoleResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `gorm:"type:varchar(500)" json:"description,omitempty"`
	IsSystem    bool    `json:"is_system"`
	// ParentRoleID is the role this one inherits from; TemplateKey marks a predefined role.
	ParentRoleID    *string             `json:"parent_role_id,omitempty"`
	TemplateKey     *string             `json:"template_key,omitempty"`
	IsAutomatic     bool                `json:"is_automatic"`
	TemplateManaged bool                `json:"template_managed"`
	Permissions     []PermissionSummary `json:"permissions,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// RoleListResponse holds paginated RoleResponses.
type RoleListResponse struct {
	Data []RoleResponse `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// AccessResponse describes what the authenticated caller may do (roles + effective permissions).
type AccessResponse struct {
	UserID        string        `json:"user_id"`
	Roles         []RoleSummary `json:"roles"`
	Permissions   []string      `json:"permissions"` // "resource:action"
	Grants        []ScopedGrant `json:"grants"`      // permissions with the widest scope held
	IsTenantAdmin bool          `json:"is_tenant_admin"`
}

// ScopedGrant is a held permission and its scope.
type ScopedGrant struct {
	Permission string `json:"permission"`
	Scope      string `json:"scope"`
}
