package constants

import "time"

// Tenant/User status values.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusInactive  = "inactive"
	StatusInvited   = "invited"
)

// Security and hashing configurations.
const (
	BcryptCost = 12
)

// Default Token Expriations.
const (
	DefaultAccessTokenTTL  = 15 * time.Minute
	DefaultRefreshTokenTTL = 7 * 24 * time.Hour
	PasswordResetTokenTTL  = 1 * time.Hour
)

// Pagination defaults.
const (
	DefaultPage     = 1
	DefaultPerPage  = 20
	MaxPerPage      = 100
)

// System Roles defined globally.
const (
	RoleTenantAdmin = "tenant_admin"
	RoleMember      = "member"
)
