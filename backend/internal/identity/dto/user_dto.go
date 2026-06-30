package dto

import "time"

// CreateUserRequest contains payload to create a new user profile.
type CreateUserRequest struct {
	Email     string   `json:"email" binding:"required,email,max=255"`
	Password  string   `json:"password" binding:"required,min=8,max=72"`
	FirstName string   `json:"first_name" binding:"required,min=1,max=100"`
	LastName  string   `json:"last_name" binding:"required,min=1,max=100"`
	Phone     string   `json:"phone" binding:"omitempty,max=20"`
	RoleIDs   []string `json:"role_ids" binding:"omitempty,dive,uuid"`
}

// UpdateUserRequest is a partial payload to update user profile.
type UpdateUserRequest struct {
	FirstName *string `json:"first_name" binding:"omitempty,min=1,max=100"`
	LastName  *string `json:"last_name" binding:"omitempty,min=1,max=100"`
	Phone     *string `json:"phone" binding:"omitempty,max=20"`
	AvatarURL *string `json:"avatar_url" binding:"omitempty,max=500,url"`
}

// InviteUserRequest contains payload to invite a new user.
type InviteUserRequest struct {
	Email     string   `json:"email" binding:"required,email,max=255"`
	FirstName string   `json:"first_name" binding:"required,min=1,max=100"`
	LastName  string   `json:"last_name" binding:"required,min=1,max=100"`
	RoleIDs   []string `json:"role_ids" binding:"omitempty,dive,uuid"`
}

// UserResponse represents user profile output payload.
type UserResponse struct {
	ID              string        `json:"id"`
	Email           string        `json:"email"`
	FirstName       string        `json:"first_name"`
	LastName        string        `json:"last_name"`
	Phone           *string       `json:"phone,omitempty"`
	AvatarURL       *string       `json:"avatar_url,omitempty"`
	Status          string        `json:"status"`
	EmailVerifiedAt *time.Time    `json:"email_verified_at,omitempty"`
	LastLoginAt     *time.Time    `json:"last_login_at,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	Roles           []RoleSummary `json:"roles"`
}

// UserListResponse holds a paginated array of UserResponses.
type UserListResponse struct {
	Data []UserResponse `json:"data"`
	Meta PaginationMeta `json:"meta"`
}
