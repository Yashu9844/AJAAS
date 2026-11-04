package series

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RSVP answers (PA-3).
const (
	RSVPNeedsAction = "needs_action"
	RSVPAccepted    = "accepted"
	RSVPTentative   = "tentative"
	RSVPDeclined    = "declined"
)

// ResponseInput is an answer to the series or to one occurrence.
type ResponseInput struct {
	RSVP    string `json:"rsvp"`
	Comment string `json:"comment"`
}

type response struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID     uuid.UUID  `gorm:"type:uuid"`
	SeriesID     uuid.UUID  `gorm:"type:uuid"`
	OccurrenceID *uuid.UUID `gorm:"type:uuid"`
	UserID       uuid.UUID  `gorm:"type:uuid"`
	RSVP         string     `gorm:"column:rsvp"`
	Comment      string
	RespondedAt  time.Time
}

func (response) TableName() string { return "meeting_responses" }

func seriesResponses(ctx context.Context, db *gorm.DB, seriesID uuid.UUID) (map[uuid.UUID]string, error) {
	var rows []response
	if err := db.WithContext(ctx).Where("series_id = ? AND occurrence_id IS NULL", seriesID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		out[r.UserID] = r.RSVP
	}
	return out, nil
}

// Respond records the caller answer for the series (occurrenceID nil) or for one occurrence, which overrides it.
func (s *Service) Respond(ctx context.Context, a api.Actor, seriesID uuid.UUID, occurrenceID *uuid.UUID, in ResponseInput) error {
	if err := kernel.OneOf("rsvp", in.RSVP, RSVPAccepted, RSVPTentative, RSVPDeclined, RSVPNeedsAction); err != nil {
		return err
	}
	comment, err := kernel.Text("comment", in.Comment, 0, 500)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		row, acc, err := s.writable(ctx, tx, a, seriesID, api.Read)
		if err != nil {
			return err
		}
		if !acc.Participant {
			return kernel.Forbidden("only participants respond")
		}
		r := response{ID: uuid.New(), TenantID: a.TenantID, SeriesID: row.ID, OccurrenceID: occurrenceID, UserID: a.UserID, RSVP: in.RSVP,
			Comment: comment, RespondedAt: s.now().UTC()}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "series_id"}, {Name: "occurrence_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"rsvp", "comment", "responded_at"}),
		}).Create(&r).Error; err != nil {
			return err
		}
		return kernel.Emit(ctx, tx, a.TenantID, "rsvp.changed", map[string]any{"series_id": row.ID, "occurrence_id": occurrenceID,
			"user_id": a.UserID, "rsvp": in.RSVP})
	})
}

// RespondOccurrence answers for one occurrence.
func (s *Service) RespondOccurrence(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in ResponseInput) error {
	_, oi, err := s.Occurrence(ctx, s.db, a, occurrenceID)
	if err != nil {
		return err
	}
	if oi.Status == StatusCancelled {
		return kernel.Conflict("OCCURRENCE_CANCELLED", "the occurrence is cancelled")
	}
	return s.Respond(ctx, a, oi.SeriesID, &oi.ID, in)
}

// ResponseCount is the number of answers per RSVP value for one occurrence (series answers apply unless overridden).
type ResponseCount struct {
	RSVP  string `json:"rsvp"`
	Count int    `json:"count"`
}

// occurrenceRSVP resolves each active participant answer for an occurrence: the occurrence answer, else the series one.
func occurrenceRSVP(ctx context.Context, db *gorm.DB, seriesID, occurrenceID uuid.UUID) (map[uuid.UUID]response, error) {
	var rows []response
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (r.user_id) r.* FROM meeting_responses r
		JOIN meeting_participants p ON p.series_id = r.series_id AND p.user_id = r.user_id AND p.active
		WHERE r.series_id = ? AND (r.occurrence_id = ? OR r.occurrence_id IS NULL)
		ORDER BY r.user_id, r.occurrence_id NULLS LAST`, seriesID, occurrenceID).Scan(&rows).Error
	out := make(map[uuid.UUID]response, len(rows))
	for _, r := range rows {
		out[r.UserID] = r
	}
	return out, err
}
