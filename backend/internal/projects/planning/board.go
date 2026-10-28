package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Board page sizes (VW-01).
const (
	defaultPerColumn = 25
	maxPerColumn     = 100
)

// BoardParams selects the board slice. Cursors holds the opaque next_cursor per column id; Sprint is "", "active", "none"
// or a sprint id and narrows both the counts and the cards.
type BoardParams struct {
	PerColumn int
	Cursors   map[uuid.UUID]string
	Sprint    string
}

// BoardColumn is a column with its total count and one keyset page of cards.
type BoardColumn struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Key        string    `json:"key"`
	Position   int       `json:"position"`
	Color      string    `json:"color"`
	Category   string    `json:"category"`
	WipLimit   *int      `json:"wip_limit"`
	WipHard    bool      `json:"wip_hard"`
	Count      int       `gorm:"-" json:"count"`
	Tasks      []Card    `gorm:"-" json:"tasks"`
	NextCursor *string   `gorm:"-" json:"next_cursor"`
}

// Board is the board read model.
type Board struct {
	ProjectID uuid.UUID     `json:"project_id"`
	Key       string        `json:"key"`
	Mode      string        `json:"mode"`
	PerColumn int           `json:"per_column"`
	Columns   []BoardColumn `json:"columns"`
}

// ClampPerColumn applies the board page default and maximum.
func ClampPerColumn(n int) int {
	if n < 1 {
		return defaultPerColumn
	}
	return min(n, maxPerColumn)
}

// Board returns the columns with counts and a cursor-paged slice of tasks per column; it never loads full history
// (VW-01, PG7, task.read).
func (s *Service) Board(ctx context.Context, a api.Actor, projectID uuid.UUID, p BoardParams) (*Board, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, err
	}
	p.PerColumn = ClampPerColumn(p.PerColumn)
	scope, err := s.sprintScope(ctx, a, projectID, p.Sprint)
	if err != nil {
		return nil, err
	}
	var cols []BoardColumn
	err = s.db.WithContext(ctx).Raw(`SELECT id, name, key, position, color, category, wip_limit, wip_hard FROM board_columns
		WHERE tenant_id = ? AND project_id = ? ORDER BY position, id`, a.TenantID, projectID).Scan(&cols).Error
	if err != nil {
		return nil, err
	}
	counts, err := s.columnCounts(ctx, a.TenantID, projectID, scope)
	if err != nil {
		return nil, err
	}
	for i := range cols {
		cols[i].Count = counts[cols[i].ID]
		if err := s.fillColumn(ctx, boardScope{tenant: a.TenantID, project: projectID, f: scope, p: p}, &cols[i]); err != nil {
			return nil, err
		}
	}
	return &Board{ProjectID: projectID, Key: acc.Project.Key, Mode: acc.Project.Mode, PerColumn: p.PerColumn, Columns: cols}, nil
}

// sprintFilter is an SQL fragment over fixed text plus its bind values.
type sprintFilter struct {
	sql  string
	args []any
}

// sprintScope turns the sprint parameter into a filter: "" none, "none" backlog only, "active" the active sprint, or a
// sprint id of the project (404 when it is not).
func (s *Service) sprintScope(ctx context.Context, a api.Actor, projectID uuid.UUID, raw string) (sprintFilter, error) {
	switch raw {
	case "":
		return sprintFilter{}, nil
	case "none":
		return sprintFilter{sql: ` AND t.sprint_id IS NULL`}, nil
	case "active":
		return sprintFilter{sql: ` AND t.sprint_id IN (SELECT id FROM sprints WHERE tenant_id = ? AND project_id = ? AND status = 'active')`,
			args: []any{a.TenantID, projectID}}, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return sprintFilter{}, kernel.Invalid("sprint", "must be none, active or a sprint id")
	}
	var n int64
	err = s.db.WithContext(ctx).Raw(`SELECT count(*) FROM sprints WHERE id = ? AND tenant_id = ? AND project_id = ?`, id, a.TenantID, projectID).Scan(&n).Error
	if err != nil {
		return sprintFilter{}, err
	}
	if n == 0 {
		return sprintFilter{}, kernel.NotFound("sprint")
	}
	return sprintFilter{sql: ` AND t.sprint_id = ?`, args: []any{id}}, nil
}

func (s *Service) columnCounts(ctx context.Context, tenantID, projectID uuid.UUID, f sprintFilter) (map[uuid.UUID]int, error) {
	var rows []struct {
		ColumnID uuid.UUID
		N        int
	}
	err := s.db.WithContext(ctx).Raw(`SELECT t.column_id, count(*) AS n FROM tasks t WHERE t.tenant_id = ? AND t.project_id = ? AND `+
		liveTask+f.sql+` GROUP BY t.column_id`, append([]any{tenantID, projectID}, f.args...)...).Scan(&rows).Error
	out := make(map[uuid.UUID]int, len(rows))
	for _, r := range rows {
		out[r.ColumnID] = r.N
	}
	return out, err
}

// boardScope is what one column page query needs.
type boardScope struct {
	tenant, project uuid.UUID
	f               sprintFilter
	p               BoardParams
}

// fillColumn loads one keyset page of cards for the column.
func (s *Service) fillColumn(ctx context.Context, bs boardScope, col *BoardColumn) error {
	where := `t.tenant_id = ? AND t.project_id = ? AND t.column_id = ? AND ` + liveTask + bs.f.sql
	args := append([]any{bs.tenant, bs.project, col.ID}, bs.f.args...)
	rows, next, err := pageRank(ctx, s.db, cardQuery{where: where, args: args, limit: bs.p.PerColumn}, bs.p.Cursors[col.ID])
	if err != nil {
		return err
	}
	col.Tasks = rows
	if col.Tasks == nil {
		col.Tasks = []Card{}
	}
	if next != "" {
		col.NextCursor = &next
	}
	return nil
}
