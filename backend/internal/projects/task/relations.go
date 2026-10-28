package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

var relationTypes = map[string]bool{"blocks": true, "related": true, "duplicates": true}

// lockRelations serialises dependency edits of a tenant: a cycle can run through any project, so one advisory lock
// per tenant is the smallest lock that makes the check-then-insert atomic (PG3).
func lockRelations(tx *gorm.DB, tenantID uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "task_rel:"+tenantID.String()).Error
}

// reaches walks blocks edges breadth first from start and reports whether goal is reachable, within maxGraphWalk nodes.
func reaches(tx *gorm.DB, tenantID, start, goal uuid.UUID) (bool, error) {
	seen := map[uuid.UUID]bool{start: true}
	frontier := []uuid.UUID{start}
	for len(frontier) > 0 {
		var next []uuid.UUID
		err := tx.Raw(`SELECT DISTINCT target_task_id FROM task_relations WHERE tenant_id = ? AND type = 'blocks' AND source_task_id IN ?`,
			tenantID, frontier).Scan(&next).Error
		if err != nil {
			return false, err
		}
		frontier = frontier[:0]
		for _, id := range next {
			if id == goal {
				return true, nil
			}
			if !seen[id] {
				seen[id] = true
				frontier = append(frontier, id)
			}
		}
		if len(seen) > maxGraphWalk {
			return false, kernel.Conflict("DEPENDENCY_LIMIT", "the dependency graph is too large to check")
		}
	}
	return false, nil
}

func (in RelationInput) validate(self uuid.UUID) error {
	if !relationTypes[in.Type] {
		return kernel.Invalid("type", "must be blocks, related or duplicates")
	}
	if in.TargetTaskID == uuid.Nil || in.TargetTaskID == self {
		return kernel.Invalid("target_task_id", "must name another task")
	}
	return nil
}

// AddRelation relates the task to another one the caller can read; blocks may not form a cycle (TK-07).
func (s *Service) AddRelation(ctx context.Context, a api.Actor, id uuid.UUID, in RelationInput) (*Relation, error) {
	src, acc, err := s.locate(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskUpdate); err != nil {
		return nil, err
	}
	if err := in.validate(id); err != nil {
		return nil, err
	}
	dst, _, err := s.locate(ctx, a, in.TargetTaskID)
	if err != nil {
		return nil, kernel.NotFound("target task")
	}
	rel := Relation{ID: uuid.New(), TenantID: a.TenantID, SourceTaskID: src.ID, TargetTaskID: dst.ID, Type: in.Type, CreatedBy: a.UserID,
		CreatedAt: s.now().UTC()}
	if in.Type == "related" && rel.SourceTaskID.String() > rel.TargetTaskID.String() {
		rel.SourceTaskID, rel.TargetTaskID = rel.TargetTaskID, rel.SourceTaskID
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error { return s.insertRelation(unit{ctx, tx, a}, rel, relEnds{src, dst}) })
	if isDup(err) {
		return nil, kernel.Conflict("RELATION_EXISTS", "the tasks are already related this way")
	}
	if err != nil {
		return nil, err
	}
	return &rel, nil
}

// relEnds are the two tasks of a relation.
type relEnds struct{ src, dst *Task }

func (s *Service) insertRelation(u unit, rel Relation, ends relEnds) error {
	tx, a := u.tx, u.a
	if rel.Type == "blocks" {
		if err := lockRelations(tx, a.TenantID); err != nil {
			return err
		}
		cycle, err := reaches(tx, a.TenantID, rel.TargetTaskID, rel.SourceTaskID)
		if err != nil {
			return err
		}
		if cycle {
			return kernel.Conflict("DEPENDENCY_CYCLE", "this dependency would create a cycle")
		}
	}
	if err := tx.Create(&rel).Error; err != nil {
		return err
	}
	return s.noteRelation(u, rel, ends, "added")
}

// noteRelation records the event once and the activity on both tasks.
func (s *Service) noteRelation(u unit, rel Relation, ends relEnds, verb string) error {
	ids := map[string]any{"relation_id": rel.ID, "source_task_id": rel.SourceTaskID, "target_task_id": rel.TargetTaskID, "type": rel.Type}
	if err := s.record(u, ends.src, note{event: "task.relation." + verb, activity: "relation_" + verb, data: ids, payload: ids}); err != nil {
		return err
	}
	return s.record(u, ends.dst, note{activity: "relation_" + verb, data: ids})
}

// RemoveRelation deletes a relation that touches the task (task.update).
func (s *Service) RemoveRelation(ctx context.Context, a api.Actor, id, relationID uuid.UUID) error {
	src, acc, err := s.locate(ctx, a, id)
	if err != nil {
		return err
	}
	if err := acc.Require(api.TaskUpdate); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		var rel Relation
		err := tx.Raw(`DELETE FROM task_relations WHERE id = ? AND tenant_id = ? AND (source_task_id = ? OR target_task_id = ?) RETURNING *`,
			relationID, a.TenantID, id, id).Scan(&rel).Error
		if err != nil {
			return err
		}
		if rel.ID == uuid.Nil {
			return kernel.NotFound("relation")
		}
		other := rel.SourceTaskID
		if other == id {
			other = rel.TargetTaskID
		}
		dst, err := s.row(ctx, tx, a.TenantID, other)
		if err != nil {
			return s.record(unit{ctx, tx, a}, src, note{event: "task.relation.removed", activity: "relation_removed"})
		}
		return s.noteRelation(unit{ctx, tx, a}, rel, relEnds{src, dst}, "removed")
	})
}

// Relations lists the relations of a task; the other end appears only when its project is readable by the caller.
func (s *Service) Relations(ctx context.Context, a api.Actor, id uuid.UUID) ([]RelationView, error) {
	if _, _, err := s.locate(ctx, a, id); err != nil {
		return nil, err
	}
	readable, err := s.auth.ReadableProjectIDs(ctx, s.db, a)
	if err != nil {
		return nil, err
	}
	ok := make(map[uuid.UUID]bool, len(readable))
	for _, p := range readable {
		ok[p] = true
	}
	var rows []RelationView
	err = s.db.WithContext(ctx).Raw(`SELECT r.id, r.type,
			CASE WHEN r.source_task_id = @id THEN 'outgoing' ELSE 'incoming' END AS direction,
			o.id AS other_task_id, p.key || '-' || o.number AS other_key, o.title AS other_title, o.project_id AS project_id,
			(c.category = 'done') AS other_done
		FROM task_relations r JOIN tasks o ON o.id = CASE WHEN r.source_task_id = @id THEN r.target_task_id ELSE r.source_task_id END
		JOIN projects p ON p.id = o.project_id JOIN board_columns c ON c.id = o.column_id
		WHERE r.tenant_id = @tenant AND (r.source_task_id = @id OR r.target_task_id = @id) AND o.deleted_at IS NULL
		ORDER BY r.created_at, r.id LIMIT 500`, map[string]any{"id": id, "tenant": a.TenantID}).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]RelationView, 0, len(rows))
	for _, r := range rows {
		if ok[r.ProjectID] {
			out = append(out, r)
		}
	}
	return out, nil
}
