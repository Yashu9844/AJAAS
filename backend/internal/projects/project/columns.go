package project

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// ColumnInput creates or edits a column; nil fields are unchanged on edit.
type ColumnInput struct {
	Name      *string `json:"name"`
	Category  *string `json:"category"`
	Color     *string `json:"color"`
	WipLimit  *int    `json:"wip_limit"`
	ClearWip  bool    `json:"clear_wip_limit"`
	WipHard   *bool   `json:"wip_hard"`
	IsDefault *bool   `json:"is_default"`
}

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

var defaultColumns = []struct{ name, key, category, color string }{
	{"Backlog", "backlog", api.ColumnBacklog, "#94A3B8"},
	{"To do", "todo", api.ColumnTodo, "#3B82F6"},
	{"In progress", "in-progress", api.ColumnInProgress, "#F59E0B"},
	{"Done", "done", api.ColumnDone, "#22C55E"},
}

func validCategory(c string) bool {
	switch c {
	case api.ColumnBacklog, api.ColumnTodo, api.ColumnInProgress, api.ColumnDone, api.ColumnCanceled:
		return true
	}
	return false
}

func createDefaultColumns(tx *gorm.DB, p Project) error {
	for i, d := range defaultColumns {
		c := Column{ID: uuid.New(), TenantID: p.TenantID, ProjectID: p.ID, Name: d.name, Key: d.key, Position: i, Color: d.color,
			Category: d.category, IsDefault: d.key == "todo", CreatedAt: p.CreatedAt, UpdatedAt: p.CreatedAt}
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
	}
	return nil
}

// Columns lists the project columns in order, with open task counts.
func (s *Service) Columns(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Column, error) {
	if _, err := s.Access(ctx, nil, a, projectID); err != nil {
		return nil, err
	}
	var rows []Column
	err := s.db.WithContext(ctx).Raw(`SELECT c.*, (SELECT count(*) FROM tasks t WHERE t.column_id = c.id AND t.deleted_at IS NULL
		AND t.archived_at IS NULL) AS task_count FROM board_columns c WHERE c.project_id = ? AND c.tenant_id = ? ORDER BY c.position`,
		projectID, a.TenantID).Scan(&rows).Error
	return rows, err
}

// CreateColumn appends a column (PR-06).
func (s *Service) CreateColumn(ctx context.Context, a api.Actor, projectID uuid.UUID, in ColumnInput) (*Column, error) {
	if err := s.require(ctx, a, projectID, api.ColumnManage); err != nil {
		return nil, err
	}
	c := Column{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Category: api.ColumnInProgress}
	if err := in.apply(&c); err != nil {
		return nil, err
	}
	if c.Name == "" {
		return nil, kernel.Invalid("name", "is required")
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		var next int
		if err := tx.Raw(`SELECT COALESCE(max(position) + 1, 0) FROM board_columns WHERE project_id = ?`, projectID).Scan(&next).Error; err != nil {
			return err
		}
		key, err := uniqueKey(tx, projectID, c.Name)
		if err != nil {
			return err
		}
		c.Position, c.Key = next, key
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "column.created", ref{"column", c.ID, map[string]any{"project_id": projectID}})
		return nil
	})
	return &c, err
}

func (in ColumnInput) apply(c *Column) error {
	if in.Name != nil {
		name, err := text("name", *in.Name, 1, 60)
		if err != nil {
			return err
		}
		c.Name = name
	}
	if in.Category != nil {
		if !validCategory(*in.Category) {
			return kernel.Invalid("category", "must be backlog, todo, in_progress, done or canceled")
		}
		c.Category = *in.Category
	}
	if in.Color != nil {
		if !colorPattern.MatchString(*in.Color) {
			return kernel.Invalid("color", "must be #RRGGBB")
		}
		c.Color = *in.Color
	}
	if in.WipLimit != nil && *in.WipLimit < 1 {
		return kernel.Invalid("wip_limit", "must be positive")
	}
	if in.WipLimit != nil {
		c.WipLimit = in.WipLimit
	}
	if in.ClearWip {
		c.WipLimit = nil
	}
	if in.WipHard != nil {
		c.WipHard = *in.WipHard
	}
	return nil
}

func uniqueKey(tx *gorm.DB, projectID uuid.UUID, name string) (string, error) {
	base := strings.Trim(slugStrip.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "column"
	}
	key := base
	for i := 2; ; i++ {
		n, err := countRows(tx, "board_columns", "project_id = ? AND key = ?", projectID, key)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return key, nil
		}
		key = base + "-" + itoa(i)
	}
}

// require resolves the project and checks one permission.
func (s *Service) require(ctx context.Context, a api.Actor, projectID uuid.UUID, p api.Perm) error {
	acc, err := s.Access(ctx, nil, a, projectID)
	if err != nil {
		return err
	}
	return acc.Require(p)
}

// UpdateColumn edits a column; the project must keep a todo and a done column (PR-06).
func (s *Service) UpdateColumn(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in ColumnInput) (*Column, error) {
	if err := s.require(ctx, a, projectID, api.ColumnManage); err != nil {
		return nil, err
	}
	var out Column
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&out).Error; err != nil {
			return err
		}
		if err := in.apply(&out); err != nil {
			return err
		}
		out.UpdatedAt = s.now().UTC()
		if in.IsDefault != nil && *in.IsDefault && !out.IsDefault {
			if err := tx.Exec(`UPDATE board_columns SET is_default = FALSE WHERE project_id = ?`, projectID).Error; err != nil {
				return err
			}
			out.IsDefault = true
		}
		if err := tx.Save(&out).Error; err != nil {
			return err
		}
		return ensureCategories(tx, projectID)
	})
	return &out, err
}

// ensureCategories enforces the todo and done columns every project needs.
func ensureCategories(tx *gorm.DB, projectID uuid.UUID) error {
	todo, err := countRows(tx, "board_columns", "project_id = ? AND category = 'todo'", projectID)
	if err != nil {
		return err
	}
	done, err := countRows(tx, "board_columns", "project_id = ? AND category = 'done'", projectID)
	if err != nil {
		return err
	}
	if todo == 0 || done == 0 {
		return kernel.Conflict("COLUMN_INVARIANT", "a project needs at least one todo column and one done column")
	}
	return nil
}

// ReorderColumns sets the display order; ids must list every column exactly once.
func (s *Service) ReorderColumns(ctx context.Context, a api.Actor, projectID uuid.UUID, ids []uuid.UUID) ([]Column, error) {
	if err := s.require(ctx, a, projectID, api.ColumnManage); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		var have []uuid.UUID
		if err := tx.Raw(`SELECT id FROM board_columns WHERE project_id = ?`, projectID).Scan(&have).Error; err != nil {
			return err
		}
		if !sameSet(have, ids) {
			return kernel.Invalid("column_ids", "must list every column of the project exactly once")
		}
		for i, id := range ids {
			if err := tx.Exec(`UPDATE board_columns SET position = ?, updated_at = ? WHERE id = ?`, i, s.now().UTC(), id).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Columns(ctx, a, projectID)
}

func sameSet(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range a {
		seen[id] = true
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
		delete(seen, id)
	}
	return len(seen) == 0
}
