package series

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// Cancel cancels the series: every future occurrence that is not published is cancelled; history stays.
func (s *Service) Cancel(ctx context.Context, a api.Actor, id uuid.UUID, in CancelInput) (*View, error) {
	reason, err := kernel.Text("reason", in.Reason, 0, 500)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		row, _, err := s.writable(ctx, tx, a, id, api.Cancel)
		if err != nil {
			return err
		}
		if err := kernel.CheckVersion(in.Version, row.Version); err != nil {
			return err
		}
		if err := s.cancelRow(ctx, tx, row, reason); err != nil {
			return err
		}
		if err := s.logChange(ctx, tx, a, change{series: row.ID, action: "series.cancelled", reason: reason}); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, a, "meeting.cancelled", row.ID, nil); err != nil {
			return err
		}
		return kernel.Emit(ctx, tx, a.TenantID, "meeting.cancelled", map[string]any{"series_id": row.ID, "version": row.Version})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, a, id)
}

func (s *Service) cancelRow(ctx context.Context, tx *gorm.DB, row *Series, reason string) error {
	now := s.now().UTC()
	row.Status, row.Version, row.Sequence, row.UpdatedAt, row.WindowUntil = SeriesCancelled, row.Version+1, row.Sequence+1, now, nil
	if err := tx.WithContext(ctx).Save(row).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&Occurrence{}).Where("series_id = ? AND end_at > ? AND status NOT IN ?", row.ID, now,
		[]string{StatusPublished, StatusCancelled}).
		Updates(map[string]any{"status": StatusCancelled, "cancel_reason": reason, "version": gorm.Expr("version + 1"), "updated_at": now}).Error
}

// Delete soft-deletes the series (organizer or manage_all); the purge job removes it after 30 days (RT-1).
func (s *Service) Delete(ctx context.Context, a api.Actor, id uuid.UUID, version int) error {
	return s.inTx(ctx, func(tx *gorm.DB) error {
		row, _, err := s.writable(ctx, tx, a, id, api.Cancel)
		if err != nil {
			return err
		}
		if err := kernel.CheckVersion(version, row.Version); err != nil {
			return err
		}
		now := s.now().UTC()
		if err := tx.WithContext(ctx).Model(&Series{}).Where("id = ?", row.ID).
			Updates(map[string]any{"deleted_at": now, "window_until": nil, "version": row.Version + 1}).Error; err != nil {
			return err
		}
		if err := s.audit(ctx, tx, a, "meeting.deleted", row.ID, nil); err != nil {
			return err
		}
		return kernel.Emit(ctx, tx, a.TenantID, "meeting.cancelled", map[string]any{"series_id": row.ID, "deleted": true})
	})
}
