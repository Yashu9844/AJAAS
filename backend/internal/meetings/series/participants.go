package series

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ParticipantView is one participant with the role M0 holds for them on the series.
type ParticipantView struct {
	UserID   uuid.UUID  `json:"user_id"`
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Active   bool       `json:"active"`
	Role     string     `json:"role"`
	RoleID   *uuid.UUID `json:"role_id"`
	Template string     `json:"template"`
	RSVP     string     `json:"rsvp"`
}

// roleFor resolves the requested role: role_id, or a predefined role key (default meeting_attendee).
func (s *Service) roleFor(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in ParticipantInput) (uuid.UUID, error) {
	if in.RoleID != nil {
		return *in.RoleID, nil
	}
	key := in.Role
	if key == "" {
		key = api.RoleAttendee
	}
	if err := kernel.OneOf("role", key, api.RoleViewer, api.RoleAttendee, api.RoleNoteTaker, api.RoleOrganizer); err != nil {
		return uuid.Nil, err
	}
	return s.p.Resources.RoleByTemplate(ctx, tx, tenantID, key)
}

// addParticipant upserts the membership row and assigns the M0 role, in tx.
func (s *Service) addParticipant(ctx context.Context, tx *gorm.DB, a api.Actor, row *Series, in ParticipantInput) error {
	if in.Kind == "" {
		in.Kind = KindRequired
	}
	if err := kernel.OneOf("kind", in.Kind, KindRequired, KindOptional); err != nil {
		return err
	}
	u, err := s.p.Users.GetByID(ctx, tx, a.TenantID, in.UserID)
	if err != nil {
		return err
	}
	if !u.Active() {
		return kernel.Unprocessable("PARTICIPANT_INACTIVE", "user "+in.UserID.String()+" is not an active user of this tenant")
	}
	roleID, err := s.roleFor(ctx, tx, a.TenantID, in)
	if err != nil {
		return err
	}
	p := Participant{TenantID: a.TenantID, SeriesID: row.ID, UserID: in.UserID, Kind: in.Kind, Active: true, AddedBy: &a.UserID}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "series_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"kind": in.Kind, "active": true, "updated_at": time.Now().UTC()}),
	}).Create(&p).Error; err != nil {
		return err
	}
	err = s.p.Resources.AssignRole(ctx, tx, ref(a.TenantID, row.ID), identityport.RoleAssignment{ActorID: a.UserID, UserID: in.UserID, RoleID: roleID})
	if errors.Is(err, identityport.ErrRoleNotFound) || errors.Is(err, identityport.ErrRoleNotApplicable) {
		return kernel.Unprocessable("ROLE_NOT_ASSIGNABLE", "the role cannot be assigned on a meeting")
	}
	return err
}

// Participants lists the members of the series with their roles (content readers only).
func (s *Service) Participants(ctx context.Context, a api.Actor, seriesID uuid.UUID) ([]ParticipantView, error) {
	acc, err := s.Access(ctx, s.db, a, seriesID)
	if err != nil {
		return nil, err
	}
	if err := requireContent(acc, "meeting"); err != nil {
		return nil, err
	}
	return s.participantViews(ctx, s.db, a.TenantID, seriesID)
}

func (s *Service) participantViews(ctx context.Context, db *gorm.DB, tenantID, seriesID uuid.UUID) ([]ParticipantView, error) {
	var rows []Participant
	if err := db.WithContext(ctx).Where("series_id = ? AND tenant_id = ?", seriesID, tenantID).Order("created_at, user_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	roles, err := s.p.Resources.RolesOn(ctx, db, ref(tenantID, seriesID))
	if err != nil {
		return nil, err
	}
	byUser := map[uuid.UUID]identityport.ResourceRole{}
	for _, r := range roles {
		byUser[r.UserID] = r
	}
	rsvp, err := seriesResponses(ctx, db, seriesID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	names := s.names(ctx, tenantID, ids)
	out := make([]ParticipantView, 0, len(rows))
	for _, r := range rows {
		v := ParticipantView{UserID: r.UserID, Name: names[r.UserID], Kind: r.Kind, Active: r.Active, RSVP: rsvpOr(rsvp[r.UserID])}
		if role, ok := byUser[r.UserID]; ok {
			id := role.RoleID
			v.Role, v.RoleID, v.Template = role.RoleName, &id, role.TemplateKey
		}
		out = append(out, v)
	}
	return out, nil
}

func rsvpOr(v string) string {
	if v == "" {
		return RSVPNeedsAction
	}
	return v
}

// RoleOption is a role that can be assigned on a meeting.
type RoleOption struct {
	RoleID   uuid.UUID `json:"role_id"`
	Name     string    `json:"name"`
	Template string    `json:"template"`
}

// AssignableRoles lists the tenant roles that apply to meeting series (M0 decides, DMT-09).
func (s *Service) AssignableRoles(ctx context.Context, a api.Actor, seriesID uuid.UUID) ([]RoleOption, error) {
	acc, err := s.Access(ctx, s.db, a, seriesID)
	if err != nil {
		return nil, err
	}
	if err := requireContent(acc, "meeting"); err != nil {
		return nil, err
	}
	roles, err := s.p.Resources.AvailableRoles(ctx, s.db, ref(a.TenantID, seriesID))
	if err != nil {
		return nil, err
	}
	out := make([]RoleOption, 0, len(roles))
	for _, r := range roles {
		out = append(out, RoleOption{RoleID: r.RoleID, Name: r.RoleName, Template: r.TemplateKey})
	}
	return out, nil
}
