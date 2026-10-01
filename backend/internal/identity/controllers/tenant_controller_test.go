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

func TestTenantController_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockTenantService{}
	ctrl := NewTenantController(nil, mockSvc)

	mockSvc.CreateTenantFunc = func(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, cid uuid.UUID) (*dto.TenantResponse, error) {
		return &dto.TenantResponse{
			ID:   uuid.New().String(),
			Name: req.Name,
			Slug: req.Slug,
		}, nil
	}

	router := gin.New()
	router.POST("/tenants", ctrl.Create)

	reqBody := dto.CreateTenantRequest{
		Name: "Acme",
		Slug: "acme",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tenants", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTenantController_GetByID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockTenantService{}
	ctrl := NewTenantController(nil, mockSvc)

	id := uuid.New()
	mockSvc.GetTenantByIDFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID) (*dto.TenantResponse, error) {
		if tid == id {
			return &dto.TenantResponse{
				ID:   id.String(),
				Name: "Acme",
			}, nil
		}
		return nil, nil
	}

	router := gin.New()
	router.GET("/tenants/:id", ctrl.GetByID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/tenants/"+id.String(), nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTenantController_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockTenantService{}
	ctrl := NewTenantController(nil, mockSvc)

	mockSvc.ListTenantsFunc = func(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error) {
		return &dto.TenantListResponse{
			Data: []dto.TenantResponse{{Name: "Acme"}},
			Meta: dto.PaginationMeta{TotalItems: 1},
		}, nil
	}

	router := gin.New()
	router.GET("/tenants", ctrl.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/tenants?page=1&per_page=10", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTenantController_Update(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockTenantService{}
	ctrl := NewTenantController(nil, mockSvc)

	id := uuid.New()
	mockSvc.UpdateTenantFunc = func(ctx context.Context, tx *gorm.DB, tid uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error) {
		return &dto.TenantResponse{ID: id.String()}, nil
	}

	router := gin.New()
	router.PATCH("/tenants/:id", ctrl.Update)

	newName := "Acme Corp"
	reqBody := dto.UpdateTenantRequest{Name: &newName}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/tenants/"+id.String(), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTenantController_ActivateSuspend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockTenantService{}
	ctrl := NewTenantController(nil, mockSvc)

	id := uuid.New()
	mockSvc.ActivateTenantFunc = func(ctx context.Context, tx *gorm.DB, tid uuid.UUID, cid uuid.UUID) (*dto.TenantResponse, error) {
		return &dto.TenantResponse{ID: id.String(), Status: "active"}, nil
	}
	mockSvc.SuspendTenantFunc = func(ctx context.Context, tx *gorm.DB, tid uuid.UUID, cid uuid.UUID) (*dto.TenantResponse, error) {
		return &dto.TenantResponse{ID: id.String(), Status: "suspended"}, nil
	}

	router := gin.New()
	router.POST("/tenants/:id/activate", ctrl.Activate)
	router.POST("/tenants/:id/suspend", ctrl.Suspend)

	// Activate
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tenants/"+id.String()+"/activate", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Suspend
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/tenants/"+id.String()+"/suspend", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
}
