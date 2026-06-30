package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

type AuthController struct {
	db      *gorm.DB
	authSvc services.AuthService
}

// NewAuthController creates an AuthController instance.
func NewAuthController(db *gorm.DB, authSvc services.AuthService) *AuthController {
	return &AuthController{
		db:      db,
		authSvc: authSvc,
	}
}

func (ctrl *AuthController) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()
	correlationID := uuid.New()

	res, err := ctrl.authSvc.Login(c.Request.Context(), ctrl.db, req, ipAddress, userAgent, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *AuthController) Logout(c *gin.Context) {
	// Extract values set by Auth/Tenant middlewares in context
	tenantIDVal, ok1 := c.Get("tenant_id")
	userIDVal, ok2 := c.Get("user_id")
	sessionIDVal, ok3 := c.Get("session_id")

	if !ok1 || !ok2 || !ok3 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized context mapping"})
		return
	}

	tenantID := tenantIDVal.(uuid.UUID)
	userID := userIDVal.(uuid.UUID)
	sessionID := sessionIDVal.(uuid.UUID)

	correlationID := uuid.New()
	err := ctrl.authSvc.Logout(c.Request.Context(), ctrl.db, tenantID, userID, sessionID, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Logged out successfully"}, nil)
}

func (ctrl *AuthController) Refresh(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	res, err := ctrl.authSvc.RefreshToken(c.Request.Context(), ctrl.db, req, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *AuthController) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	err := ctrl.authSvc.ForgotPassword(c.Request.Context(), ctrl.db, req, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	// Always return 200 message response for security reasons
	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "If the email is registered, reset link was sent"}, nil)
}

func (ctrl *AuthController) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	err := ctrl.authSvc.ResetPassword(c.Request.Context(), ctrl.db, req, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Password updated successfully"}, nil)
}
