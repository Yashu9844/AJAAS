package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type DocumentController struct {
	svc services.EmployeeDocumentService
}

func NewDocumentController(svc services.EmployeeDocumentService) *DocumentController {
	return &DocumentController{svc: svc}
}

func (ctrl *DocumentController) Upload(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	var req dto.UploadDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	res, err := ctrl.svc.Upload(c.Request.Context(), tenantID, profileID, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *DocumentController) List(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	res, err := ctrl.svc.List(c.Request.Context(), tenantID, profileID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *DocumentController) Verify(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	verifierID, ok := getUserID(c)
	if !ok {
		return
	}

	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	docID, err := uuid.Parse(c.Param("doc_id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid document ID", StatusCode: http.StatusBadRequest})
		return
	}

	res, err := ctrl.svc.Verify(c.Request.Context(), tenantID, profileID, docID, verifierID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}
