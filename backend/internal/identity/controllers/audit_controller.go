package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// AuditController exposes the read-only audit trail of a tenant.
type AuditController struct {
	db       *gorm.DB
	auditSvc services.AuditService
}

// NewAuditController creates an AuditController.
func NewAuditController(db *gorm.DB, auditSvc services.AuditService) *AuditController {
	return &AuditController{db: db, auditSvc: auditSvc}
}

// List handles GET /audit-logs.
func (ctrl *AuditController) List(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	page, perPage := utils.ParsePagination(c)
	res, err := ctrl.auditSvc.List(c.Request.Context(), ctrl.db, tenantIDVal.(uuid.UUID), page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}
