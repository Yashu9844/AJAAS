package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/organization/dto"
	"github.com/jaas/jaas/internal/hr/organization/services"
	"github.com/jaas/jaas/internal/shared/database"
	"github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// LocationController handles location endpoints (FR-L).
type LocationController struct {
	db  *gorm.DB
	svc services.LocationService
}

// NewLocationController returns a LocationController.
func NewLocationController(db *gorm.DB, svc services.LocationService) *LocationController {
	return &LocationController{db: db, svc: svc}
}

// Create handles POST /locations. FR-L001.
func (ctrl *LocationController) Create(c *gin.Context) {
	var req dto.CreateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.CreateLocation(c.Request.Context(), tx, tenantID, req, middleware.CorrelationOrNew(c.Request.Context()))
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusCreated, res, nil)
}

// GetByID handles GET /locations/:id. FR-L002.
func (ctrl *LocationController) GetByID(c *gin.Context) {
	id, tenantID, ok := ctrl.ids(c)
	if !ok {
		return
	}
	res, err := ctrl.svc.GetLocation(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// List handles GET /locations?status&state_code&search. FR-L002.
func (ctrl *LocationController) List(c *gin.Context) {
	page, perPage := utils.ParsePagination(c)
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	f := dto.LocationFilter{Status: c.Query("status"), StateCode: c.Query("state_code"), Search: c.Query("search")}
	res, err := ctrl.svc.ListLocations(c.Request.Context(), ctrl.db, tenantID, f, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

// Update handles PATCH /locations/:id. FR-L003.
func (ctrl *LocationController) Update(c *gin.Context) {
	id, tenantID, ok := ctrl.ids(c)
	if !ok {
		return
	}
	var req dto.UpdateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.UpdateLocation(c.Request.Context(), tx, tenantID, id, req, middleware.CorrelationOrNew(c.Request.Context()))
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// Deactivate handles POST /locations/:id/deactivate. FR-L004.
func (ctrl *LocationController) Deactivate(c *gin.Context) { ctrl.setStatus(c, "inactive") }

// Activate handles POST /locations/:id/activate. FR-L004.
func (ctrl *LocationController) Activate(c *gin.Context) { ctrl.setStatus(c, "active") }

func (ctrl *LocationController) setStatus(c *gin.Context, status string) {
	id, tenantID, ok := ctrl.ids(c)
	if !ok {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.SetLocationStatus(c.Request.Context(), tx, tenantID, id, status, middleware.CorrelationOrNew(c.Request.Context()))
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// ids parses :id and the tenant; failures are answered here.
func (ctrl *LocationController) ids(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid location ID")
		return uuid.Nil, uuid.Nil, false
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return uuid.Nil, uuid.Nil, false
	}
	return id, tenantID, true
}
