package access

import (
	"errors"

	"github.com/google/uuid"
)

// Errors returned when a role is assigned on a resource.
var (
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleNotApplicable = errors.New("role does not apply to this resource")
	ErrResourceType      = errors.New("unsupported resource type")
)

// ResourceRef names one resource of a tenant, or (with a zero ID) every resource of a type.
type ResourceRef struct {
	TenantID uuid.UUID
	Type     string // one of ResourceTypes (AppliesProject, AppliesMeetingSeries)
	ID       uuid.UUID
}

// RoleAssignment gives a user a role on a resource. ActorID is the zero value for a system change.
type RoleAssignment struct {
	ActorID uuid.UUID
	UserID  uuid.UUID
	RoleID  uuid.UUID
	Source  string // manual, group or automatic
}

// ResourceRole is a role held on a resource (UserID set), or an assignable role (UserID zero).
type ResourceRole struct {
	UserID      uuid.UUID
	RoleID      uuid.UUID
	RoleName    string
	TemplateKey string
	Source      string
}
