package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// InvitationRepository persists user invitations.
type InvitationRepository interface {
	Create(ctx context.Context, tx *gorm.DB, inv *models.Invitation) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Invitation, error)
	FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.Invitation, error)
	LatestForUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Invitation, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool, now time.Time) ([]models.Invitation, error)
	MarkAccepted(ctx context.Context, tx *gorm.DB, id uuid.UUID, at time.Time) error
	RevokeOpenForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, at time.Time) error
}

type invitationRepository struct{}

// NewInvitationRepository returns an InvitationRepository.
func NewInvitationRepository() InvitationRepository { return invitationRepository{} }

func (invitationRepository) Create(ctx context.Context, tx *gorm.DB, inv *models.Invitation) error {
	return tx.WithContext(ctx).Create(inv).Error
}

func firstInvitation(q *gorm.DB) (*models.Invitation, error) {
	var inv models.Invitation
	err := q.First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (invitationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Invitation, error) {
	return firstInvitation(db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (invitationRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.Invitation, error) {
	return firstInvitation(db.WithContext(ctx).Where("token_hash = ?", hash))
}

func (invitationRepository) LatestForUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Invitation, error) {
	return firstInvitation(db.WithContext(ctx).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Order("created_at DESC"))
}

func (invitationRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool, now time.Time) ([]models.Invitation, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if pendingOnly {
		q = q.Where("accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", now)
	}
	var out []models.Invitation
	err := q.Order("created_at DESC").Limit(500).Find(&out).Error
	return out, err
}

func (invitationRepository) MarkAccepted(ctx context.Context, tx *gorm.DB, id uuid.UUID, at time.Time) error {
	return tx.WithContext(ctx).Model(&models.Invitation{}).Where("id = ? AND accepted_at IS NULL", id).Update("accepted_at", at).Error
}

func (invitationRepository) RevokeOpenForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, at time.Time) error {
	return tx.WithContext(ctx).Model(&models.Invitation{}).
		Where("tenant_id = ? AND user_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", tenantID, userID).
		Update("revoked_at", at).Error
}
