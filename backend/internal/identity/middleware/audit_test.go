package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAuditLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auditSvc := &MockAuditService{}

	tenantID := uuid.New()
	userID := uuid.New()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		c.Set("user_id", userID)
		c.Next()
	})
	router.Use(AuditLog(nil, auditSvc, "user.read", "user"))
	router.GET("/test/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/test-err", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	// 1. Success case: 200 OK logs audit entry
	var logCalled bool
	auditSvc.LogFunc = func(ctx context.Context, tx *gorm.DB, tid, uid, action, resource, rid string, meta interface{}, ip, ua string) error {
		if tid == tenantID.String() && uid == userID.String() && action == "user.read" && resource == "user" && rid == "123" {
			logCalled = true
		}
		return nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test/123", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !logCalled {
		t.Error("expected audit log service to be called")
	}

	// 2. Failure case: 500 InternalServerError skips logging
	logCalled = false
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test-err", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w2.Code)
	}
	if logCalled {
		t.Error("expected audit log service to NOT be called for error responses")
	}
}
