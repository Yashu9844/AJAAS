package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// MappingController handles user→org mapping endpoints.
type MappingController struct {
	db  *gorm.DB
	svc services.MappingService
}

// NewMappingController returns a MappingController.
func NewMappingController(db *gorm.DB, svc services.MappingService) *MappingController {
	return &MappingController{db: db, svc: svc}
}

// Create handles POST /mappings.
func (ctrl *MappingController) Create(c *gin.Context) {
	var req dto.CreateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.CreateMapping(c.Request.Context(), ctrl.db, tenantID, req, uuid.New())
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusCreated, res, nil)
}

// GetByID handles GET /mappings/:id.
func (ctrl *MappingController) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping ID"})
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.GetMapping(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// ListByUser handles GET /mappings?user_id=.
func (ctrl *MappingController) ListByUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Query("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user_id"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.ListUserMappings(c.Request.Context(), ctrl.db, tenantID, userID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

// Update handles PATCH /mappings/:id.
func (ctrl *MappingController) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping ID"})
		return
	}
	var req dto.UpdateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.UpdateMapping(c.Request.Context(), ctrl.db, tenantID, id, req)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// Deactivate handles POST /mappings/:id/deactivate.
func (ctrl *MappingController) Deactivate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping ID"})
		return
	}
	var req dto.DeactivateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.DeactivateMapping(c.Request.Context(), ctrl.db, tenantID, id, req)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}
