package controllers

import (
	"github.com/jaas/jaas/internal/shared/database"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// DesignationController handles designation endpoints.
type DesignationController struct {
	db  *gorm.DB
	svc services.DesignationService
}

// NewDesignationController returns a DesignationController.
func NewDesignationController(db *gorm.DB, svc services.DesignationService) *DesignationController {
	return &DesignationController{db: db, svc: svc}
}

// Create handles POST /designations.
func (ctrl *DesignationController) Create(c *gin.Context) {
	var req dto.CreateDesignationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
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
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.CreateDesignation(c.Request.Context(), tx, tenantID, req, uuid.New())
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusCreated, res, nil)
}

// GetByID handles GET /designations/:id.
func (ctrl *DesignationController) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid designation ID")
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.GetDesignation(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// List handles GET /designations.
func (ctrl *DesignationController) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.ListDesignations(c.Request.Context(), ctrl.db, tenantID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

// Update handles PATCH /designations/:id.
func (ctrl *DesignationController) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid designation ID")
		return
	}
	var req dto.UpdateDesignationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
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
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.UpdateDesignation(c.Request.Context(), tx, tenantID, id, req)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// Deactivate handles POST /designations/:id/deactivate. FR-DG004.
func (ctrl *DesignationController) Deactivate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid designation ID")
		return
	}
	var req dto.DeactivateRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			respondBindError(c, err)
			return
		}
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.DeactivateDesignation(c.Request.Context(), tx, tenantID, id, req.Reason)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}
