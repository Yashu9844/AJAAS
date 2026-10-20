package dto

import "time"

// InvitationResponse describes an invitation (never its token).
type InvitationResponse struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Email      string     `json:"email"`
	Status     string     `json:"status"` // pending | accepted | revoked | expired
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// InviteResponse is the result of inviting a user: the invited account and its open invitation.
type InviteResponse struct {
	User       UserResponse        `json:"user"`
	Invitation *InvitationResponse `json:"invitation,omitempty"`
}

// AcceptInvitationRequest sets the invited user's password using the token from the invitation.
type AcceptInvitationRequest struct {
	Token           string `json:"token" binding:"required,max=200"`
	Password        string `json:"password" binding:"required,min=8,max=72"`
	ConfirmPassword string `json:"confirm_password" binding:"required,eqfield=Password"`
}
