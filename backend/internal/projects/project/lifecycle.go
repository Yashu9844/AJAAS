package project

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// List returns the projects the caller can read, most recently active first (PR-09).
func (s *Service) List(ctx context.Context, a api.Actor, f ListFilter) ([]Project, string, error) {
	var cur listCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	if f.Status == "" {
		f.Status = "active"
	}
	if f.Status != "active" && f.Status != api.StatusArchived {
		return nil, "", kernel.Invalid("status", "must be active or archived")
	}
	where, args := s.listWhere(a, f, cur)
	var rows []Project
	err := s.db.WithContext(ctx).Raw(`SELECT p.*, (fv.user_id IS NOT NULL) AS favorite, COALESCE(m.role_key, cr.name, '') AS my_role
		FROM projects p
		LEFT JOIN project_favorites fv ON fv.project_id = p.id AND fv.user_id = ?
		LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = ? AND m.active
		LEFT JOIN project_roles cr ON cr.id = m.custom_role_id
		WHERE `+where+` ORDER BY p.last_activity_at DESC, p.id DESC LIMIT ?`,
		append([]any{a.UserID, a.UserID}, append(args, f.Limit+1)...)...).Scan(&rows).Error
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(listCursor{At: last.LastActivityAt.UTC().Format(time.RFC3339Nano), ID: last.ID.String()})
	}
	return rows, next, nil
}

func (s *Service) listWhere(a api.Actor, f ListFilter, cur listCursor) (string, []any) {
	where := `p.tenant_id = ? AND p.deleted_at IS NULL AND p.status = ?
		AND (p.visibility = 'tenant' OR ? OR m.user_id IS NOT NULL)`
	args := []any{a.TenantID, f.Status, a.ReadAll || a.ManageAll}
	if f.Query != "" {
		where += ` AND (p.name ILIKE ? OR p.key ILIKE ?)`
		like := "%" + escapeLike(f.Query) + "%"
		args = append(args, like, like)
	}
	if f.Mine {
		where += ` AND m.user_id IS NOT NULL`
	}
	if f.Favorite {
		where += ` AND fv.user_id IS NOT NULL`
	}
	if cur.ID != "" {
		where += ` AND (p.last_activity_at, p.id) < (?::timestamptz, ?::uuid)`
		args = append(args, cur.At, cur.ID)
	}
	return where, args
}

func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

// Archive hides the project and freezes its tasks; reversible (PR-03).
func (s *Service) Archive(ctx context.Context, a api.Actor, id uuid.UUID) (*Project, error) {
	return s.setArchived(ctx, a, id, true)
}

// Restore reverses Archive.
func (s *Service) Restore(ctx context.Context, a api.Actor, id uuid.UUID) (*Project, error) {
	return s.setArchived(ctx, a, id, false)
}

func (s *Service) setArchived(ctx context.Context, a api.Actor, id uuid.UUID, archive bool) (*Project, error) {
	acc, err := s.Access(ctx, nil, a, id)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ProjectArchive); err != nil {
		return nil, err
	}
	want, event := "active", "project.restored"
	if archive {
		want, event = api.StatusArchived, "project.archived"
	}
	if acc.Project.Status == want {
		return nil, kernel.Conflict("PROJECT_STATE", "the project is already "+want)
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		var at *time.Time
		if archive {
			t := s.now().UTC()
			at = &t
		}
		res := tx.Exec(`UPDATE projects SET status = ?, archived_at = ?, version = version + 1, updated_at = ?, last_activity_at = ?
			WHERE id = ? AND tenant_id = ?`, want, at, s.now().UTC(), s.now().UTC(), id, a.TenantID)
		if res.Error != nil {
			return res.Error
		}
		s.audit(ctx, tx, a, event, "project", id, nil)
		return kernel.Emit(ctx, tx, a.TenantID, event, map[string]any{"project_id": id})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, a, id)
}

// Delete soft-deletes an archived project; only tenant administrators may (PR-03).
func (s *Service) Delete(ctx context.Context, a api.Actor, id uuid.UUID) error {
	acc, err := s.Access(ctx, nil, a, id)
	if err != nil {
		return err
	}
	if !a.ManageAll {
		return kernel.Forbidden("projects:manage_all")
	}
	if acc.Project.Status != api.StatusArchived {
		return kernel.Conflict("PROJECT_NOT_ARCHIVED", "archive the project before deleting it")
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE projects SET deleted_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			s.now().UTC(), s.now().UTC(), id, a.TenantID).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.deleted", "project", id, nil)
		return kernel.Emit(ctx, tx, a.TenantID, "project.deleted", map[string]any{"project_id": id})
	})
}

// SetFavorite stars or unstars the project for the caller.
func (s *Service) SetFavorite(ctx context.Context, a api.Actor, id uuid.UUID, on bool) error {
	if _, err := s.Access(ctx, nil, a, id); err != nil {
		return err
	}
	if on {
		return s.db.WithContext(ctx).Exec(`INSERT INTO project_favorites (project_id, user_id, tenant_id) VALUES (?, ?, ?)
			ON CONFLICT DO NOTHING`, id, a.UserID, a.TenantID).Error
	}
	return s.db.WithContext(ctx).Exec(`DELETE FROM project_favorites WHERE project_id = ? AND user_id = ?`, id, a.UserID).Error
}

// lockProject takes a row lock on the project inside tx and returns it; used by operations that must serialise.
func lockProject(tx *gorm.DB, tenantID, id uuid.UUID) (*Project, error) {
	var p Project
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).Take(&p).Error
	return &p, err
}
