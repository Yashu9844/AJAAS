package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/services"
	"gorm.io/gorm"
)

func TestAuthenticate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokenSvc := &MockTokenService{}
	sessionSvc := &MockSessionService{}

	tenantID := uuid.New()
	userID := uuid.New()
	sessionID := uuid.New()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Mock resolved tenant context
		c.Set("tenant_id", tenantID)
		c.Next()
	})
	router.Use(Authenticate(nil, tokenSvc, sessionSvc))
	router.GET("/test", func(c *gin.Context) {
		uid, _ := c.Get("user_id")
		sid, _ := c.Get("session_id")
		c.JSON(http.StatusOK, gin.H{
			"user_id":    uid.(uuid.UUID).String(),
			"session_id": sid.(uuid.UUID).String(),
		})
	})

	// 1. Success case
	tokenSvc.ValidateAccessTokenFunc = func(tokenStr string) (*services.UserClaims, error) {
		return &services.UserClaims{
			TenantID:  tenantID.String(),
			SessionID: sessionID.String(),
		}, nil
	}
	// set subject claims using reflection because claims struct Subject field is embedded under RegisteredClaims.Subject
	// Actually Subject is claims.Subject in JWT. So we can just set:
	// wait, the claims struct is:
	// type UserClaims struct {
	// 	jwt.RegisteredClaims
	// 	TenantID  string   `json:"tid"`
	// 	Email     string   `json:"email"`
	// 	Roles     []string `json:"roles"`
	// 	SessionID string   `json:"sid"`
	// }
	// jwt.RegisteredClaims has field Subject string. We can set it directly!
	// let's confirm. Yes, Subject is exported.
	tokenSvc.ValidateAccessTokenFunc = func(tokenStr string) (*services.UserClaims, error) {
		claims := &services.UserClaims{
			TenantID:  tenantID.String(),
			SessionID: sessionID.String(),
		}
		claims.Subject = userID.String()
		return claims, nil
	}

	sessionSvc.ValidateSessionFunc = func(ctx context.Context, db *gorm.DB, sid uuid.UUID) (bool, error) {
		return sid == sessionID, nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Missing authorization header
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w2.Code)
	}

	// 3. Tenant mismatch isolation breach
	tokenSvc.ValidateAccessTokenFunc = func(tokenStr string) (*services.UserClaims, error) {
		claims := &services.UserClaims{
			TenantID:  uuid.New().String(), // different tenant
			SessionID: sessionID.String(),
		}
		claims.Subject = userID.String()
		return claims, nil
	}

	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.Header.Set("Authorization", "Bearer token-mismatch")
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w3.Code)
	}
}
