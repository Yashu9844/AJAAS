package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Card is the task summary every read model returns. It is a projection of the tasks table, not the task context model.
type Card struct {
	ID              uuid.UUID         `json:"id"`
	ProjectID       uuid.UUID         `json:"project_id"`
	ProjectKey      string            `json:"project_key"`
	Number          int               `json:"number"`
	Ticket          string            `json:"ticket"`
	Title           string            `json:"title"`
	ColumnID        uuid.UUID         `json:"column_id"`
	Category        string            `json:"category"`
	Priority        string            `json:"priority"`
	StartOn         *kernel.Date      `json:"start_on"`
	DueAt           *time.Time        `json:"due_at"`
	EstimateMinutes *int              `json:"estimate_minutes"`
	MilestoneID     *uuid.UUID        `json:"milestone_id"`
	SprintID        *uuid.UUID        `json:"sprint_id"`
	Rank            string            `json:"rank"`
	CompletedAt     *time.Time        `json:"completed_at"`
	Version         int               `json:"version"`
	UpdatedAt       time.Time         `json:"updated_at"`
	AssigneeIDs     kernel.StringList `gorm:"type:jsonb" json:"assignee_ids"`
	LabelIDs        kernel.StringList `gorm:"type:jsonb" json:"label_ids"`
}

// cardSelect lists the card columns; assignees and labels are folded in as JSON arrays so a page costs one query.
// Every sub-select is tenant scoped.
const cardSelect = `SELECT t.id, t.project_id, p.key AS project_key, t.number, p.key || '-' || t.number AS ticket, t.title,
	t.column_id, c.category, t.priority, t.start_on, t.due_at, t.estimate_minutes, t.milestone_id, t.sprint_id, t.rank,
	t.completed_at, t.version, t.updated_at,
	COALESCE((SELECT jsonb_agg(a.user_id) FROM task_assignees a WHERE a.task_id = t.id AND a.tenant_id = t.tenant_id), '[]'::jsonb) AS assignee_ids,
	COALESCE((SELECT jsonb_agg(l.label_id) FROM task_labels l WHERE l.task_id = t.id AND l.tenant_id = t.tenant_id), '[]'::jsonb) AS label_ids
	FROM tasks t
	JOIN projects p ON p.id = t.project_id AND p.tenant_id = t.tenant_id
	JOIN board_columns c ON c.id = t.column_id`

// cardQuery is a card page: where and order are fixed SQL fragments chosen by the service, never client text; every
// client value travels in args.
type cardQuery struct {
	where string
	args  []any
	order string
	limit int
}

// run executes the page query (limit rows).
func (q cardQuery) run(ctx context.Context, db *gorm.DB) ([]Card, error) {
	var rows []Card
	sql := cardSelect + ` WHERE ` + q.where + ` ORDER BY ` + q.order + ` LIMIT ?`
	err := db.WithContext(ctx).Raw(sql, append(append([]any{}, q.args...), q.limit)...).Scan(&rows).Error
	return rows, err
}

// rankCursor is the keyset position of a rank-ordered page.
type rankCursor struct {
	Rank string `json:"r"`
	ID   string `json:"id"`
}

func rankCursorOf(c Card) string {
	return kernel.EncodeCursor(rankCursor{Rank: c.Rank, ID: c.ID.String()})
}

// rankAfter appends the keyset predicate for a decoded rank cursor.
func rankAfter(where string, args []any, cur rankCursor) (string, []any) {
	if cur.ID == "" {
		return where, args
	}
	return where + ` AND (t.rank, t.id) > (?::text COLLATE "C", ?::uuid)`, append(args, cur.Rank, cur.ID)
}

const rankOrder = `t.rank, t.id`

// openCategories excludes finished work (done and canceled columns).
const openCategories = `c.category NOT IN ('done', 'canceled')`

// liveTask keeps archived and deleted tasks out of every read model.
const liveTask = `t.deleted_at IS NULL AND t.archived_at IS NULL`

// pageRank runs a rank-ordered keyset page and returns the rows plus the next cursor.
func pageRank(ctx context.Context, db *gorm.DB, q cardQuery, cursor string) ([]Card, string, error) {
	var cur rankCursor
	if err := kernel.DecodeCursor(cursor, &cur); err != nil {
		return nil, "", err
	}
	limit := q.limit
	q.where, q.args = rankAfter(q.where, q.args, cur)
	q.order, q.limit = rankOrder, limit+1
	rows, err := q.run(ctx, db)
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, limit)
	next := ""
	if more {
		next = rankCursorOf(rows[len(rows)-1])
	}
	return rows, next, nil
}
