package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/database"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// AccessController serves per-user grants and blocks, explain-access and automatic-role sync
// (docs/ARCHITECTURE.md §17).
type AccessController struct {
	db        *gorm.DB
	svc       services.AccessService
	automatic services.AutomaticRoles
}

// NewAccessController creates an AccessController.
func NewAccessController(db *gorm.DB, svc services.AccessService, automatic services.AutomaticRoles) *AccessController {
	return &AccessController{db: db, svc: svc, automatic: automatic}
}

// target reads the tenant from context and the user from :id.
func target(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	tid, ok := c.Get("tenant_id")
	tenantID, ok2 := tid.(uuid.UUID)
	if !ok || !ok2 {
		respondBadRequest(c, "Missing tenant scope")
		return uuid.Nil, uuid.Nil, false
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid user ID")
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func permissionParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("permission_id"))
	if err != nil {
		respondBadRequest(c, "Invalid permission ID")
		return uuid.Nil, false
	}
	return id, true
}

func bindValid(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		respondBindError(c, err)
		return false
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		respondValidationError(c, errs)
		return false
	}
	return true
}

// Explain returns a user's effective access with sources (GET /users/:id/access).
func (ctrl *AccessController) Explain(c *gin.Context) {
	tenantID, userID, ok := target(c)
	if !ok {
		return
	}
	res, err := ctrl.svc.Explain(c.Request.Context(), ctrl.db, tenantID, userID)
	respond(c, http.StatusOK, res, err)
}

// ExplainMe returns the caller's own effective access with sources (GET /auth/me/access).
func (ctrl *AccessController) ExplainMe(c *gin.Context) {
	tid, ok1 := c.Get("tenant_id")
	uid, ok2 := c.Get("user_id")
	if !ok1 || !ok2 {
		respondError(c, sharedErrors.ErrUnauthorized)
		return
	}
	res, err := ctrl.svc.Explain(c.Request.Context(), ctrl.db, tid.(uuid.UUID), uid.(uuid.UUID))
	respond(c, http.StatusOK, res, err)
}

func respond(c *gin.Context, status int, res interface{}, err error) {
	if err != nil {
		respondError(c, err)
		return
	}
	respondSuccess(c, status, res, nil)
}

// ListGrants lists a user's individual grants (GET /users/:id/grants).
func (ctrl *AccessController) ListGrants(c *gin.Context) {
	tenantID, userID, ok := target(c)
	if !ok {
		return
	}
	res, err := ctrl.svc.ListGrants(c.Request.Context(), ctrl.db, tenantID, userID)
	respond(c, http.StatusOK, res, err)
}

// Grant gives a user a permission at a scope (POST /users/:id/grants).
func (ctrl *AccessController) Grant(c *gin.Context) {
	tenantID, userID, ok := target(c)
	var req dto.UserGrantRequest
	if !ok || !bindValid(c, &req) {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.Grant(reqCtx(c), tx, tenantID, userID, req)
	respond(c, http.StatusCreated, res, database.Finish(tx, err))
}

// RevokeGrant removes an individual grant (DELETE /users/:id/grants/:permission_id).
func (ctrl *AccessController) RevokeGrant(c *gin.Context) {
	tenantID, userID, ok := target(c)
	if !ok {
		return
	}
	permID, ok := permissionParam(c)
	if !ok {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err := database.Finish(tx, ctrl.svc.RevokeGrant(reqCtx(c), tx, tenantID, userID, permID))
	respond(c, http.StatusOK, dto.MessageResponse{Message: "Grant revoked"}, err)
}

// ListBlocks lists a user's blocks (GET /users/:id/blocks).
func (ctrl *AccessController) ListBlocks(c *gin.Context) {
	tenantID, userID, ok := target(c)
	if !ok {
		return
	}
	res, err := ctrl.svc.ListBlocks(c.Request.Context(), ctrl.db, tenantID, userID)
	respond(c, http.StatusOK, res, err)
}

// Block removes a permission from a user whatever their roles grant (POST /users/:id/blocks).
func (ctrl *AccessController) Block(c *gin.Context) {
	tenantID, userID, ok := target(c)
	var req dto.UserBlockRequest
	if !ok || !bindValid(c, &req) {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.svc.Block(reqCtx(c), tx, tenantID, userID, req)
	respond(c, http.StatusCreated, res, database.Finish(tx, err))
}

// Unblock lifts a block (DELETE /users/:id/blocks/:permission_id).
func (ctrl *AccessController) Unblock(c *gin.Context) {
	tenantID, userID, ok := target(c)
	if !ok {
		return
	}
	permID, ok := permissionParam(c)
	if !ok {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err := database.Finish(tx, ctrl.svc.Unblock(reqCtx(c), tx, tenantID, userID, permID))
	respond(c, http.StatusOK, dto.MessageResponse{Message: "Block removed"}, err)
}

// SyncAutomaticRoles reconciles the tenant's automatic employee/manager roles now (POST /access/automatic-roles/sync).
func (ctrl *AccessController) SyncAutomaticRoles(c *gin.Context) {
	tid, ok := c.Get("tenant_id")
	tenantID, ok2 := tid.(uuid.UUID)
	if !ok || !ok2 {
		respondBadRequest(c, "Missing tenant scope")
		return
	}
	added, removed, err := ctrl.automatic.SyncTenant(c.Request.Context(), tenantID)
	respond(c, http.StatusOK, dto.AutomaticRoleSyncResponse{Added: added, Removed: removed}, err)
}
