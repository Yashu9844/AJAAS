package dto

import "time"

// CreateTenantRequest contains payload to register a new tenant organization.
type CreateTenantRequest struct {
	Name   string `json:"name" binding:"required,min=1,max=255"`
	Slug   string `json:"slug" binding:"required,min=3,max=64"`
	Domain string `json:"domain" binding:"omitempty,max=255,hostname_rfc1123"`
	Plan   string `json:"plan" binding:"omitempty,max=50"`
	// Admin, when present, provisions the tenant's first tenant_admin user atomically with the tenant.
	Admin *BootstrapAdminRequest `json:"admin" binding:"omitempty"`
}

// BootstrapAdminRequest describes the first administrator created together with a tenant.
type BootstrapAdminRequest struct {
	Email     string `json:"email" binding:"required,email,max=255"`
	Password  string `json:"password" binding:"required,min=8,max=72"`
	FirstName string `json:"first_name" binding:"required,min=1,max=100"`
	LastName  string `json:"last_name" binding:"required,min=1,max=100"`
}

// UpdateTenantRequest contains payload to update tenant fields.
type UpdateTenantRequest struct {
	Name   *string `json:"name" binding:"omitempty,min=1,max=255"`
	Domain *string `json:"domain" binding:"omitempty,max=255,hostname_rfc1123"`
	Plan   *string `json:"plan" binding:"omitempty,max=50"`
}

// TenantResponse represents serialized tenant details in API responses.
type TenantResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Domain    *string   `json:"domain,omitempty"`
	Status    string    `json:"status"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// AdminUserID is set only on create when an initial admin was provisioned.
	AdminUserID *string `json:"admin_user_id,omitempty"`
}

// TenantListResponse represents a paginated list of tenants.
type TenantListResponse struct {
	Data []TenantResponse `json:"data"`
	Meta PaginationMeta   `json:"meta"`
}
