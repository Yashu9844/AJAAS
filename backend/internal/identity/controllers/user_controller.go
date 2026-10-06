package controllers

import (
	"github.com/jaas/jaas/internal/shared/database"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

type UserController struct {
	db      *gorm.DB
	userSvc services.UserService
}

// NewUserController creates a UserController.
func NewUserController(db *gorm.DB, userSvc services.UserService) *UserController {
	return &UserController{
		db:      db,
		userSvc: userSvc,
	}
}

func (ctrl *UserController) Create(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	var req dto.CreateUserRequest
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
	res, err := ctrl.userSvc.CreateUser(c.Request.Context(), tx, tenantID, req, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusCreated, res, nil)
}

func (ctrl *UserController) GetByID(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid user ID")
		return
	}

	res, err := ctrl.userSvc.GetByID(c.Request.Context(), ctrl.db, tenantID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *UserController) List(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	res, err := ctrl.userSvc.ListUsers(c.Request.Context(), ctrl.db, tenantID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res.Data, res.Meta)
}

func (ctrl *UserController) Update(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid user ID")
		return
	}

	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if valErrs := utils.ValidateStruct(req); len(valErrs) > 0 {
		respondValidationError(c, valErrs)
		return
	}

	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.userSvc.UpdateUser(c.Request.Context(), tx, tenantID, id, req)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, res, nil)
}

func (ctrl *UserController) Deactivate(c *gin.Context) {
	tenantIDVal, ok := c.Get("tenant_id")
	if !ok {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	tenantID := tenantIDVal.(uuid.UUID)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondBadRequest(c, "Invalid user ID")
		return
	}

	correlationID := uuid.New()
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err = ctrl.userSvc.DeactivateUser(c.Request.Context(), tx, tenantID, id, correlationID)
	if err = database.Finish(tx, err); err != nil {
		respondError(c, err)
		return
	}

	respondSuccess(c, http.StatusOK, dto.MessageResponse{Message: "User deactivated successfully"}, nil)
}
