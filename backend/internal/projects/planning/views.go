package planning

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

type viewCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

// CreateView saves a personal view, or one shared with a project (view.share there) (VW-06).
func (s *Service) CreateView(ctx context.Context, a api.Actor, in ViewInput) (*SavedView, error) {
	v := SavedView{ID: uuid.New(), TenantID: a.TenantID, OwnerUserID: a.UserID, ProjectID: in.ProjectID, Version: 1,
		Filters: RawJSON("{}"), Sort: RawJSON("[]")}
	if in.Name == nil || in.Layout == nil {
		return nil, kernel.Invalid("name", "name and layout are required")
	}
	if err := in.apply(&v); err != nil {
		return nil, err
	}
	v.Shared = in.Shared != nil && *in.Shared
	if v.Shared && v.ProjectID == nil {
		return nil, kernel.Invalid("project_id", "a shared view belongs to a project")
	}
	if err := s.checkViewProject(ctx, a, v.ProjectID, v.Shared); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	v.CreatedAt, v.UpdatedAt = now, now
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "view.created", ref{"saved_view", v.ID})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("VIEW_NAME_TAKEN", "you already have a view with this name")
	}
	return &v, err
}

// checkViewProject requires read access to the view project, and view.share when the view is shared.
func (s *Service) checkViewProject(ctx context.Context, a api.Actor, projectID *uuid.UUID, shared bool) error {
	if projectID == nil {
		return nil
	}
	acc, err := s.access(ctx, a, *projectID)
	if err != nil {
		return err
	}
	if shared {
		return acc.Require(api.ViewShare)
	}
	return nil
}

// ListViews returns the caller own views plus the shared views of readable projects (VW-06).
func (s *Service) ListViews(ctx context.Context, a api.Actor, f ViewFilter) ([]SavedView, string, error) {
	var cur viewCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	ids, err := s.authz.ReadableProjectIDs(ctx, s.db, a)
	if err != nil {
		return nil, "", err
	}
	if f.ProjectID != nil {
		if _, err := s.access(ctx, a, *f.ProjectID); err != nil {
			return nil, "", err
		}
	}
	where, args := viewWhere(a, f, cur, ids)
	var rows []SavedView
	err = s.db.WithContext(ctx).Where(where, args...).Order("created_at DESC, id DESC").Limit(f.Limit + 1).Find(&rows).Error
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(viewCursor{At: last.CreatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID.String()})
	}
	return rows, next, nil
}

// viewWhere builds the visibility predicate: own views (project still readable) or shared views of readable projects.
func viewWhere(a api.Actor, f ViewFilter, cur viewCursor, readable []uuid.UUID) (string, []any) {
	arr := idArray(readable)
	where := `tenant_id = ? AND ((owner_user_id = ? AND (project_id IS NULL OR project_id = ANY(?::uuid[])))
		OR (shared AND project_id = ANY(?::uuid[])))`
	args := []any{a.TenantID, a.UserID, arr, arr}
	if f.ProjectID != nil {
		where += ` AND (project_id = ? OR project_id IS NULL)`
		args = append(args, *f.ProjectID)
	}
	if cur.ID != "" {
		where += ` AND (created_at, id) < (?::timestamptz, ?::uuid)`
		args = append(args, cur.At, cur.ID)
	}
	return where, args
}

// loadView finds a view the caller may see: their own, or a shared view of a project they can read. Anything else is 404.
func (s *Service) loadView(ctx context.Context, a api.Actor, id uuid.UUID) (*SavedView, *api.Access, error) {
	var v SavedView
	err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && v.OwnerUserID != a.UserID && !v.Shared) {
		return nil, nil, kernel.NotFound("saved view")
	}
	if err != nil {
		return nil, nil, err
	}
	if v.ProjectID == nil {
		return &v, nil, nil
	}
	acc, err := s.access(ctx, a, *v.ProjectID)
	if err != nil {
		return nil, nil, kernel.NotFound("saved view")
	}
	return &v, &acc, nil
}

// canManageView allows the owner, or a view.share holder for a shared view.
func canManageView(v *SavedView, acc *api.Access, a api.Actor) error {
	if v.OwnerUserID == a.UserID {
		return nil
	}
	if v.Shared && acc != nil {
		return acc.Require(api.ViewShare)
	}
	return kernel.Forbidden("saved view owner")
}

// UpdateView edits a view; sharing it needs view.share in the project (optimistic version).
func (s *Service) UpdateView(ctx context.Context, a api.Actor, id uuid.UUID, in ViewInput) (*SavedView, error) {
	v, acc, err := s.loadView(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if err := canManageView(v, acc, a); err != nil {
		return nil, err
	}
	if in.Version != nil && *in.Version != v.Version {
		return nil, &kernel.VersionConflict{Current: v.Version}
	}
	if err := in.apply(v); err != nil {
		return nil, err
	}
	if in.Shared != nil && *in.Shared != v.Shared {
		if err := s.changeSharing(v, acc, *in.Shared); err != nil {
			return nil, err
		}
	}
	return s.saveView(ctx, a, v)
}

func (s *Service) changeSharing(v *SavedView, acc *api.Access, shared bool) error {
	if shared {
		if acc == nil {
			return kernel.Invalid("shared", "a shared view belongs to a project")
		}
		if err := acc.Require(api.ViewShare); err != nil {
			return err
		}
	}
	v.Shared = shared
	return nil
}

// saveView writes the edit guarded by the version it was read at, so a concurrent edit becomes a 409.
func (s *Service) saveView(ctx context.Context, a api.Actor, v *SavedView) (*SavedView, error) {
	prev := v.Version
	v.UpdatedAt = s.now().UTC()
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE saved_views SET name = ?, layout = ?, filters = ?::jsonb, sort = ?::jsonb, grouping = ?, shared = ?,
			version = version + 1, updated_at = ? WHERE id = ? AND tenant_id = ? AND version = ?`,
			v.Name, v.Layout, string(v.Filters), string(v.Sort), v.Grouping, v.Shared, v.UpdatedAt, v.ID, a.TenantID, prev)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var cur int
			if err := tx.Raw(`SELECT version FROM saved_views WHERE id = ? AND tenant_id = ?`, v.ID, a.TenantID).Scan(&cur).Error; err != nil {
				return err
			}
			return &kernel.VersionConflict{Current: cur}
		}
		s.audit(ctx, tx, a, "view.updated", ref{"saved_view", v.ID})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("VIEW_NAME_TAKEN", "you already have a view with this name")
	}
	v.Version = prev + 1
	return v, err
}

// DeleteView removes a view the caller may manage.
func (s *Service) DeleteView(ctx context.Context, a api.Actor, id uuid.UUID) error {
	v, acc, err := s.loadView(ctx, a, id)
	if err != nil {
		return err
	}
	if err := canManageView(v, acc, a); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM saved_views WHERE id = ? AND tenant_id = ?`, id, a.TenantID).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "view.deleted", ref{"saved_view", id})
		return nil
	})
}
