package dto

import "time"

// LoginRequest contains payload for user authentication.
type LoginRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
	TenantSlug string `json:"tenant_slug" binding:"required"`
}

// UserLoginSummary maps basic user profile inside a login response.
type UserLoginSummary struct {
	ID        string        `json:"id"`
	Email     string        `json:"email"`
	FirstName string        `json:"first_name"`
	LastName  string        `json:"last_name"`
	Status    string        `json:"status"`
	Roles     []RoleSummary `json:"roles"`
}

// RoleSummary maps roles in user details response.
type RoleSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LoginResponse contains response payload after successful login.
type LoginResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresAt    time.Time        `json:"expires_at"`
	User         UserLoginSummary `json:"user"`
}

// RefreshTokenRequest contains payload to swap refresh token for new tokens.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshTokenResponse holds rotated token credentials.
type RefreshTokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ForgotPasswordRequest triggers reset email generation.
type ForgotPasswordRequest struct {
	Email      string `json:"email" binding:"required,email"`
	TenantSlug string `json:"tenant_slug" binding:"required"`
}

// ResetPasswordRequest consumes reset token to update password.
type ResetPasswordRequest struct {
	Token           string `json:"token" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
	ConfirmPassword string `json:"confirm_password" binding:"required,eqfield=NewPassword"`
}

// MessageResponse is a generic success feedback envelope.
type MessageResponse struct {
	Message string `json:"message"`
}
