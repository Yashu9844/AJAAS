package series

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Job timings and retention defaults (DMT-14).
const (
	ReminderLead   = 10 * time.Minute
	PurgeAfter     = 30 * 24 * time.Hour
	DraftRetention = 30 * 24 * time.Hour
)

// DeactivateUserMappings ends the participation of a deactivated login (DMT-15): future answers are moot, the M0
// roles on every series are revoked and feed tokens stop working; attendance, notes and answers stay as history.
func (s *Service) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, _ uuid.UUID) error {
	var series []uuid.UUID
	if err := tx.WithContext(ctx).Model(&Participant{}).Where("tenant_id = ? AND user_id = ? AND active", tenantID, userID).
		Pluck("series_id", &series).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Model(&Participant{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Updates(map[string]any{"active": false, "updated_at": s.now().UTC()}).Error; err != nil {
		return err
	}
	for _, id := range series {
		if err := s.p.Resources.RevokeRole(ctx, tx, ref(tenantID, id), uuid.Nil, userID); err != nil {
			return err
		}
	}
	return tx.WithContext(ctx).Exec(`UPDATE meeting_feed_tokens SET revoked_at = ? WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL`,
		s.now().UTC(), tenantID, userID).Error
}

// RemindStartingSoon publishes occurrence.starting_soon once for every occurrence that starts within ReminderLead
// (M10 delivers reminders, DMT-18). It returns the number of events written.
func (s *Service) RemindStartingSoon(ctx context.Context) (int, error) {
	now := s.now().UTC()
	n := 0
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		var rows []Occurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND reminder_sent_at IS NULL AND start_at > ? AND start_at <= ?", StatusScheduled, now, now.Add(ReminderLead)).
			Limit(500).Find(&rows).Error; err != nil {
			return err
		}
		for _, o := range rows {
			if err := kernel.Emit(ctx, tx, o.TenantID, "occurrence.starting_soon", map[string]any{"series_id": o.SeriesID,
				"occurrence_id": o.ID, "start_at": o.StartAt}); err != nil {
				return err
			}
			if err := tx.Model(&Occurrence{}).Where("id = ?", o.ID).Update("reminder_sent_at", now).Error; err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// Purge removes series deleted more than 30 days ago (with their M0 roles) and draft note versions of occurrences
// published more than 30 days ago (RT-1, DMT-14). It returns the number of series removed.
func (s *Service) Purge(ctx context.Context) (int, error) {
	now := s.now().UTC()
	var rows []Series
	if err := s.db.WithContext(ctx).Where("deleted_at < ?", now.Add(-PurgeAfter)).Limit(100).Find(&rows).Error; err != nil {
		return 0, err
	}
	for i := range rows {
		if err := s.purgeSeries(ctx, &rows[i]); err != nil {
			return i, err
		}
	}
	err := s.db.WithContext(ctx).Exec(`DELETE FROM meeting_note_versions n USING meeting_occurrences o
		WHERE n.occurrence_id = o.id AND o.status = ? AND NOT EXISTS (SELECT 1 FROM meeting_minutes m
		WHERE m.occurrence_id = o.id AND m.created_at > ?)`, StatusPublished, now.Add(-DraftRetention)).Error
	return len(rows), err
}

func (s *Service) purgeSeries(ctx context.Context, row *Series) error {
	return s.inTx(ctx, func(tx *gorm.DB) error {
		roles, err := s.p.Resources.RolesOn(ctx, tx, ref(row.TenantID, row.ID))
		if err != nil {
			return err
		}
		for _, r := range roles {
			if err := s.p.Resources.RevokeRole(ctx, tx, ref(row.TenantID, row.ID), uuid.Nil, r.UserID); err != nil {
				return err
			}
		}
		return tx.WithContext(ctx).Exec(`DELETE FROM meeting_series WHERE id = ?`, row.ID).Error
	})
}

// Run starts the background jobs until ctx ends: reminders every minute, window extension and purge every hour.
func (s *Service) Run(ctx context.Context, onErr func(string, error)) {
	tick, hourly := time.NewTicker(time.Minute), time.NewTicker(time.Hour)
	defer tick.Stop()
	defer hourly.Stop()
	report := func(msg string, err error) {
		if err != nil && onErr != nil {
			onErr(msg, err)
		}
	}
	_, err := s.ExtendWindows(ctx)
	report("meeting window extension failed", err)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_, err := s.RemindStartingSoon(ctx)
			report("meeting reminders failed", err)
		case <-hourly.C:
			_, err := s.ExtendWindows(ctx)
			report("meeting window extension failed", err)
			_, err = s.Purge(ctx)
			report("meeting purge failed", err)
		}
	}
}
