package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type TimelineController struct {
	svc services.EmployeeTimelineService
}

func NewTimelineController(svc services.EmployeeTimelineService) *TimelineController {
	return &TimelineController{svc: svc}
}

func (ctrl *TimelineController) List(c *gin.Context) {
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
