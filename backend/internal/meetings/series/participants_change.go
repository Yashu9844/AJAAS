package series

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// writable resolves the series for a write that needs p.
func (s *Service) writable(ctx context.Context, tx *gorm.DB, a api.Actor, seriesID uuid.UUID, p api.Perm) (*Series, api.Access, error) {
	row, err := loadSeries(ctx, tx, a.TenantID, seriesID)
	if err != nil {
		return nil, api.Access{}, err
	}
	acc, err := s.accessFor(ctx, tx, a, row)
	if err != nil {
		return nil, api.Access{}, err
	}
	if !acc.ReadsContent() {
		return nil, api.Access{}, kernel.NotFound("meeting")
	}
	return row, acc, acc.Require(p)
}

// guardOrganizerGrant refuses to hand out the organizer role unless the caller is an organizer or manage_all (T4).
func (s *Service) guardOrganizerGrant(ctx context.Context, tx *gorm.DB, a api.Actor, acc api.Access, in ParticipantInput) error {
	if a.ManageAll || acc.Template == api.RoleOrganizer {
		return nil
	}
	roleID, err := s.roleFor(ctx, tx, a.TenantID, in)
	if err != nil {
		return err
	}
	org, err := s.p.Resources.RoleByTemplate(ctx, tx, a.TenantID, api.RoleOrganizer)
	if err != nil {
		return err
	}
	if roleID == org {
		return kernel.Forbidden("meeting organizer role (only an organizer can grant it)")
	}
	return nil
}

// AddParticipant invites a user (PA-1, PA-4).
func (s *Service) AddParticipant(ctx context.Context, a api.Actor, seriesID uuid.UUID, in ParticipantInput) ([]ParticipantView, error) {
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		row, acc, err := s.writable(ctx, tx, a, seriesID, api.ManageParticipants)
		if err != nil {
			return err
		}
		if err := s.guardOrganizerGrant(ctx, tx, a, acc, in); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&Participant{}).Where("series_id = ? AND active", seriesID).Count(&n).Error; err != nil {
			return err
		}
		if n >= MaxParticipants {
			return kernel.Unprocessable("PARTICIPANT_LIMIT", "a meeting has at most 500 participants")
		}
		if err := s.addParticipant(ctx, tx, a, row, in); err != nil {
			return err
		}
		return s.participantChanged(ctx, tx, a, partChange{row.ID, in.UserID, "participant.added"})
	})
	if err != nil {
		return nil, err
	}
	return s.participantViews(ctx, s.db, a.TenantID, seriesID)
}

// UpdateParticipant changes a participant kind or role; the last organizer cannot lose the role.
func (s *Service) UpdateParticipant(ctx context.Context, a api.Actor, seriesID uuid.UUID, in ParticipantInput) ([]ParticipantView, error) {
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		row, acc, err := s.writable(ctx, tx, a, seriesID, api.ManageParticipants)
		if err != nil {
			return err
		}
		if err := s.memberOf(ctx, tx, seriesID, in.UserID); err != nil {
			return err
		}
		if err := s.guardOrganizerGrant(ctx, tx, a, acc, in); err != nil {
			return err
		}
		if err := s.guardLastOrganizer(ctx, tx, a, seriesID, in); err != nil {
			return err
		}
		if err := s.addParticipant(ctx, tx, a, row, in); err != nil {
			return err
		}
		return s.participantChanged(ctx, tx, a, partChange{row.ID, in.UserID, "participant.changed"})
	})
	if err != nil {
		return nil, err
	}
	return s.participantViews(ctx, s.db, a.TenantID, seriesID)
}

// RemoveParticipant ends a participation and revokes the M0 role; responses and attendance stay as history.
func (s *Service) RemoveParticipant(ctx context.Context, a api.Actor, seriesID, userID uuid.UUID) error {
	return s.inTx(ctx, func(tx *gorm.DB) error {
		row, _, err := s.writable(ctx, tx, a, seriesID, api.ManageParticipants)
		if err != nil {
			return err
		}
		if err := s.memberOf(ctx, tx, seriesID, userID); err != nil {
			return err
		}
		if err := s.guardLastOrganizer(ctx, tx, a, seriesID, ParticipantInput{UserID: userID, Role: api.RoleViewer}); err != nil {
			return err
		}
		if err := s.deactivateMember(ctx, tx, a, seriesID, userID); err != nil {
			return err
		}
		return s.participantChanged(ctx, tx, a, partChange{row.ID, userID, "participant.removed"})
	})
}

func (s *Service) deactivateMember(ctx context.Context, tx *gorm.DB, a api.Actor, seriesID, userID uuid.UUID) error {
	if err := tx.WithContext(ctx).Model(&Participant{}).Where("series_id = ? AND user_id = ?", seriesID, userID).
		Updates(map[string]any{"active": false, "updated_at": s.now().UTC()}).Error; err != nil {
		return err
	}
	return s.p.Resources.RevokeRole(ctx, tx, ref(a.TenantID, seriesID), a.UserID, userID)
}

func (s *Service) memberOf(ctx context.Context, tx *gorm.DB, seriesID, userID uuid.UUID) error {
	var n int64
	if err := tx.WithContext(ctx).Model(&Participant{}).Where("series_id = ? AND user_id = ? AND active", seriesID, userID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return kernel.NotFound("participant")
	}
	return nil
}

// guardLastOrganizer refuses a change that would leave the series without an organizer.
func (s *Service) guardLastOrganizer(ctx context.Context, tx *gorm.DB, a api.Actor, seriesID uuid.UUID, in ParticipantInput) error {
	r := ref(a.TenantID, seriesID)
	role, _, err := s.p.Resources.PermissionsOn(ctx, tx, r, in.UserID)
	if err != nil || role.TemplateKey != api.RoleOrganizer {
		return err
	}
	org, err := s.p.Resources.RoleByTemplate(ctx, tx, a.TenantID, api.RoleOrganizer)
	if err != nil {
		return err
	}
	if next, err := s.roleFor(ctx, tx, a.TenantID, in); err != nil || next == org {
		return err
	}
	n, err := s.p.Resources.CountWithTemplate(ctx, tx, r, api.RoleOrganizer)
	if err != nil {
		return err
	}
	if n <= 1 {
		return kernel.Conflict("LAST_ORGANIZER", "a meeting needs at least one organizer")
	}
	return nil
}

// partChange is a membership change to log, audit and publish.
type partChange struct {
	series, user uuid.UUID
	key          string
}

func (s *Service) participantChanged(ctx context.Context, tx *gorm.DB, a api.Actor, c partChange) error {
	if err := s.logChange(ctx, tx, a, change{series: c.series, action: c.key, fields: map[string]any{"user_id": c.user}}); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, a, auditEntry{"meeting." + c.key, c.series, map[string]string{"user_id": c.user.String()}}); err != nil {
		return err
	}
	return kernel.Emit(ctx, tx, a.TenantID, c.key, map[string]any{"series_id": c.series, "user_id": c.user})
}
