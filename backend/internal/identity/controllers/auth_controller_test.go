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

func TestAuthController_Login(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockAuthService{}
	ctrl := NewAuthController(nil, mockSvc)

	mockSvc.LoginFunc = func(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ip, ua string, cid uuid.UUID) (*dto.LoginResponse, error) {
		return &dto.LoginResponse{
			AccessToken: "signed-jwt",
		}, nil
	}

	router := gin.New()
	router.POST("/auth/login", ctrl.Login)

	reqBody := dto.LoginRequest{
		TenantSlug: "acme",
		Email:      "user@example.com",
		Password:   "password123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestAuthController_Logout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockAuthService{}
	ctrl := NewAuthController(nil, mockSvc)

	mockSvc.LogoutFunc = func(ctx context.Context, tx *gorm.DB, tid, uid, sid, cid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.POST("/auth/logout", func(c *gin.Context) {
		// Mock middlewares setting values in context
		c.Set("tenant_id", uuid.New())
		c.Set("user_id", uuid.New())
		c.Set("session_id", uuid.New())
		ctrl.Logout(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/logout", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthController_Refresh(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockAuthService{}
	ctrl := NewAuthController(nil, mockSvc)

	mockSvc.RefreshTokenFunc = func(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, cid uuid.UUID) (*dto.RefreshTokenResponse, error) {
		return &dto.RefreshTokenResponse{
			AccessToken: "new-access",
		}, nil
	}

	router := gin.New()
	router.POST("/auth/refresh", ctrl.Refresh)

	reqBody := dto.RefreshTokenRequest{
		RefreshToken: "old-refresh-token",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/refresh", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthController_ForgotPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockAuthService{}
	ctrl := NewAuthController(nil, mockSvc)

	mockSvc.ForgotPasswordFunc = func(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, cid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.POST("/auth/forgot-password", ctrl.ForgotPassword)

	reqBody := dto.ForgotPasswordRequest{
		TenantSlug: "acme",
		Email:      "user@example.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/forgot-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthController_ResetPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockSvc := &MockAuthService{}
	ctrl := NewAuthController(nil, mockSvc)

	mockSvc.ResetPasswordFunc = func(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, cid uuid.UUID) error {
		return nil
	}

	router := gin.New()
	router.POST("/auth/reset-password", ctrl.ResetPassword)

	reqBody := dto.ResetPasswordRequest{
		Token:           "reset-token",
		NewPassword:     "newpassword123",
		ConfirmPassword: "newpassword123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/reset-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
