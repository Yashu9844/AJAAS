package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"gorm.io/gorm"
)

func TestRoleController_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	mockSvc.CreateRoleFunc = func(ctx context.Context, tx *gorm.DB, tid uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error) {
		return &dto.RoleResponse{
			ID:   uuid.New().String(),
			Name: req.Name,
		}, nil
	}

	router := gin.New()
	router.POST("/roles", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.Create(c)
	})

	reqBody := dto.CreateRoleRequest{
		Name: "manager",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/roles", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestRoleController_GetByID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	roleID := uuid.New()
	mockSvc.GetRoleByIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) (*dto.RoleResponse, error) {
		return &dto.RoleResponse{ID: rid.String(), Name: "manager"}, nil
	}

	router := gin.New()
	router.GET("/roles/:id", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.GetByID(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/roles/"+roleID.String(), nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRoleController_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	mockSvc.ListRolesFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, page, perPage int) (*dto.RoleListResponse, error) {
		return &dto.RoleListResponse{
			Data: []dto.RoleResponse{{Name: "admin"}},
			Meta: dto.PaginationMeta{TotalItems: 1},
		}, nil
	}

	router := gin.New()
	router.GET("/roles", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.List(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/roles?page=1&per_page=10", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRoleController_Update(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	roleID := uuid.New()
	mockSvc.UpdateRoleFunc = func(ctx context.Context, tx *gorm.DB, tid, rid uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error) {
		return &dto.RoleResponse{ID: rid.String()}, nil
	}

	router := gin.New()
	router.PATCH("/roles/:id", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.Update(c)
	})

	name := "manager2"
	reqBody := dto.UpdateRoleRequest{Name: &name}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/roles/"+roleID.String(), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRoleController_Delete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	roleID := uuid.New()
	mockSvc.DeleteRoleFunc = func(ctx context.Context, tx *gorm.DB, tid, rid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.DELETE("/roles/:id", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.Delete(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/roles/"+roleID.String(), nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRoleController_AssignPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	roleID := uuid.New()
	mockSvc.AssignPermissionsFunc = func(ctx context.Context, tx *gorm.DB, tid, rid uuid.UUID, req dto.AssignPermissionsRequest, cid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.POST("/roles/:id/permissions", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.AssignPermissions(c)
	})

	reqBody := dto.AssignPermissionsRequest{
		PermissionIDs: []string{uuid.New().String()},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/roles/"+roleID.String()+"/permissions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRoleController_AssignRolesToUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockRoleService{}
	ctrl := NewRoleController(nil, mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()
	mockSvc.AssignRolesToUserFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID, req dto.AssignRoleRequest, cid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.POST("/users/:id/roles", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.AssignRolesToUser(c)
	})

	reqBody := dto.AssignRoleRequest{
		RoleIDs: []string{uuid.New().String()},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/users/"+userID.String()+"/roles", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
