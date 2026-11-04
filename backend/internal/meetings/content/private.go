package content

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// PrivatePoint is a talking point only its author can see (DMT-07): no permission reaches it, not manage_all, not
// read_all, not exports, search, context packs or events.
type PrivatePoint struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid" json:"-"`
	OccurrenceID uuid.UUID `gorm:"type:uuid" json:"occurrence_id"`
	AuthorID     uuid.UUID `gorm:"type:uuid" json:"-"`
	BodyMD       string    `gorm:"column:body_md" json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName maps to meeting_private_points.
func (PrivatePoint) TableName() string { return "meeting_private_points" }

// PointInput is the text of a private point.
type PointInput struct {
	Body string `json:"body"`
}

const maxPrivatePoints = 100

// participantScope resolves an occurrence the caller takes part in; anyone else gets 404.
func (s *Service) participantScope(ctx context.Context, db *gorm.DB, a api.Actor, occurrenceID uuid.UUID) (scope, error) {
	sc, err := s.occurrence(ctx, db, a, occurrenceID)
	if err != nil {
		return scope{}, err
	}
	if !sc.acc.Participant {
		return scope{}, kernel.NotFound("occurrence")
	}
	return sc, nil
}

// PrivatePoints lists the caller own points on an occurrence.
func (s *Service) PrivatePoints(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]PrivatePoint, error) {
	if _, err := s.participantScope(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	var rows []PrivatePoint
	err := s.db.WithContext(ctx).Where("occurrence_id = ? AND author_id = ? AND tenant_id = ?", occurrenceID, a.UserID, a.TenantID).
		Order("created_at, id").Find(&rows).Error
	return rows, err
}

// AddPrivatePoint adds a point for the caller.
func (s *Service) AddPrivatePoint(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in PointInput) (*PrivatePoint, error) {
	body, err := kernel.Text("body", in.Body, 1, 5000)
	if err != nil {
		return nil, err
	}
	if _, err := s.participantScope(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&PrivatePoint{}).Where("occurrence_id = ? AND author_id = ?", occurrenceID, a.UserID).Count(&n).Error; err != nil {
		return nil, err
	}
	if n >= maxPrivatePoints {
		return nil, kernel.Unprocessable("POINT_LIMIT", "at most 100 private points per occurrence")
	}
	p := &PrivatePoint{ID: uuid.New(), TenantID: a.TenantID, OccurrenceID: occurrenceID, AuthorID: a.UserID, BodyMD: body}
	return p, s.db.WithContext(ctx).Create(p).Error
}

// own loads a point of the caller; points of others are a 404.
func (s *Service) own(ctx context.Context, db *gorm.DB, a api.Actor, id uuid.UUID) (*PrivatePoint, error) {
	var p PrivatePoint
	err := db.WithContext(ctx).Where("id = ? AND author_id = ? AND tenant_id = ?", id, a.UserID, a.TenantID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("private point")
	}
	return &p, err
}

// UpdatePrivatePoint replaces the text of the caller own point.
func (s *Service) UpdatePrivatePoint(ctx context.Context, a api.Actor, id uuid.UUID, in PointInput) (*PrivatePoint, error) {
	body, err := kernel.Text("body", in.Body, 1, 5000)
	if err != nil {
		return nil, err
	}
	p, err := s.own(ctx, s.db, a, id)
	if err != nil {
		return nil, err
	}
	p.BodyMD, p.UpdatedAt = body, s.now().UTC()
	return p, s.db.WithContext(ctx).Save(p).Error
}

// DeletePrivatePoint removes the caller own point.
func (s *Service) DeletePrivatePoint(ctx context.Context, a api.Actor, id uuid.UUID) error {
	p, err := s.own(ctx, s.db, a, id)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&PrivatePoint{}, "id = ?", p.ID).Error
}

// PromotePrivatePoint turns the caller point into a shared agenda item (J7 step 2): the private copy is removed.
func (s *Service) PromotePrivatePoint(ctx context.Context, a api.Actor, id uuid.UUID) (*AgendaItem, error) {
	p, err := s.own(ctx, s.db, a, id)
	if err != nil {
		return nil, err
	}
	title := []rune(p.BodyMD)
	if len(title) > 200 {
		title = title[:200]
	}
	t, body := string(title), p.BodyMD
	it, err := s.AddAgendaItem(ctx, a, p.OccurrenceID, AgendaInput{Title: &t, Body: &body})
	if err != nil {
		return nil, err
	}
	return it, s.db.WithContext(ctx).Delete(&PrivatePoint{}, "id = ?", p.ID).Error
}
