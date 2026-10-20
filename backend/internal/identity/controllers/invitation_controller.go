package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/shared/database"
	sharedMiddleware "github.com/jaas/jaas/internal/shared/middleware"
	"gorm.io/gorm"
)

// InvitationController serves the invitation lifecycle (M0 plan B3).
type InvitationController struct {
	db      *gorm.DB
	users   services.UserService
	invites services.InvitationService
}

// NewInvitationController creates an InvitationController.
func NewInvitationController(db *gorm.DB, users services.UserService, invites services.InvitationService) *InvitationController {
	return &InvitationController{db: db, users: users, invites: invites}
}

func tenantOf(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get("tenant_id")
	id, ok2 := v.(uuid.UUID)
	if !ok || !ok2 {
		respondBadRequest(c, "Missing tenant scope")
		return uuid.Nil, false
	}
	return id, true
}

// Create invites a user: the account is created as invited and the token is sent to them (POST /invitations).
func (ctrl *InvitationController) Create(c *gin.Context) {
	tenantID, ok := tenantOf(c)
	var req dto.InviteUserRequest
	if !ok || !bindValid(c, &req) {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	user, err := ctrl.users.InviteUser(reqCtx(c), tx, tenantID, req, sharedMiddleware.CorrelationOrNew(c.Request.Context()))
	var res *dto.InviteResponse
	if err == nil {
		res = &dto.InviteResponse{User: *user}
		res.Invitation, err = ctrl.invites.Latest(c.Request.Context(), tx, tenantID, uuid.MustParse(user.ID))
	}
	respond(c, http.StatusCreated, res, database.Finish(tx, err))
}

// List shows the tenant's invitations; ?status=pending limits to open ones (GET /invitations).
func (ctrl *InvitationController) List(c *gin.Context) {
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	res, err := ctrl.invites.List(c.Request.Context(), ctrl.db, tenantID, c.Query("status") == "pending")
	respond(c, http.StatusOK, res, err)
}

func (ctrl *InvitationController) withID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	tenantID, ok := tenantOf(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondBadRequest(c, "Invalid invitation ID")
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, id, true
}

// Resend issues a fresh token and revokes the old one (POST /invitations/:id/resend).
func (ctrl *InvitationController) Resend(c *gin.Context) {
	tenantID, id, ok := ctrl.withID(c)
	if !ok {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	res, err := ctrl.invites.Resend(reqCtx(c), tx, tenantID, id)
	respond(c, http.StatusOK, res, database.Finish(tx, err))
}

// Revoke cancels an open invitation (POST /invitations/:id/revoke).
func (ctrl *InvitationController) Revoke(c *gin.Context) {
	tenantID, id, ok := ctrl.withID(c)
	if !ok {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err := database.Finish(tx, ctrl.invites.Revoke(reqCtx(c), tx, tenantID, id))
	respond(c, http.StatusOK, dto.MessageResponse{Message: "Invitation revoked"}, err)
}

// Accept sets the password with the invitation token and activates the account (POST /auth/accept-invitation).
func (ctrl *InvitationController) Accept(c *gin.Context) {
	var req dto.AcceptInvitationRequest
	if !bindValid(c, &req) {
		return
	}
	tx := database.BeginTx(c.Request.Context(), ctrl.db)
	err := ctrl.invites.Accept(c.Request.Context(), tx, req, sharedMiddleware.CorrelationOrNew(c.Request.Context()))
	respond(c, http.StatusOK, dto.MessageResponse{Message: "Invitation accepted; you can now sign in"}, database.Finish(tx, err))
}
