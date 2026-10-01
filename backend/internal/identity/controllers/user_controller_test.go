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

func TestUserController_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockUserService{}
	ctrl := NewUserController(nil, mockSvc)

	tenantID := uuid.New()
	mockSvc.CreateUserFunc = func(ctx context.Context, tx *gorm.DB, tid uuid.UUID, req dto.CreateUserRequest, cid uuid.UUID) (*dto.UserResponse, error) {
		return &dto.UserResponse{
			ID:    uuid.New().String(),
			Email: req.Email,
		}, nil
	}

	router := gin.New()
	router.POST("/users", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.Create(c)
	})

	reqBody := dto.CreateUserRequest{
		Email:     "user@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUserController_GetByID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockUserService{}
	ctrl := NewUserController(nil, mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()
	mockSvc.GetByIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) (*dto.UserResponse, error) {
		return &dto.UserResponse{ID: uid.String(), Email: "user@example.com"}, nil
	}

	router := gin.New()
	router.GET("/users/:id", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.GetByID(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/"+userID.String(), nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserController_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockUserService{}
	ctrl := NewUserController(nil, mockSvc)

	tenantID := uuid.New()
	mockSvc.ListUsersFunc = func(ctx context.Context, db *gorm.DB, tid uuid.UUID, page, perPage int) (*dto.UserListResponse, error) {
		return &dto.UserListResponse{
			Data: []dto.UserResponse{{Email: "user@example.com"}},
			Meta: dto.PaginationMeta{TotalItems: 1},
		}, nil
	}

	router := gin.New()
	router.GET("/users", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.List(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users?page=1&per_page=10", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserController_Update(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockUserService{}
	ctrl := NewUserController(nil, mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()
	mockSvc.UpdateUserFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error) {
		return &dto.UserResponse{ID: uid.String()}, nil
	}

	router := gin.New()
	router.PATCH("/users/:id", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.Update(c)
	})

	firstName := "Johnny"
	reqBody := dto.UpdateUserRequest{FirstName: &firstName}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/users/"+userID.String(), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserController_Deactivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockUserService{}
	ctrl := NewUserController(nil, mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()
	mockSvc.DeactivateUserFunc = func(ctx context.Context, tx *gorm.DB, tid, uid, cid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.POST("/users/:id/deactivate", func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		ctrl.Deactivate(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/users/"+userID.String()+"/deactivate", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
