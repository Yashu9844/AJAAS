package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
)

// Authorizer answers which scope a user holds for resource:action (docs/ARCHITECTURE.md §17); services.AccessResolver
// implements it.
type Authorizer interface {
	ScopeFor(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (access.Scope, error)
}

// ScopeKey is the gin context key under which scoped routes find the caller's scope for the route's permission.
const ScopeKey = "access_scope"

// permissionDecision is the outcome of evaluating resource:action for the authenticated caller.
type permissionDecision struct {
	scope   access.Scope
	status  int    // HTTP status to use when denied
	code    string // error code to use when denied
	message string
}

func denied(status int, code, msg string) permissionDecision {
	return permissionDecision{status: status, code: code, message: msg}
}

// evaluateScope resolves the caller's widest scope for resource:action. The tenant_admin system role (from the
// verified token) holds every permission at tenant scope.
func evaluateScope(c *gin.Context, authz Authorizer, resource, action string) permissionDecision {
	roles, _ := c.Get("roles")
	names, _ := roles.([]string)
	for _, r := range names {
		if r == "tenant_admin" {
			return permissionDecision{scope: access.ScopeTenant}
		}
	}
	tenantID, ok1 := c.Get("tenant_id")
	userID, ok2 := c.Get("user_id")
	tid, ok3 := tenantID.(uuid.UUID)
	uid, ok4 := userID.(uuid.UUID)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return denied(http.StatusForbidden, "FORBIDDEN", "Missing tenant or user context")
	}
	scope, err := authz.ScopeFor(c.Request.Context(), tid, uid, resource, action)
	if err != nil {
		return denied(http.StatusInternalServerError, "DATABASE_ERROR", "Failed to resolve access")
	}
	return permissionDecision{scope: scope}
}

func forbidden() permissionDecision {
	return denied(http.StatusForbidden, "FORBIDDEN", "You do not have permission to execute this action")
}

func abort(c *gin.Context, d permissionDecision) {
	c.AbortWithStatusJSON(d.status, gin.H{"error": gin.H{"code": d.code, "message": d.message}})
}

// RequirePermission admits callers holding resource:action at tenant scope. Routes written before scoped access
// assume tenant-wide reach, so a narrower grant must never pass them.
func RequirePermission(authz Authorizer, resource, action string) gin.HandlerFunc {
	return RequireScopedPermission(authz, resource, action, access.ScopeTenant)
}

// RequireScopedPermission admits callers holding resource:action at minScope or wider and stores the caller's scope
// under ScopeKey; the owning service must then limit the data to that scope.
func RequireScopedPermission(authz Authorizer, resource, action string, minScope access.Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		d := evaluateScope(c, authz, resource, action)
		if d.status != 0 {
			abort(c, d)
			return
		}
		if !access.Covers(d.scope, minScope) {
			abort(c, forbidden())
			return
		}
		c.Set(ScopeKey, d.scope)
		c.Next()
	}
}

// PermissionFlag never blocks: it stores under key whether the caller holds resource:action at tenant scope, so
// handlers can vary behaviour (e.g. unmasked PII) on a finer permission than the route requires.
func PermissionFlag(authz Authorizer, resource, action, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		d := evaluateScope(c, authz, resource, action)
		c.Set(key, d.status == 0 && d.scope == access.ScopeTenant)
		c.Next()
	}
}

// ScopeFrom returns the scope a scoped route stored for the caller (ScopeNone when absent).
func ScopeFrom(c *gin.Context) access.Scope {
	v, _ := c.Get(ScopeKey)
	s, _ := v.(access.Scope)
	return s
}
