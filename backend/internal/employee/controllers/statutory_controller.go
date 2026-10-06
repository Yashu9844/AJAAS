package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type StatutoryController struct {
	svc services.EmployeeStatutoryService
}

func NewStatutoryController(svc services.EmployeeStatutoryService) *StatutoryController {
	return &StatutoryController{svc: svc}
}

func (ctrl *StatutoryController) Get(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	// NFR-SEC002: PII stays masked unless the caller explicitly asks (?unmasked=true) AND holds employee:read_sensitive
	// (resolved server-side by the PermissionFlag middleware; the query string alone never grants access).
	sensitive := false
	if c.Query("unmasked") == "true" {
		if !c.GetBool("has_sensitive_perm") {
			respondError(c, &sharedErrors.AppError{Code: "FORBIDDEN", Message: "Permission employee:read_sensitive is required to view unmasked data", StatusCode: http.StatusForbidden})
			return
		}
		sensitive = true
	}

	res, err := ctrl.svc.GetByProfileID(c.Request.Context(), tenantID, profileID, sensitive)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *StatutoryController) Upsert(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	var req dto.UpdateStatutoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	res, err := ctrl.svc.Upsert(c.Request.Context(), tenantID, profileID, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}
