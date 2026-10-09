package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/leave/controllers"
)

func guard(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Required", action)
		c.AbortWithStatus(http.StatusTeapot)
	}
}

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	RegisterRoutes(r.Group("/api/v1"), Middleware{TenantResolver: pass, Authenticate: pass, Require: guard},
		Controllers{
			Policy:  controllers.NewPolicyController(nil, nil),
			Balance: controllers.NewBalanceController(nil),
			Request: controllers.NewRequestController(nil),
		})
	return r
}

const anyID = "00000000-0000-0000-0000-000000000001"

func hit(r *gin.Engine, route string) *httptest.ResponseRecorder {
	parts := strings.SplitN(route, " ", 2)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(parts[0], strings.ReplaceAll(parts[1], ":id", anyID), strings.NewReader("{}")))
	return w
}

// TestRoutes_ContractMatchesSpec freezes spec §5: 20 operations and RBAC per endpoint.
func TestRoutes_ContractMatchesSpec(t *testing.T) {
	r := newRouter()
	if n := len(r.Routes()); n != 20 {
		t.Fatalf("expected 20 routes, got %d", n)
	}
	rbac := map[string]string{
		"POST /api/v1/leave/types":                "manage",
		"PATCH /api/v1/leave/types/:id":           "manage",
		"POST /api/v1/leave/types/:id/deactivate": "manage",
		"POST /api/v1/leave/holidays":             "manage",
		"DELETE /api/v1/leave/holidays/:id":       "manage",
		"GET /api/v1/leave/balances":              "read",
		"POST /api/v1/leave/balances/adjust":      "manage",
		"GET /api/v1/leave/ledger":                "read",
		"GET /api/v1/leave/requests":              "read",
		"GET /api/v1/leave/requests/:id":          "read",
		"POST /api/v1/leave/requests/:id/approve": "approve",
		"POST /api/v1/leave/requests/:id/reject":  "approve",
	}
	for route, want := range rbac {
		if w := hit(r, route); w.Code != http.StatusTeapot || w.Header().Get("X-Required") != want {
			t.Errorf("%s: required %q (%d), want %q", route, w.Header().Get("X-Required"), w.Code, want)
		}
	}
}

// Self endpoints and type/holiday reads need authentication only; "me" is not swallowed by ":id".
func TestRoutes_OpenToAuthenticated(t *testing.T) {
	r := newRouter()
	for _, route := range []string{
		"GET /api/v1/leave/types", "GET /api/v1/leave/types/:id", "GET /api/v1/leave/holidays",
		"GET /api/v1/leave/balances/me", "POST /api/v1/leave/requests/preview", "POST /api/v1/leave/requests",
		"GET /api/v1/leave/requests/me", "POST /api/v1/leave/requests/:id/cancel",
	} {
		if w := hit(r, route); w.Code != http.StatusUnauthorized {
			t.Errorf("%s must reach its controller unguarded (401 without actor), got %d %s", route, w.Code, w.Header().Get("X-Required"))
		}
	}
}
