package dto

import "time"

// UserGrantRequest grants one permission to one user at a scope, optionally until expires_at.
type UserGrantRequest struct {
	PermissionID string     `json:"permission_id" binding:"required,uuid"`
	Scope        string     `json:"scope" binding:"omitempty,oneof=own direct_reports all_reports department tenant"`
	ExpiresAt    *time.Time `json:"expires_at"`
	Reason       string     `json:"reason" binding:"omitempty,max=500"`
}

// UserBlockRequest removes one permission from one user whatever their roles grant.
type UserBlockRequest struct {
	PermissionID string `json:"permission_id" binding:"required,uuid"`
	Reason       string `json:"reason" binding:"omitempty,max=500"`
}

// UserGrantResponse is a stored per-user grant.
type UserGrantResponse struct {
	PermissionID string     `json:"permission_id"`
	Permission   string     `json:"permission"`
	Scope        string     `json:"scope"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Expired      bool       `json:"expired"`
	Reason       *string    `json:"reason,omitempty"`
	GrantedBy    *string    `json:"granted_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// UserBlockResponse is a stored per-user block.
type UserBlockResponse struct {
	PermissionID string    `json:"permission_id"`
	Permission   string    `json:"permission"`
	Reason       *string   `json:"reason,omitempty"`
	CreatedBy    *string   `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// AccessSource explains one reason a permission is held.
type AccessSource struct {
	Kind     string `json:"kind"` // role | automatic_role | inherited | user_grant
	RoleID   string `json:"role_id,omitempty"`
	RoleName string `json:"role_name,omitempty"`
	Scope    string `json:"scope"`
}

// ExplainedPermission is one effective permission with its scope, sources and whether a block removes it.
type ExplainedPermission struct {
	Permission string         `json:"permission"`
	Module     string         `json:"module,omitempty"`
	Sensitive  bool           `json:"sensitive"`
	Scope      string         `json:"scope"`
	Blocked    bool           `json:"blocked"`
	Sources    []AccessSource `json:"sources"`
}

// ExplainAccessResponse shows a user's effective access and where every permission comes from.
type ExplainAccessResponse struct {
	UserID        string                `json:"user_id"`
	IsTenantAdmin bool                  `json:"is_tenant_admin"`
	Roles         []RoleSummary         `json:"roles"`
	Permissions   []ExplainedPermission `json:"permissions"`
}

// AutomaticRoleSyncResponse reports an automatic-role reconciliation.
type AutomaticRoleSyncResponse struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
}
