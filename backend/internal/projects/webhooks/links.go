package webhooks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// TaskLinks lists the pull requests, branches and commits linked to a task (task.read).
func (s *Service) TaskLinks(ctx context.Context, a api.Actor, taskID uuid.UUID) ([]ExternalLink, error) {
	_, acc, err := s.tasks.Task(ctx, s.db, a, taskID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, err
	}
	var rows []ExternalLink
	err = s.db.WithContext(ctx).Where("task_id = ? AND tenant_id = ?", taskID, a.TenantID).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// linkTarget is the integration, task and requested link an effect belongs to.
type linkTarget struct {
	it     Integration
	taskID uuid.UUID
	cand   candidate
}

// linkRow is what the upsert reports for a link whose state is new or changed.
type linkRow struct {
	ID       uuid.UUID
	State    string
	Inserted bool
}

// applyLinks writes the links, their activity and their events in one transaction and returns how many links are new or
// changed. Replays of the same provider call change nothing, so they emit nothing (idempotent).
func (s *Service) applyLinks(ctx context.Context, it Integration, cands []candidate) (int, error) {
	changed := 0
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		ids, err := taskIDsByNumber(ctx, tx, it, cands)
		if err != nil {
			return err
		}
		for _, c := range cands {
			taskID, ok := ids[c.number]
			if !ok {
				continue
			}
			row, fresh, err := s.upsertLink(ctx, tx, it, taskID, c)
			if err != nil {
				return err
			}
			if fresh {
				changed++
				if err := s.linkEffects(ctx, tx, linkTarget{it: it, taskID: taskID, cand: c}, row); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return changed, err
}

// taskIDsByNumber finds the tasks of THIS project only: a key of another project never links (tenant and project come
// from the integration row).
func taskIDsByNumber(ctx context.Context, tx *gorm.DB, it Integration, cands []candidate) (map[int]uuid.UUID, error) {
	nums := make([]int, 0, len(cands))
	for _, c := range cands {
		nums = append(nums, c.number)
	}
	var rows []struct {
		ID     uuid.UUID
		Number int
	}
	err := tx.WithContext(ctx).Raw(`SELECT id, number FROM tasks WHERE tenant_id = ? AND project_id = ? AND number IN ? AND deleted_at IS NULL`,
		it.TenantID, it.ProjectID, nums).Scan(&rows).Error
	out := make(map[int]uuid.UUID, len(rows))
	for _, r := range rows {
		out[r.Number] = r.ID
	}
	return out, err
}

func (s *Service) upsertLink(ctx context.Context, tx *gorm.DB, it Integration, taskID uuid.UUID, c candidate) (linkRow, bool, error) {
	now := s.stamp()
	var rows []linkRow
	err := tx.WithContext(ctx).Raw(`INSERT INTO external_links (id, tenant_id, task_id, provider, kind, external_id, url, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (task_id, provider, kind, external_id) DO UPDATE SET state = EXCLUDED.state, url = EXCLUDED.url, updated_at = EXCLUDED.updated_at
		WHERE external_links.state IS DISTINCT FROM EXCLUDED.state
		RETURNING id, state, (xmax = 0) AS inserted`,
		uuid.New(), it.TenantID, taskID, it.Provider, c.kind, c.externalID, c.url, c.state, now, now).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return linkRow{}, false, err
	}
	return rows[0], true, nil
}

// linkEffects records the activity (source "integration", ids and state only) and the events for a new or changed link.
func (s *Service) linkEffects(ctx context.Context, tx *gorm.DB, t linkTarget, row linkRow) error {
	it, taskID, c := t.it, t.taskID, t.cand
	typ := "external_updated"
	if row.Inserted {
		typ = "external_linked"
	}
	data := map[string]any{"link_id": row.ID, "provider": it.Provider, "kind": c.kind, "state": row.State}
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: it.TenantID, ProjectID: it.ProjectID, TaskID: &taskID, Type: typ,
		Data: data, Source: "integration"}); err != nil {
		return err
	}
	payload := map[string]any{"task_id": taskID, "project_id": it.ProjectID, "link_id": row.ID, "provider": it.Provider, "kind": c.kind, "state": row.State}
	if row.Inserted {
		if err := kernel.Emit(ctx, tx, it.TenantID, "task.external_linked", payload); err != nil {
			return err
		}
	}
	if row.State == StateMerged && c.kind == KindPR {
		return kernel.Emit(ctx, tx, it.TenantID, "task.pr_merged", payload)
	}
	return nil
}
