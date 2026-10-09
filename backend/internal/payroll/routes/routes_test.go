package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/payroll/controllers"
)

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	guard := func(action string) gin.HandlerFunc {
		return func(c *gin.Context) { c.Header("X-Required", action); c.AbortWithStatus(http.StatusTeapot) }
	}
	RegisterRoutes(r.Group("/api/v1"), Middleware{TenantResolver: pass, Authenticate: pass, Require: guard},
		Controllers{Setup: controllers.NewSetupController(nil, nil), Run: controllers.NewRunController(nil, nil)})
	return r
}

func hit(r *gin.Engine, route string) *httptest.ResponseRecorder {
	parts := strings.SplitN(route, " ", 2)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(parts[0], strings.ReplaceAll(parts[1], ":id", "00000000-0000-0000-0000-000000000001"), strings.NewReader("{}")))
	return w
}

// TestRoutes_Contract freezes spec §5: 20 operations, RBAC per endpoint, self endpoints unguarded.
func TestRoutes_Contract(t *testing.T) {
	r := newRouter()
	if n := len(r.Routes()); n != 20 {
		t.Fatalf("want 20 routes, got %d", n)
	}
	guarded := map[string]string{
		"POST /api/v1/payroll/structures": "manage", "GET /api/v1/payroll/structures": "read", "GET /api/v1/payroll/structures/:id": "read",
		"PATCH /api/v1/payroll/structures/:id": "manage", "POST /api/v1/payroll/structures/:id/deactivate": "manage",
		"POST /api/v1/payroll/assignments": "manage", "GET /api/v1/payroll/assignments": "read", "POST /api/v1/payroll/assignments/preview": "manage",
		"POST /api/v1/payroll/runs": "manage", "GET /api/v1/payroll/runs": "read", "GET /api/v1/payroll/runs/:id": "read",
		"POST /api/v1/payroll/runs/:id/calculate": "manage", "POST /api/v1/payroll/runs/:id/approve": "approve",
		"POST /api/v1/payroll/runs/:id/finalize": "approve", "GET /api/v1/payroll/runs/:id/payslips": "read",
		"GET /api/v1/payroll/runs/:id/payout.csv": "read", "GET /api/v1/payroll/payslips/:id": "read",
	}
	for route, want := range guarded {
		if w := hit(r, route); w.Code != http.StatusTeapot || w.Header().Get("X-Required") != want {
			t.Errorf("%s: %q (%d), want %q", route, w.Header().Get("X-Required"), w.Code, want)
		}
	}
	for _, self := range []string{"GET /api/v1/payroll/assignments/me", "GET /api/v1/payroll/payslips/me", "GET /api/v1/payroll/payslips/me/:id"} {
		if w := hit(r, self); w.Code != http.StatusUnauthorized {
			t.Errorf("%s must reach its controller unguarded (401 without actor), got %d", self, w.Code)
		}
	}
}
