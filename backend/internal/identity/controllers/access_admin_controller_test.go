package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// stubAccess implements services.AccessService; err (when set) is returned by every call.
type stubAccess struct{ err error }

func (s stubAccess) ListGrants(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]dto.UserGrantResponse, error) {
	return []dto.UserGrantResponse{{Permission: "users:read"}}, s.err
}
func (s stubAccess) Grant(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, dto.UserGrantRequest) (*dto.UserGrantResponse, error) {
	return &dto.UserGrantResponse{Permission: "users:read"}, s.err
}
func (s stubAccess) RevokeGrant(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return s.err
}
func (s stubAccess) ListBlocks(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]dto.UserBlockResponse, error) {
	return nil, s.err
}
func (s stubAccess) Block(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, dto.UserBlockRequest) (*dto.UserBlockResponse, error) {
	return &dto.UserBlockResponse{}, s.err
}
func (s stubAccess) Unblock(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return s.err
}
func (s stubAccess) Explain(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*dto.ExplainAccessResponse, error) {
	return &dto.ExplainAccessResponse{}, s.err
}

type stubAuto struct{ err error }

func (stubAuto) SetSource(services.AutomaticRoleSource) {}
func (s stubAuto) SyncTenant(context.Context, uuid.UUID) (int, int, error) {
	return 1, 2, s.err
}
func (s stubAuto) SyncUser(context.Context, uuid.UUID, uuid.UUID) error { return s.err }
func (stubAuto) Run(context.Context, time.Duration)                     {}

func accessRouter(svc services.AccessService, auto services.AutomaticRoles, withTenant bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctrl := NewAccessController(nil, svc, auto)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if withTenant {
			c.Set("tenant_id", uuid.New())
			c.Set("user_id", uuid.New())
		}
	})
	r.GET("/users/:id/access", ctrl.Explain)
	r.GET("/me/access", ctrl.ExplainMe)
	r.GET("/users/:id/grants", ctrl.ListGrants)
	r.POST("/users/:id/grants", ctrl.Grant)
	r.DELETE("/users/:id/grants/:permission_id", ctrl.RevokeGrant)
	r.GET("/users/:id/blocks", ctrl.ListBlocks)
	r.POST("/users/:id/blocks", ctrl.Block)
	r.DELETE("/users/:id/blocks/:permission_id", ctrl.Unblock)
	r.POST("/sync", ctrl.SyncAutomaticRoles)
	return r
}

func hit(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAccessControllerRoutes(t *testing.T) {
	u, p := uuid.New().String(), uuid.New().String()
	ok := accessRouter(stubAccess{}, stubAuto{}, true)
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/users/" + u + "/access", "", 200},
		{"GET", "/me/access", "", 200},
		{"GET", "/users/" + u + "/grants", "", 200},
		{"POST", "/users/" + u + "/grants", `{"permission_id":"` + p + `","scope":"own"}`, 201},
		{"POST", "/users/" + u + "/grants", `{"permission_id":"nope"}`, 400},
		{"POST", "/users/" + u + "/grants", `{"permission_id":"` + p + `","scope":"galaxy"}`, 400},
		{"POST", "/users/" + u + "/grants", `{bad`, 400},
		{"DELETE", "/users/" + u + "/grants/" + p, "", 200},
		{"DELETE", "/users/" + u + "/grants/nope", "", 400},
		{"GET", "/users/" + u + "/blocks", "", 200},
		{"POST", "/users/" + u + "/blocks", `{"permission_id":"` + p + `"}`, 201},
		{"DELETE", "/users/" + u + "/blocks/" + p, "", 200},
		{"DELETE", "/users/" + u + "/blocks/nope", "", 400},
		{"GET", "/users/nope/access", "", 400},
		{"POST", "/sync", "", 200},
	}
	for _, c := range cases {
		if w := hit(ok, c.method, c.path, c.body); w.Code != c.want {
			t.Errorf("%s %s: want %d got %d %s", c.method, c.path, c.want, w.Code, w.Body)
		}
	}
	if w := hit(ok, "POST", "/sync", ""); !strings.Contains(w.Body.String(), `"removed":2`) {
		t.Fatalf("sync counts: %s", w.Body)
	}
}

func TestAccessControllerErrorsAndMissingContext(t *testing.T) {
	u, p := uuid.New().String(), uuid.New().String()
	failing := accessRouter(stubAccess{err: sharedErrors.ErrNotFound}, stubAuto{err: errors.New("x")}, true)
	for _, c := range [][3]string{{"GET", "/users/" + u + "/access"}, {"GET", "/users/" + u + "/grants"}, {"DELETE", "/users/" + u + "/grants/" + p},
		{"GET", "/users/" + u + "/blocks"}, {"DELETE", "/users/" + u + "/blocks/" + p}} {
		if w := hit(failing, c[0], c[1], ""); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d", c[0], c[1], w.Code)
		}
	}
	if w := hit(failing, "POST", "/sync", ""); w.Code != http.StatusInternalServerError {
		t.Fatalf("sync error: %d", w.Code)
	}
	bare := accessRouter(stubAccess{}, stubAuto{}, false)
	for _, path := range []string{"/users/" + u + "/access", "/me/access"} {
		if w := hit(bare, "GET", path, ""); w.Code < 400 {
			t.Errorf("%s without tenant context: %d", path, w.Code)
		}
	}
	if w := hit(bare, "POST", "/sync", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("sync without tenant: %d", w.Code)
	}
}
