package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
)

func TestDepartmentController_Create(t *testing.T) {
	ctrl := NewDepartmentController(nil, &stubDeptSvc{res: &dto.DepartmentResponse{ID: uuid.New().String(), Name: "Engineering", Status: "active"}})

	c, w := newTestContext(t, http.MethodPost, "/departments", strings.NewReader(`{"name":"Engineering"}`))
	ctrl.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("expected data envelope, got %s", w.Body.String())
	}
}

func TestDepartmentController_CreateBadBody(t *testing.T) {
	ctrl := NewDepartmentController(nil, &stubDeptSvc{})
	c, w := newTestContext(t, http.MethodPost, "/departments", strings.NewReader(`{bad json`))
	ctrl.Create(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDepartmentController_GetByID(t *testing.T) {
	ctrl := NewDepartmentController(nil, &stubDeptSvc{res: &dto.DepartmentResponse{ID: uuid.New().String()}})

	c, w := newTestContext(t, http.MethodGet, "/departments/"+uuid.New().String(), nil)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.GetByID(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Bad UUID → 400.
	c2, w2 := newTestContext(t, http.MethodGet, "/departments/nope", nil)
	c2.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	ctrl.GetByID(c2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on bad uuid, got %d", w2.Code)
	}

	// Service 404 propagates.
	ctrlMissing := NewDepartmentController(nil, &stubDeptSvc{err: errNotFound})
	c3, w3 := newTestContext(t, http.MethodGet, "/departments/x", nil)
	c3.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlMissing.GetByID(c3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w3.Code)
	}
}

func TestDepartmentController_List(t *testing.T) {
	ctrl := NewDepartmentController(nil, &stubDeptSvc{})
	c, w := newTestContext(t, http.MethodGet, "/departments?page=1&per_page=20", nil)
	ctrl.List(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"meta"`) {
		t.Fatalf("expected 200 with meta envelope, got %d %s", w.Code, w.Body.String())
	}
}

func TestDepartmentController_MissingTenant(t *testing.T) {
	ctrl := NewDepartmentController(nil, &stubDeptSvc{})
	gin.SetMode(gin.TestMode)
	w := httptestRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptestRequest(http.MethodGet, "/departments", nil)
	ctrl.List(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without tenant context, got %d", w.Code)
	}
}

func TestDepartmentController_Deactivate(t *testing.T) {
	ctrl := NewDepartmentController(nil, &stubDeptSvc{res: &dto.DepartmentResponse{Status: "inactive"}})

	// No body + force query → 200.
	c, w := newTestContext(t, http.MethodPost, "/departments/x/deactivate?force=true", nil)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Deactivate(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Conflict from service (referenced) → 409.
	ctrlConflict := NewDepartmentController(nil, &stubDeptSvc{err: errConflict})
	c2, w2 := newTestContext(t, http.MethodPost, "/departments/x/deactivate", nil)
	c2.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlConflict.Deactivate(c2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w2.Code)
	}
}
