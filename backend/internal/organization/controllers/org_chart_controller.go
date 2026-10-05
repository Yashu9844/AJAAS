package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/services"
	"gorm.io/gorm"
)

// OrgChartController serves the org-chart read model.
type OrgChartController struct {
	db  *gorm.DB
	svc services.OrgChartService
}

// NewOrgChartController returns an OrgChartController.
func NewOrgChartController(db *gorm.DB, svc services.OrgChartService) *OrgChartController {
	return &OrgChartController{db: db, svc: svc}
}

// Chart handles GET /org-chart.
func (ctrl *OrgChartController) Chart(c *gin.Context) {
	maxDepth, _ := strconv.Atoi(c.DefaultQuery("max_depth", "10"))
	includeInactive, _ := strconv.ParseBool(c.DefaultQuery("include_inactive", "false"))
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.GetChart(c.Request.Context(), ctrl.db, tenantID, maxDepth, includeInactive)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res.Data, nil)
}

// UserChain handles GET /org-chart/chain?user_id=.
func (ctrl *OrgChartController) UserChain(c *gin.Context) {
	userID, err := uuid.Parse(c.Query("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user_id"})
		return
	}
	tenantID, ok := tenantIDOf(c)
	if !ok {
		respondError(c, errMissingTenant)
		return
	}
	res, err := ctrl.svc.GetUserChain(c.Request.Context(), ctrl.db, tenantID, userID)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}
