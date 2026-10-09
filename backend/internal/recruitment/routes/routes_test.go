package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/recruitment/controllers"
)

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	guard := func(action string) gin.HandlerFunc {
		return func(c *gin.Context) { c.Header("X-Required", action); c.AbortWithStatus(http.StatusTeapot) }
	}
	RegisterRoutes(r.Group("/api/v1"), Middleware{TenantResolver: pass, Authenticate: pass, Require: guard},
		controllers.New(controllers.Services{}))
	return r
}

func hit(r *gin.Engine, route string) *httptest.ResponseRecorder {
	parts := strings.SplitN(route, " ", 2)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(parts[0], strings.ReplaceAll(parts[1], ":id", "00000000-0000-0000-0000-000000000001"), strings.NewReader("{}")))
	return w
}

// TestRoutes_Contract freezes spec section 5: 20 operations, RBAC per endpoint, interviewer self endpoints unguarded.
func TestRoutes_Contract(t *testing.T) {
	r := newRouter()
	if n := len(r.Routes()); n != 20 {
		t.Fatalf("want 20 routes, got %d", n)
	}
	b := "/api/v1/recruitment"
	guarded := map[string]string{
		"POST " + b + "/jobs": "manage", "GET " + b + "/jobs": "read", "GET " + b + "/jobs/:id": "read",
		"PATCH " + b + "/jobs/:id": "manage", "POST " + b + "/jobs/:id/status": "manage",
		"POST " + b + "/candidates": "manage", "GET " + b + "/candidates": "read", "GET " + b + "/candidates/:id": "read",
		"PATCH " + b + "/candidates/:id": "manage", "POST " + b + "/candidates/:id/stage": "manage",
		"POST " + b + "/candidates/:id/hire": "hire",
		"POST " + b + "/interviews":          "manage", "GET " + b + "/interviews": "read", "POST " + b + "/interviews/:id/cancel": "manage",
		"POST " + b + "/offers": "manage", "GET " + b + "/offers": "read", "POST " + b + "/offers/:id/decision": "manage",
		"POST " + b + "/offers/:id/withdraw": "manage",
	}
	for route, want := range guarded {
		if w := hit(r, route); w.Code != http.StatusTeapot || w.Header().Get("X-Required") != want {
			t.Errorf("%s: %q (%d), want %q", route, w.Header().Get("X-Required"), w.Code, want)
		}
	}
	for _, self := range []string{"GET " + b + "/interviews/me", "POST " + b + "/interviews/:id/feedback"} {
		if w := hit(r, self); w.Code != http.StatusUnauthorized {
			t.Errorf("%s must reach its controller unguarded (401 without actor), got %d", self, w.Code)
		}
	}
}
