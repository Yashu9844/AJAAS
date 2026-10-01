package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userRoleRepo := &MockUserRoleRepository{}
	rolePermRepo := &MockRolePermissionRepository{}

	tenantID := uuid.New()
	userID := uuid.New()
	roleID := uuid.New()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		c.Set("user_id", userID)
		c.Next()
	})

	router.GET("/users", func(c *gin.Context) {
		// Mock roles in token claims context
		c.Set("roles", []string{"member"})
		c.Next()
	}, RequirePermission(nil, "users", "read", userRoleRepo, rolePermRepo), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	router.GET("/admin-bypass", func(c *gin.Context) {
		c.Set("roles", []string{"tenant_admin"})
		c.Next()
	}, RequirePermission(nil, "users", "read", userRoleRepo, rolePermRepo), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 1. Success case: user has roles with matching permission
	userRoleRepo.FindByUserIDFunc = func(ctx context.Context, db *gorm.DB, tid, uid uuid.UUID) ([]models.UserRole, error) {
		return []models.UserRole{{RoleID: roleID}}, nil
	}

	rolePermRepo.FindByRoleIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) ([]models.RolePermission, error) {
		return []models.RolePermission{
			{
				Permission: &models.Permission{
					Resource: "users",
					Action:   "read",
				},
			},
		}, nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Bypass case: tenant_admin
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/admin-bypass", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}

	// 3. Forbidden case: user role doesn't have the permission
	rolePermRepo.FindByRoleIDFunc = func(ctx context.Context, db *gorm.DB, tid, rid uuid.UUID) ([]models.RolePermission, error) {
		return []models.RolePermission{
			{
				Permission: &models.Permission{
					Resource: "users",
					Action:   "write", // mismatch action
				},
			},
		}, nil
	}

	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/users", nil)
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w3.Code)
	}
}
