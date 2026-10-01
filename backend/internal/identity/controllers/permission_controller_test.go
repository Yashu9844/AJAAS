package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/dto"
	"gorm.io/gorm"
)

func TestPermissionController_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockPermissionService{}
	ctrl := NewPermissionController(nil, mockSvc)

	mockSvc.ListPermissionsFunc = func(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error) {
		return &dto.PermissionListResponse{
			Data: []dto.PermissionResponse{{Resource: "users", Action: "read"}},
			Meta: dto.PaginationMeta{TotalItems: 1},
		}, nil
	}

	router := gin.New()
	router.GET("/permissions", ctrl.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/permissions?page=1&per_page=10", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
