package controllers

import (
	"github.com/jaas/jaas/internal/shared/database"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"net/http"

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
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.roleSvc.CreateRole(c.Request.Context(), tx, tenantID, req)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *RoleController) GetByID(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid role ID")
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
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	page, perPage := utils.ParsePagination(c)

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
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid role ID")
		return
	}

	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.roleSvc.UpdateRole(c.Request.Context(), tx, tenantID, id, req)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *RoleController) Delete(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid role ID")
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err = ctrl.roleSvc.DeleteRole(c.Request.Context(), tx, tenantID, id)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Role deleted successfully"}, nil)
}

func (ctrl *RoleController) AssignPermissions(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid role ID")
		return
	}

	var req dto.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err = ctrl.roleSvc.AssignPermissions(c.Request.Context(), tx, tenantID, id, req, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Permissions assigned to role successfully"}, nil)
}

func (ctrl *RoleController) AssignRolesToUser(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		respondBadRequest(c, "Invalid user ID")
		return
	}

	var req dto.AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	correlationID := uuid.New()
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err = ctrl.roleSvc.AssignRolesToUser(c.Request.Context(), tx, tenantID, userID, req, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Roles assigned to user successfully"}, nil)
}

// RemovePermission detaches a permission from a role (DELETE /roles/:id/permissions/:permission_id).
func (ctrl *RoleController) RemovePermission(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)
	roleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid role ID")
		return
	}
	permID, err := uuid.Parse(c.Param("permission_id"))
	if err != nil {
		respondBadRequest(c, "Invalid permission ID")
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err = ctrl.roleSvc.RemovePermission(c.Request.Context(), tx, tenantID, roleID, permID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Permission removed from role successfully"}, nil)
}

// RemoveRoleFromUser detaches a role from a user (DELETE /users/:id/roles/:role_id).
func (ctrl *RoleController) RemoveRoleFromUser(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid user ID")
		return
	}
	roleID, err := uuid.Parse(c.Param("role_id"))
	if err != nil {
		respondBadRequest(c, "Invalid role ID")
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err = ctrl.roleSvc.RemoveRoleFromUser(c.Request.Context(), tx, tenantID, userID, roleID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "Role removed from user successfully"}, nil)
}

// Me returns the caller's roles and effective permissions (GET /auth/me).
func (ctrl *RoleController) Me(c *gin.Context) {
	tenantIDVal, ok1 := c.Get("tenant_id")
	userIDVal, ok2 := c.Get("user_id")
	if !ok1 || !ok2 {
		respondError(c, sharedErrors.ErrUnauthorized)
		return
	}
	res, err := ctrl.roleSvc.GetMyAccess(c.Request.Context(), ctrl.db, tenantIDVal.(uuid.UUID), userIDVal.(uuid.UUID))
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, res, nil)
}
