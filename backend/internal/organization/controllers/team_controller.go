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

// TeamController handles team endpoints.
type TeamController struct {
	db  *gorm.DB
	svc services.TeamService
}

// NewTeamController returns a TeamController.
func NewTeamController(db *gorm.DB, svc services.TeamService) *TeamController {
	return &TeamController{db: db, svc: svc}
}

// Create handles POST /teams.
func (ctrl *TeamController) Create(c *gin.Context) {
	var req dto.CreateTeamRequest
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
	res, err := ctrl.svc.CreateTeam(c.Request.Context(), ctrl.db, tenantID, req, uuid.New())
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusCreated, res, nil)
}

// GetByID handles GET /teams/:id.
func (ctrl *TeamController) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.GetTeam(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}

// List handles GET /teams with optional department_id filter.
func (ctrl *TeamController) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	var deptID *uuid.UUID
	if raw := c.Query("department_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid department_id"})
			return
		}
		deptID = &parsed
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.ListTeams(c.Request.Context(), ctrl.db, tenantID, deptID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

// Update handles PATCH /teams/:id.
func (ctrl *TeamController) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}
	var req dto.UpdateTeamRequest
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
	res, err := ctrl.svc.UpdateTeam(c.Request.Context(), ctrl.db, tenantID, id, req)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}
