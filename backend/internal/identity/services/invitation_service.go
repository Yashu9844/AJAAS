package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// InvitationTTL is how long an invitation token stays valid.
const InvitationTTL = 7 * 24 * time.Hour

// InvitationService issues, lists, resends, revokes and accepts user invitations (M0 plan B3).
type InvitationService interface {
	Issue(ctx context.Context, tx *gorm.DB, user *models.User) (*models.Invitation, error)
	Latest(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.InvitationResponse, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool) ([]dto.InvitationResponse, error)
	Resend(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.InvitationResponse, error)
	Revoke(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	Accept(ctx context.Context, tx *gorm.DB, req dto.AcceptInvitationRequest, correlationID uuid.UUID) error
}

// InvitationDeps are the invitation service's collaborators.
type InvitationDeps struct {
	Invitations repositories.InvitationRepository
	Users       repositories.UserRepository
	Tokens      TokenService
	Secrets     SecretDelivery
	Sink        EventSink
	Audit       AuditService
	Now         func() time.Time
}

type invitationService struct{ d InvitationDeps }

// NewInvitationService builds an InvitationService.
func NewInvitationService(d InvitationDeps) InvitationService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &invitationService{d: d}
}

var errInvalidInvitation = &sharedErrors.AppError{Code: "INVALID_TOKEN", Message: "The invitation is invalid, used, revoked or expired", StatusCode: 400}

// Issue revokes any open invitation of the user, stores a new hashed token and delivers the raw token.
func (s *invitationService) Issue(ctx context.Context, tx *gorm.DB, user *models.User) (*models.Invitation, error) {
	now := s.d.Now().UTC()
	if err := s.d.Invitations.RevokeOpenForUser(ctx, tx, user.TenantID, user.ID, now); err != nil {
		return nil, err
	}
	raw, hash, err := s.d.Tokens.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}
	inv := &models.Invitation{TenantID: user.TenantID, UserID: user.ID, Email: user.Email, TokenHash: hash,
		ExpiresAt: now.Add(InvitationTTL), CreatedBy: callerID(ctx), CreatedAt: now}
	if err := s.d.Invitations.Create(ctx, tx, inv); err != nil {
		return nil, err
	}
	if s.d.Secrets != nil {
		_ = s.d.Secrets.Deliver(ctx, SecretMessage{Kind: SecretInvitation, TenantID: user.TenantID, UserID: user.ID, Email: user.Email,
			Token: raw, ExpiresAt: inv.ExpiresAt})
	}
	return inv, nil
}

func (s *invitationService) respond(inv *models.Invitation) *dto.InvitationResponse {
	status := "pending"
	switch {
	case inv.AcceptedAt != nil:
		status = "accepted"
	case inv.RevokedAt != nil:
		status = "revoked"
	case !s.d.Now().Before(inv.ExpiresAt):
		status = "expired"
	}
	return &dto.InvitationResponse{ID: inv.ID.String(), UserID: inv.UserID.String(), Email: inv.Email, Status: status,
		ExpiresAt: inv.ExpiresAt, AcceptedAt: inv.AcceptedAt, RevokedAt: inv.RevokedAt, CreatedAt: inv.CreatedAt}
}

func (s *invitationService) Latest(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.InvitationResponse, error) {
	inv, err := s.d.Invitations.LatestForUser(ctx, db, tenantID, userID)
	if err != nil || inv == nil {
		return nil, err
	}
	return s.respond(inv), nil
}

func (s *invitationService) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool) ([]dto.InvitationResponse, error) {
	rows, err := s.d.Invitations.List(ctx, db, tenantID, pendingOnly, s.d.Now())
	if err != nil {
		return nil, err
	}
	out := make([]dto.InvitationResponse, 0, len(rows))
	for i := range rows {
		out = append(out, *s.respond(&rows[i]))
	}
	return out, nil
}

// openInvitation loads an invitation that has not been accepted and whose user is still invited.
func (s *invitationService) openInvitation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Invitation, *models.User, error) {
	inv, err := s.d.Invitations.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, nil, err
	}
	if inv == nil {
		return nil, nil, sharedErrors.ErrNotFound
	}
	if inv.AcceptedAt != nil {
		return nil, nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "The invitation was already accepted", StatusCode: 409}
	}
	user, err := s.d.Users.FindByID(ctx, tx, tenantID, inv.UserID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil || user.Status != "invited" {
		return nil, nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "The user is no longer awaiting an invitation", StatusCode: 409}
	}
	return inv, user, nil
}

func (s *invitationService) Resend(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.InvitationResponse, error) {
	_, user, err := s.openInvitation(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	inv, err := s.Issue(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	_ = s.d.Audit.Log(ctx, tx, tenantID.String(), idString(callerID(ctx)), "invitation.resent", "user", user.ID.String(), nil, "", "")
	return s.respond(inv), nil
}

func (s *invitationService) Revoke(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	inv, _, err := s.openInvitation(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if err := s.d.Invitations.RevokeOpenForUser(ctx, tx, tenantID, inv.UserID, s.d.Now().UTC()); err != nil {
		return err
	}
	_ = s.d.Audit.Log(ctx, tx, tenantID.String(), idString(callerID(ctx)), "invitation.revoked", "user", inv.UserID.String(), nil, "", "")
	return nil
}

func idString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// Accept sets the password and activates the invited user; the token is single use.
func (s *invitationService) Accept(ctx context.Context, tx *gorm.DB, req dto.AcceptInvitationRequest, correlationID uuid.UUID) error {
	inv, err := s.d.Invitations.FindByTokenHash(ctx, tx, s.d.Tokens.HashOpaqueToken(req.Token))
	if err != nil {
		return err
	}
	now := s.d.Now().UTC()
	if inv == nil || !inv.Pending(now) {
		return errInvalidInvitation
	}
	user, err := s.d.Users.FindByID(ctx, tx, inv.TenantID, inv.UserID)
	if err != nil {
		return err
	}
	if user == nil || user.Status != "invited" {
		return errInvalidInvitation
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return err
	}
	user.PasswordHash, user.Status = string(hashed), "active"
	if err := s.d.Users.Update(ctx, tx, user); err != nil {
		return err
	}
	if err := s.d.Invitations.MarkAccepted(ctx, tx, inv.ID, now); err != nil {
		return err
	}
	payload := events.UserActivatedPayload{UserID: user.ID, TenantID: user.TenantID, ActivatedAt: now}
	if err := s.d.Sink.Emit(ctx, tx, events.NewEvent(events.TypeUserActivated, events.RoutingKeyUserActivated, user.TenantID, correlationID, payload)); err != nil {
		return err
	}
	_ = s.d.Audit.Log(ctx, tx, user.TenantID.String(), user.ID.String(), "invitation.accepted", "user", user.ID.String(), nil, "", "")
	return nil
}
