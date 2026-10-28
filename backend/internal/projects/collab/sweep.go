package collab

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SweepResult counts what one attachment sweep did.
type SweepResult struct {
	Retired int // pending uploads older than the limit and attachments of purged tasks marked deleted
	Removed int // deleted rows whose object is gone and whose row was removed
}

// SweepAttachments retires pending uploads older than olderThan (AT-04) and attachments whose task no longer exists,
// then deletes the object of every deleted attachment and, once the object is gone, its row. It handles one batch of
// up to 500 rows; a failed object delete keeps the row for the next run. Without storage it does nothing.
func (s *Service) SweepAttachments(ctx context.Context, olderThan time.Duration) (SweepResult, error) {
	var out SweepResult
	st, err := s.store()
	if err != nil {
		return out, nil
	}
	res := s.db.WithContext(ctx).Exec(`UPDATE attachments a SET status = 'deleted', deleted_at = ?
		WHERE a.status <> 'deleted' AND ((a.status = 'pending' AND a.created_at < ?)
			OR NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = a.task_id AND t.tenant_id = a.tenant_id))`,
		s.now().UTC(), s.now().UTC().Add(-olderThan))
	if res.Error != nil {
		return out, res.Error
	}
	out.Retired = int(res.RowsAffected)
	var rows []struct {
		ID, TenantID uuid.UUID
		StorageKey   string
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT id, tenant_id, storage_key FROM attachments WHERE status = 'deleted' ORDER BY deleted_at LIMIT ?`,
		sweepBatch).Scan(&rows).Error; err != nil {
		return out, err
	}
	for _, r := range rows {
		if st.Delete(ctx, r.StorageKey) != nil {
			continue
		}
		if err := s.db.WithContext(ctx).Exec(`DELETE FROM attachments WHERE id = ? AND tenant_id = ? AND status = 'deleted'`, r.ID, r.TenantID).Error; err != nil {
			return out, err
		}
		out.Removed++
	}
	return out, nil
}

// Run stops stale timers (TM-03) and sweeps attachments (AT-04) every interval until ctx ends. onErr receives failures;
// a failing sweep never stops the loop.
func (s *Service) Run(ctx context.Context, interval time.Duration, onErr func(error)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx, onErr)
		}
	}
}

func (s *Service) tick(ctx context.Context, onErr func(error)) {
	if _, err := s.StopStaleTimers(ctx, MaxTimerAge); err != nil && onErr != nil {
		onErr(err)
	}
	if _, err := s.SweepAttachments(ctx, PendingMaxAge); err != nil && onErr != nil {
		onErr(err)
	}
}
