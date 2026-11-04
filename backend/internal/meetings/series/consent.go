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

// Consent states (DMT-13).
const (
	ConsentGranted  = "granted"
	ConsentDeclined = "declined"
)

type consent struct {
	TenantID  uuid.UUID `gorm:"type:uuid"`
	SeriesID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	State     string
	Method    string
	DecidedAt time.Time
}

// TableName maps to meeting_consents.
func (consent) TableName() string { return "meeting_consents" }

// ConsentInput is a participant decision on recording and transcription.
type ConsentInput struct {
	State string `json:"state"`
}

// ConsentView is one participant decision.
type ConsentView struct {
	UserID    uuid.UUID  `json:"user_id"`
	Name      string     `json:"name"`
	State     string     `json:"state"` // granted, declined or pending
	DecidedAt *time.Time `json:"decided_at"`
}

// ConsentSummary tells M9 whether it may record: the series allows it and every active participant granted (DMT-13).
type ConsentSummary struct {
	RecordingAllowed bool          `json:"recording_allowed"`
	MayRecord        bool          `json:"may_record"`
	Participants     []ConsentView `json:"participants"`
}

// SetConsent records the caller decision; only participants decide, and only for themselves.
func (s *Service) SetConsent(ctx context.Context, a api.Actor, seriesID uuid.UUID, in ConsentInput) (*ConsentSummary, error) {
	if err := kernel.OneOf("state", in.State, ConsentGranted, ConsentDeclined); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		row, acc, err := s.writable(ctx, tx, a, seriesID, api.Read)
		if err != nil {
			return err
		}
		if !acc.Participant {
			return kernel.Forbidden("only participants decide on consent")
		}
		c := consent{TenantID: a.TenantID, SeriesID: row.ID, UserID: a.UserID, State: in.State, Method: "api", DecidedAt: s.now().UTC()}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "series_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"state", "method", "decided_at"}),
		}).Create(&c).Error; err != nil {
			return err
		}
		return s.audit(ctx, tx, a, auditEntry{"meeting.consent." + in.State, row.ID, nil})
	})
	if err != nil {
		return nil, err
	}
	return s.Consents(ctx, a, seriesID)
}

// Consents returns every participant decision; content readers only.
func (s *Service) Consents(ctx context.Context, a api.Actor, seriesID uuid.UUID) (*ConsentSummary, error) {
	row, err := loadSeries(ctx, s.db, a.TenantID, seriesID)
	if err != nil {
		return nil, err
	}
	acc, err := s.accessFor(ctx, s.db, a, row)
	if err != nil {
		return nil, err
	}
	if err := requireContent(acc, "meeting"); err != nil {
		return nil, err
	}
	parts, err := s.participantViews(ctx, s.db, a.TenantID, seriesID)
	if err != nil {
		return nil, err
	}
	var rows []consent
	if err := s.db.WithContext(ctx).Where("series_id = ?", seriesID).Find(&rows).Error; err != nil {
		return nil, err
	}
	by := map[uuid.UUID]consent{}
	for _, c := range rows {
		by[c.UserID] = c
	}
	out := &ConsentSummary{RecordingAllowed: row.RecordingAllowed, MayRecord: row.RecordingAllowed, Participants: []ConsentView{}}
	for _, p := range parts {
		if !p.Active {
			continue
		}
		v := ConsentView{UserID: p.UserID, Name: p.Name, State: "pending"}
		if c, ok := by[p.UserID]; ok {
			at := c.DecidedAt
			v.State, v.DecidedAt = c.State, &at
		}
		out.MayRecord = out.MayRecord && v.State == ConsentGranted
		out.Participants = append(out.Participants, v)
	}
	return out, nil
}
