// Package identityport is M1 HR Management's only view of M0 Identity & Access (docs/modules/m1-hr-management/
// design.md §4). HR packages depend on these interfaces, never on internal/identity/... packages (enforced by the
// import-boundary test). The in-process adapter lives in identityport/inprocess; a JWT-claims + HTTP adapter replaces
// it when HR is deployed separately. The permission catalogue types come from the published contract package
// internal/identity/access.
package identityport

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"gorm.io/gorm"
)

// User is HR's view of a login identity.
type User struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Email    string
	Status   string // active | invited | inactive
}

// Active reports whether the user can sign in.
func (u *User) Active() bool { return u != nil && u.Status == "active" }

// Invite asks M0 to create an invited user and send them an invitation.
type Invite struct {
	Email     string
	FirstName string
	LastName  string
}

// Users reads and changes login identities.
type Users interface {
	// GetByID returns the user, or nil when it does not exist in the tenant.
	GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*User, error)
	DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id, correlationID uuid.UUID) error
	// ActivateUser re-enables a deactivated login (HR/Core rehire and rescinded termination).
	ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in Invite, correlationID uuid.UUID) (*User, error)
}

// Types shared with M0 (defined in the published contract package).
type (
	// ResourceRef names one resource of a tenant.
	ResourceRef = access.ResourceRef
	// RoleAssignment gives a user a role on a resource.
	RoleAssignment = access.RoleAssignment
	// ResourceRole is a role held on a resource, or an assignable role.
	ResourceRole = access.ResourceRole
)

// ResourceAccess is M0's view of roles assigned on a resource (docs/modules/m0-identity-access/resource-roles.md).
// Write methods run inside the caller's transaction so a membership change and its role commit together.
type ResourceAccess interface {
	// PermissionsOn returns the role and the permission keys the user holds on the resource through it (zero role: none).
	PermissionsOn(ctx context.Context, db *gorm.DB, ref ResourceRef, userID uuid.UUID) (ResourceRole, []string, error)
	// RolesFor maps each resource id to the role the user holds on it (ref.ID is ignored).
	RolesFor(ctx context.Context, db *gorm.DB, ref ResourceRef, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ResourceRole, error)
	AssignRole(ctx context.Context, tx *gorm.DB, ref ResourceRef, a RoleAssignment) error
	RevokeRole(ctx context.Context, tx *gorm.DB, ref ResourceRef, actorID, userID uuid.UUID) error
	RolesOn(ctx context.Context, db *gorm.DB, ref ResourceRef) ([]ResourceRole, error)
	// AvailableRoles lists the roles of the tenant that can be assigned on the resource type.
	AvailableRoles(ctx context.Context, db *gorm.DB, ref ResourceRef) ([]ResourceRole, error)
	RoleByTemplate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, templateKey string) (uuid.UUID, error)
	CountWithTemplate(ctx context.Context, db *gorm.DB, ref ResourceRef, templateKey string) (int64, error)
}

// Auditor writes audit records (HR keeps using M0's audit trail until hr.audit_events exists).
type Auditor interface {
	Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

// HTTP provides the request guards HR routes use.
type HTTP interface {
	// TenantResolver resolves the tenant from the host; Authenticate verifies the access token.
	TenantResolver() gin.HandlerFunc
	Authenticate() gin.HandlerFunc
	// Require admits callers holding resource:action at tenant scope.
	Require(resource, action string) gin.HandlerFunc
	// RequireScoped admits callers holding resource:action at minScope or wider and exposes their scope.
	RequireScoped(resource, action string, minScope access.Scope) gin.HandlerFunc
	// Flag stores under key whether the caller holds resource:action at tenant scope (never blocks).
	Flag(resource, action, key string) gin.HandlerFunc
	// AuditTrail records a successful mutation after the handler ran.
	AuditTrail(action, resource string) gin.HandlerFunc
}

// DeactivationHook converges HR data when a login identity is deactivated, inside the same transaction.
type DeactivationHook interface {
	DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, correlationID uuid.UUID) error
}

// RoleFacts tells M0 who currently qualifies for the automatic Employee and Manager roles (docs/ARCHITECTURE.md §17).
type RoleFacts interface {
	Members(ctx context.Context, tenantID uuid.UUID) (employees, managers []uuid.UUID, err error)
	MemberOf(ctx context.Context, tenantID, userID uuid.UUID) (employee, manager bool, err error)
}

// Port is everything HR needs from M0.
type Port interface {
	HTTP
	Users() Users
	Audit() Auditor
	// Resources manages roles assigned on resources such as projects.
	Resources() ResourceAccess
	// RegisterPermissions registers HR permission definitions (with predefined-role defaults) in M0's catalogue.
	RegisterPermissions(ctx context.Context, defs []access.PermissionDef) error
	// OnUserDeactivated registers a hook run when M0 deactivates a user.
	OnUserDeactivated(h DeactivationHook)
	// ProvideRoleFacts installs HR as the source of the automatic Employee and Manager roles.
	ProvideRoleFacts(f RoleFacts)
}

// Errors returned when assigning a role on a resource.
var (
	ErrRoleNotFound      = access.ErrRoleNotFound
	ErrRoleNotApplicable = access.ErrRoleNotApplicable
)
