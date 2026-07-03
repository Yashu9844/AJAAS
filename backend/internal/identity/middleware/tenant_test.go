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

func TestTenantResolver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &MockTenantRepository{}

	router := gin.New()
	router.Use(TenantResolver(nil, repo))
	router.GET("/test", func(c *gin.Context) {
		tenantID, _ := c.Get("tenant_id")
		tenantSlug, _ := c.Get("tenant_slug")
		c.JSON(http.StatusOK, gin.H{
			"tenant_id":   tenantID.(uuid.UUID).String(),
			"tenant_slug": tenantSlug.(string),
		})
	})

	tenantID := uuid.New()

	// 1. Success case: acme.localhost
	repo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		if slug == "acme" {
			tnt := &models.Tenant{
				Slug:   "acme",
				Status: "active",
			}
			tnt.ID = tenantID
			return tnt, nil
		}
		return nil, nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Host = "acme.localhost:8080"
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Suspended tenant
	repo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		if slug == "suspended" {
			tnt := &models.Tenant{
				Slug:   "suspended",
				Status: "suspended",
			}
			tnt.ID = tenantID
			return tnt, nil
		}
		return nil, nil
	}

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Host = "suspended.localhost"
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w2.Code)
	}

	// 3. Tenant not found
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.Host = "missing.localhost"
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w3.Code)
	}

	// 4. Missing subdomain
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("GET", "/test", nil)
	req4.Host = "localhost"
	router.ServeHTTP(w4, req4)

	if w4.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w4.Code)
	}
}
