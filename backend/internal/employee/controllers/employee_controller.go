package controllers

import (
	"github.com/jaas/jaas/internal/shared/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type EmployeeController struct {
	svc services.EmployeeService
}

func NewEmployeeController(svc services.EmployeeService) *EmployeeController {
	return &EmployeeController{svc: svc}
}

func (ctrl *EmployeeController) Create(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	var req dto.CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	res, err := ctrl.svc.CreateEmployee(c.Request.Context(), tenantID, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *EmployeeController) GetByID(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	res, err := ctrl.svc.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *EmployeeController) GetMe(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	res, err := ctrl.svc.GetByUserID(c.Request.Context(), tenantID, userID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *EmployeeController) UpdateMe(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req dto.UpdateSelfContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	res, err := ctrl.svc.UpdateSelfContact(c.Request.Context(), tenantID, userID, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *EmployeeController) List(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	page, perPage := utils.ParsePagination(c)
	status := c.Query("status")
	search := c.Query("search")

	var deptID *uuid.UUID
	if d := c.Query("department_id"); d != "" {
		if parsed, err := uuid.Parse(d); err == nil {
			deptID = &parsed
		}
	}

	filter := dto.EmployeeFilter{
		Page:         page,
		PerPage:      perPage,
		Status:       status,
		Search:       search,
		DepartmentID: deptID,
	}

	items, total, err := ctrl.svc.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		respondError(c, err)
		return
	}

	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}
	meta := gin.H{
		"page":        page,
		"per_page":    perPage,
		"total_items": total,
		"total_pages": totalPages,
	}
	respondSuccess(c, http.StatusOK, items, meta)
}

func (ctrl *EmployeeController) Update(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	var req dto.UpdateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	res, err := ctrl.svc.Update(c.Request.Context(), tenantID, id, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *EmployeeController) TransitionStatus(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	var req dto.TransitionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	res, err := ctrl.svc.TransitionStatus(c.Request.Context(), tenantID, id, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *EmployeeController) Deactivate(c *gin.Context) {
	tenantID, ok := getTenantID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid employee ID", StatusCode: http.StatusBadRequest})
		return
	}

	if err := ctrl.svc.Deactivate(c.Request.Context(), tenantID, id); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, gin.H{"status": "inactive"}, nil)
}
