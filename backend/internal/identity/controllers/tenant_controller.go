package controllers

import (
	"github.com/jaas/jaas/internal/shared/database"
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
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New() // generated context correlation tracking
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.tenantSvc.CreateTenant(c.Request.Context(), tx, req, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *TenantController) GetByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid tenant ID")
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
		respondBadRequest(c, "Invalid tenant ID")
		return
	}

	var req dto.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.tenantSvc.UpdateTenant(c.Request.Context(), tx, id, req)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *TenantController) Activate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid tenant ID")
		return
	}

	correlationID := uuid.New()
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.tenantSvc.ActivateTenant(c.Request.Context(), tx, id, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *TenantController) Suspend(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid tenant ID")
		return
	}

	correlationID := uuid.New()
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.tenantSvc.SuspendTenant(c.Request.Context(), tx, id, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}
