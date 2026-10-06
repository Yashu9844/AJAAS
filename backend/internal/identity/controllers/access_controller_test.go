package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

type accessFixture struct {
	tenant, user uuid.UUID
	users        *MockUserService
	roles        *MockRoleService
	router       *gin.Engine
}

func newAccessFixture(callerIsTarget bool) *accessFixture {
	gin.SetMode(gin.TestMode)
	f := &accessFixture{tenant: uuid.New(), user: uuid.New(), users: &MockUserService{}, roles: &MockRoleService{}}
	uc, rc := NewUserController(nil, f.users), NewRoleController(nil, f.roles)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", f.tenant)
		if callerIsTarget {
			c.Set("user_id", f.user)
		} else {
			c.Set("user_id", uuid.New())
		}
	})
	r.POST("/users/:id/activate", uc.Activate)
	r.POST("/users/:id/deactivate", uc.Deactivate)
	r.DELETE("/users/:id/roles/:role_id", rc.RemoveRoleFromUser)
	r.DELETE("/roles/:id/permissions/:permission_id", rc.RemovePermission)
	r.GET("/auth/me", rc.Me)
	f.router = r
	return f
}

func (f *accessFixture) do(method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, nil)
	f.router.ServeHTTP(w, req)
	return w
}

func TestUserController_ActivateAndSelfDeactivationGuard(t *testing.T) {
	f := newAccessFixture(false)
	if w := f.do("POST", "/users/"+f.user.String()+"/activate"); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"active"`) {
		t.Fatalf("activate: %d %s", w.Code, w.Body.String())
	}
	if w := f.do("POST", "/users/nope/activate"); w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("bad id: %d %s", w.Code, w.Body.String())
	}
	if w := f.do("POST", "/users/"+f.user.String()+"/deactivate"); w.Code != 200 {
		t.Fatalf("deactivating someone else: %d", w.Code)
	}

	self := newAccessFixture(true)
	if w := self.do("POST", "/users/"+self.user.String()+"/deactivate"); w.Code != 409 || !strings.Contains(w.Body.String(), "own account") {
		t.Fatalf("self deactivation must be refused: %d %s", w.Code, w.Body.String())
	}
}

func TestRoleController_RemoveAndMe(t *testing.T) {
	f := newAccessFixture(true)
	role, perm := uuid.New().String(), uuid.New().String()
	if w := f.do("DELETE", "/users/"+f.user.String()+"/roles/"+role); w.Code != 200 {
		t.Fatalf("remove role: %d %s", w.Code, w.Body.String())
	}
	if w := f.do("DELETE", "/users/"+f.user.String()+"/roles/bad"); w.Code != 400 {
		t.Fatalf("bad role id: %d", w.Code)
	}
	if w := f.do("DELETE", "/users/bad/roles/"+role); w.Code != 400 {
		t.Fatalf("bad user id: %d", w.Code)
	}
	if w := f.do("DELETE", "/roles/"+role+"/permissions/"+perm); w.Code != 200 {
		t.Fatalf("remove permission: %d", w.Code)
	}
	if w := f.do("DELETE", "/roles/bad/permissions/"+perm); w.Code != 400 {
		t.Fatalf("bad role id: %d", w.Code)
	}
	if w := f.do("DELETE", "/roles/"+role+"/permissions/bad"); w.Code != 400 {
		t.Fatalf("bad permission id: %d", w.Code)
	}
	if w := f.do("GET", "/auth/me"); w.Code != 200 || !strings.Contains(w.Body.String(), f.user.String()) {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
}

// errRoleService fails the access-management calls with a NOT_FOUND AppError.
type errRoleService struct{ MockRoleService }

func (errRoleService) RemovePermission(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return sharedErrors.ErrNotFound
}
func (errRoleService) RemoveRoleFromUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return sharedErrors.ErrNotFound
}
func (errRoleService) GetMyAccess(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*dto.AccessResponse, error) {
	return nil, sharedErrors.ErrNotFound
}

func TestRoleController_ServiceErrorsMapToEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant, user := uuid.New(), uuid.New()
	rc := NewRoleController(nil, &errRoleService{})
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", tenant); c.Set("user_id", user) })
	r.DELETE("/roles/:id/permissions/:permission_id", rc.RemovePermission)
	r.DELETE("/users/:id/roles/:role_id", rc.RemoveRoleFromUser)
	r.GET("/auth/me", rc.Me)
	for _, c := range []struct{ method, path string }{
		{"DELETE", "/roles/" + uuid.NewString() + "/permissions/" + uuid.NewString()},
		{"DELETE", "/users/" + uuid.NewString() + "/roles/" + uuid.NewString()},
		{"GET", "/auth/me"},
	} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(c.method, c.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != 404 || !strings.Contains(w.Body.String(), `"code":"NOT_FOUND"`) {
			t.Fatalf("%s %s: %d %s", c.method, c.path, w.Code, w.Body.String())
		}
	}

	// /auth/me without the auth context is a 401
	bare := gin.New()
	bare.GET("/auth/me", rc.Me)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/auth/me", nil)
	bare.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("me without context: %d", w.Code)
	}
}

func TestAuditController_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := NewAuditController(nil, auditStub{})
	r := gin.New()
	r.GET("/audit", func(c *gin.Context) { c.Set("tenant_id", uuid.New()); ctrl.List(c) })
	r.GET("/noscope", ctrl.List)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/audit?per_page=5&page=2", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"per_page":5`) || !strings.Contains(w.Body.String(), `"page":2`) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/noscope", nil)
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("missing tenant scope: %d", w.Code)
	}
	failing := NewAuditController(nil, auditStub{err: errors.New("db")})
	r2 := gin.New()
	r2.GET("/audit", func(c *gin.Context) { c.Set("tenant_id", uuid.New()); failing.List(c) })
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/audit", nil)
	r2.ServeHTTP(w, req)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("service failure: %d %s", w.Code, w.Body.String())
	}
}

type auditStub struct{ err error }

func (auditStub) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}
func (a auditStub) List(_ context.Context, _ *gorm.DB, _ uuid.UUID, page, perPage int) (*dto.AuditLogListResponse, error) {
	if a.err != nil {
		return nil, a.err
	}
	return &dto.AuditLogListResponse{Data: []dto.AuditLogResponse{}, Meta: dto.PaginationMeta{Page: page, PerPage: perPage}}, nil
}
