package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

type RoleController struct {
	db      *gorm.DB
	roleSvc services.RoleService
}

// NewRoleController creates a RoleController.
func NewRoleController(db *gorm.DB, roleSvc services.RoleService) *RoleController {
	return &RoleController{
		db:      db,
		roleSvc: roleSvc,
	}
}

func (ctrl *RoleController) Create(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	res, err := ctrl.roleSvc.CreateRole(c.Request.Context(), ctrl.db, tenantID, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *RoleController) GetByID(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	res, err := ctrl.roleSvc.GetRoleByID(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *RoleController) List(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	res, err := ctrl.roleSvc.ListRoles(c.Request.Context(), ctrl.db, tenantID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

func (ctrl *RoleController) Update(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	res, err := ctrl.roleSvc.UpdateRole(c.Request.Context(), ctrl.db, tenantID, id, req)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *RoleController) Delete(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	err = ctrl.roleSvc.DeleteRole(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Role deleted successfully"}, nil)
}

func (ctrl *RoleController) AssignPermissions(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var req dto.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	err = ctrl.roleSvc.AssignPermissions(c.Request.Context(), ctrl.db, tenantID, id, req, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Permissions assigned to role successfully"}, nil)
}

func (ctrl *RoleController) AssignRolesToUser(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tenant scope"})
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req dto.AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	err = ctrl.roleSvc.AssignRolesToUser(c.Request.Context(), ctrl.db, tenantID, userID, req, correlationID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Roles assigned to user successfully"}, nil)
}
