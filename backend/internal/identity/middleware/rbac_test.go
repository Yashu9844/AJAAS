package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
)

// fakeAuthz returns a fixed scope per permission key.
type fakeAuthz struct {
	scopes map[string]access.Scope
	err    error
	calls  int
}

func (f *fakeAuthz) ScopeFor(_ context.Context, _, _ uuid.UUID, resource, action string) (access.Scope, error) {
	f.calls++
	return f.scopes[access.Key(resource, action)], f.err
}

func serve(t *testing.T, roles []string, withIDs bool, h ...gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	setup := func(c *gin.Context) {
		if withIDs {
			c.Set("tenant_id", uuid.New())
			c.Set("user_id", uuid.New())
		}
		c.Set("roles", roles)
	}
	handlers := append([]gin.HandlerFunc{setup}, h...)
	handlers = append(handlers, func(c *gin.Context) { c.String(http.StatusOK, string(ScopeFrom(c))) })
	r.GET("/x", handlers...)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w
}

func TestRequirePermissionNeedsTenantScope(t *testing.T) {
	authz := &fakeAuthz{scopes: map[string]access.Scope{"users:read": access.ScopeTenant, "leave:approve": access.ScopeDirectReports}}
	if w := serve(t, []string{"member"}, true, RequirePermission(authz, "users", "read")); w.Code != http.StatusOK || w.Body.String() != "tenant" {
		t.Fatalf("tenant scope passes: %d %s", w.Code, w.Body)
	}
	if w := serve(t, []string{"member"}, true, RequirePermission(authz, "leave", "approve")); w.Code != http.StatusForbidden {
		t.Fatalf("a narrower grant never passes a legacy route: %d", w.Code)
	}
	if w := serve(t, []string{"member"}, true, RequirePermission(authz, "payroll", "read")); w.Code != http.StatusForbidden {
		t.Fatalf("not held: %d", w.Code)
	}
}

func TestRequireScopedPermission(t *testing.T) {
	authz := &fakeAuthz{scopes: map[string]access.Scope{"leave:approve": access.ScopeAllReports}}
	if w := serve(t, nil, true, RequireScopedPermission(authz, "leave", "approve", access.ScopeDirectReports)); w.Code != http.StatusOK || w.Body.String() != "all_reports" {
		t.Fatalf("wider than the minimum passes and exposes the scope: %d %s", w.Code, w.Body)
	}
	if w := serve(t, nil, true, RequireScopedPermission(authz, "leave", "approve", access.ScopeDepartment)); w.Code != http.StatusForbidden {
		t.Fatalf("narrower than the minimum: %d", w.Code)
	}
}

func TestTenantAdminBypassAndFailures(t *testing.T) {
	authz := &fakeAuthz{}
	if w := serve(t, []string{"tenant_admin"}, false, RequirePermission(authz, "anything", "goes")); w.Code != http.StatusOK || authz.calls != 0 {
		t.Fatalf("tenant_admin bypasses without a lookup: %d", w.Code)
	}
	if w := serve(t, []string{"member"}, false, RequirePermission(authz, "users", "read")); w.Code != http.StatusForbidden {
		t.Fatalf("missing ids: %d", w.Code)
	}
	failing := &fakeAuthz{err: errors.New("db down")}
	if w := serve(t, nil, true, RequirePermission(failing, "users", "read")); w.Code != http.StatusInternalServerError {
		t.Fatalf("resolver error: %d", w.Code)
	}
}

func TestPermissionFlag(t *testing.T) {
	authz := &fakeAuthz{scopes: map[string]access.Scope{"employee:read_sensitive": access.ScopeTenant, "employee:admin": access.ScopeOwn}}
	read := func(key string) gin.HandlerFunc {
		return func(c *gin.Context) {
			if v, _ := c.Get(key); v != true {
				c.AbortWithStatus(http.StatusTeapot)
			}
		}
	}
	if w := serve(t, nil, true, PermissionFlag(authz, "employee", "read_sensitive", "k"), read("k")); w.Code != http.StatusOK {
		t.Fatalf("flag set: %d", w.Code)
	}
	if w := serve(t, nil, true, PermissionFlag(authz, "employee", "admin", "k"), read("k")); w.Code != http.StatusTeapot {
		t.Fatalf("own scope does not set a tenant flag: %d", w.Code)
	}
}
