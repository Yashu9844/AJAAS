package automation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

type runCursor struct {
	At time.Time `json:"a"`
	ID uuid.UUID `json:"i"`
}

// RunFilter narrows and pages the run history of a rule.
type RunFilter struct {
	Status string
	Cursor string
	Limit  int
}

// Runs lists what a rule did, newest first (project.update). Statuses are ok, skipped and failed.
func (s *Service) Runs(ctx context.Context, a api.Actor, projectID, ruleID uuid.UUID, f RunFilter) ([]Run, string, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, "", err
	}
	if _, err := s.load(ctx, a.TenantID, projectID, ruleID); err != nil {
		return nil, "", err
	}
	var cur runCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	if f.Status != "" && f.Status != StatusOK && f.Status != StatusSkipped && f.Status != StatusFailed {
		return nil, "", kernel.Invalid("status", "must be ok, skipped or failed")
	}
	q := s.db.WithContext(ctx).Where("rule_id = ? AND tenant_id = ?", ruleID, a.TenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Cursor != "" {
		q = q.Where("(created_at, id) < (?, ?)", cur.At, cur.ID)
	}
	var rows []Run
	if err := q.Order("created_at DESC, id DESC").Limit(f.Limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(runCursor{last.CreatedAt, last.ID})
	}
	return rows, next, nil
}
