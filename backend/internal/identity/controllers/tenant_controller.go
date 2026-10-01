package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

type TenantController struct {
	db        *gorm.DB
	tenantSvc services.TenantService
}

// NewTenantController returns a TenantController instance.
func NewTenantController(db *gorm.DB, tenantSvc services.TenantService) *TenantController {
	return &TenantController{
		db:        db,
		tenantSvc: tenantSvc,
	}
}

func (ctrl *TenantController) Create(c *gin.Context) {
	var req dto.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New() // generated context correlation tracking
	res, err := ctrl.tenantSvc.CreateTenant(c.Request.Context(), ctrl.db, req, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *TenantController) GetByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	res, err := ctrl.tenantSvc.GetTenantByID(c.Request.Context(), ctrl.db, id)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *TenantController) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	res, err := ctrl.tenantSvc.ListTenants(c.Request.Context(), ctrl.db, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

func (ctrl *TenantController) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var req dto.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	res, err := ctrl.tenantSvc.UpdateTenant(c.Request.Context(), ctrl.db, id, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *TenantController) Activate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	correlationID := uuid.New()
	res, err := ctrl.tenantSvc.ActivateTenant(c.Request.Context(), ctrl.db, id, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *TenantController) Suspend(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	correlationID := uuid.New()
	res, err := ctrl.tenantSvc.SuspendTenant(c.Request.Context(), ctrl.db, id, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}
