package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/services"
	"gorm.io/gorm"
)

type PermissionController struct {
	db      *gorm.DB
	permSvc services.PermissionService
}

// NewPermissionController creates a PermissionController.
func NewPermissionController(db *gorm.DB, permSvc services.PermissionService) *PermissionController {
	return &PermissionController{
		db:      db,
		permSvc: permSvc,
	}
}

func (ctrl *PermissionController) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	res, err := ctrl.permSvc.ListPermissions(c.Request.Context(), ctrl.db, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}
