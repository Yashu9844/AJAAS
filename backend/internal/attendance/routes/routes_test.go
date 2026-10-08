package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance/controllers"
)

// guard records which permission each route demanded, then aborts before any controller runs.
func guard(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Required", action)
		c.AbortWithStatus(http.StatusTeapot)
	}
}

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	RegisterRoutes(r.Group("/api/v1"), Middleware{TenantResolver: pass, Authenticate: pass, Require: guard, PunchLimit: pass},
		Controllers{
			Attendance:     controllers.NewAttendanceController(nil, nil),
			Regularization: controllers.NewRegularizationController(nil),
			Shift:          controllers.NewShiftController(nil, nil),
		})
	return r
}

// TestRoutes_ContractMatchesSpec freezes spec §5: 19 routes, no Gin conflicts, RBAC per endpoint.
func TestRoutes_ContractMatchesSpec(t *testing.T) {
	r := newRouter(t)
	if n := len(r.Routes()); n != 19 {
		t.Fatalf("expected 19 routes, got %d", n)
	}
	rbac := map[string]string{
		"GET /api/v1/attendance/records":                      "read",
		"GET /api/v1/attendance/records/:id":                  "read",
		"GET /api/v1/attendance/summary":                      "read",
		"GET /api/v1/attendance/regularizations":              "read",
		"POST /api/v1/attendance/regularizations/:id/approve": "approve",
		"POST /api/v1/attendance/regularizations/:id/reject":  "approve",
		"POST /api/v1/shifts":                                 "manage",
		"GET /api/v1/shifts":                                  "read",
		"GET /api/v1/shifts/:id":                              "read",
		"PATCH /api/v1/shifts/:id":                            "manage",
		"POST /api/v1/shifts/:id/deactivate":                  "manage",
		"POST /api/v1/shifts/:id/assignments":                 "manage",
		"GET /api/v1/shift-assignments":                       "read",
	}
	for route, want := range rbac {
		parts := strings.SplitN(route, " ", 2)
		path := strings.ReplaceAll(parts[1], ":id", "00000000-0000-0000-0000-000000000001")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(parts[0], path, nil))
		if w.Code != http.StatusTeapot || w.Header().Get("X-Required") != want {
			t.Errorf("%s: required %q (status %d), want %q", route, w.Header().Get("X-Required"), w.Code, want)
		}
	}
}

// Self endpoints carry no permission guard (security.md) and "me" is not swallowed by ":id".
func TestRoutes_SelfEndpointsHaveNoPermissionGuard(t *testing.T) {
	r := newRouter(t)
	self := map[string]string{
		"POST /api/v1/attendance/punch":                      "Punch",
		"GET /api/v1/attendance/me/today":                    "Today",
		"GET /api/v1/attendance/me":                          "Mine",
		"POST /api/v1/attendance/regularizations":            "Create",
		"GET /api/v1/attendance/regularizations/me":          "ListMine",
		"POST /api/v1/attendance/regularizations/:id/cancel": "Cancel",
	}
	found := map[string]string{}
	for _, rt := range r.Routes() {
		found[rt.Method+" "+rt.Path] = rt.Handler
	}
	for route, handler := range self {
		parts := strings.SplitN(route, " ", 2)
		path := strings.ReplaceAll(parts[1], ":id", "00000000-0000-0000-0000-000000000001")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(parts[0], path, strings.NewReader("{}")))
		if w.Code == http.StatusTeapot {
			t.Errorf("%s must not require attendance:%s", route, w.Header().Get("X-Required"))
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s must reach its controller (401 without actor), got %d", route, w.Code)
		}
		if !strings.HasSuffix(found[route], handler+"-fm") {
			t.Errorf("%s handled by %q, want %s", route, found[route], handler)
		}
	}
}
