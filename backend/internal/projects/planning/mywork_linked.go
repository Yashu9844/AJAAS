package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Linked groups of My work (VW-05): tasks the caller watches or is mentioned in. Their tables belong to the
// collaboration context and are read here by SQL only.
const (
	GroupWatching = "watching"
	GroupMentions = "mentions"
)

var allGroups = append(append([]string{}, WorkGroups...), GroupWatching, GroupMentions)

const watchingPredicate = `EXISTS (SELECT 1 FROM task_watchers w WHERE w.task_id = t.id AND w.tenant_id = t.tenant_id AND w.user_id = ?)`

const mentionPredicate = `EXISTS (SELECT 1 FROM mentions m WHERE m.task_id = t.id AND m.tenant_id = t.tenant_id AND m.user_id = ?)`

func validGroup(g string) bool {
	if _, _, ok := groupWhere(g, dayBounds{}); ok {
		return true
	}
	return isLinked(g)
}

func isLinked(g string) bool { return g == GroupWatching || g == GroupMentions }

func linkedPredicate(g string) string {
	if g == GroupWatching {
		return watchingPredicate
	}
	return mentionPredicate
}

// linkedCounts fills the watching and mentions badge counts.
func (s *Service) linkedCounts(ctx context.Context, a api.Actor, ids []uuid.UUID, c *WorkCounts) error {
	for g, dst := range map[string]*int{GroupWatching: &c.Watching, GroupMentions: &c.Mentions} {
		var n int64
		err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM tasks t JOIN projects p ON p.id = t.project_id AND p.tenant_id = t.tenant_id
			JOIN board_columns c ON c.id = t.column_id WHERE `+workFilter+` AND `+linkedPredicate(g), a.TenantID, idArray(ids), a.UserID).Scan(&n).Error
		if err != nil {
			return err
		}
		*dst = int(n)
	}
	return nil
}

// linkedGroup pages the watched or mentioned tasks, most recently updated first (keyset on updated_at, id).
func (s *Service) linkedGroup(ctx context.Context, w workScope, p WorkParams) ([]Card, string, error) {
	var cur workCursor
	if err := kernel.DecodeCursor(p.Cursor, &cur); err != nil {
		return nil, "", err
	}
	where, args := workFilter+` AND `+linkedPredicate(w.group), []any{w.a.TenantID, idArray(w.ids), w.a.UserID}
	if cur.ID != "" {
		where += ` AND (t.updated_at, t.id) < (?::timestamptz, ?::uuid)`
		args = append(args, cur.At, cur.ID)
	}
	limit := ClampWorkLimit(p.PerGroup)
	rows, err := cardQuery{where: where, args: args, order: `t.updated_at DESC, t.id DESC`, limit: limit + 1}.run(ctx, s.db)
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(workCursor{At: last.UpdatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID.String()})
	}
	return rows, next, nil
}
