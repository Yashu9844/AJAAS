package project

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// CreateInput creates a project (PR-01).
type CreateInput struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Visibility  string       `json:"visibility"`
	Mode        string       `json:"mode"`
	StartOn     *kernel.Date `json:"start_on"`
	DueOn       *kernel.Date `json:"due_on"`
	Icon        string       `json:"icon"`
	Color       string       `json:"color"`
}

// UpdateInput edits a project; nil fields are unchanged. Version, when set, must match (optimistic concurrency).
type UpdateInput struct {
	Name                *string      `json:"name"`
	Description         *string      `json:"description"`
	Visibility          *string      `json:"visibility"`
	Mode                *string      `json:"mode"`
	StartOn             *kernel.Date `json:"start_on"`
	DueOn               *kernel.Date `json:"due_on"`
	Icon                *string      `json:"icon"`
	Color               *string      `json:"color"`
	TimeTrackingEnabled *bool        `json:"time_tracking_enabled"`
	Version             *int         `json:"version"`
}

// ListFilter narrows the project list (PR-09).
type ListFilter struct {
	Status   string
	Query    string
	Mine     bool
	Favorite bool
	Cursor   string
	Limit    int
}

type listCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

func (in *CreateInput) normalise() error {
	in.Key = strings.ToUpper(strings.TrimSpace(in.Key))
	if !keyPattern.MatchString(in.Key) {
		return kernel.Invalid("key", "must be 2-10 uppercase letters or digits and start with a letter")
	}
	var err error
	if in.Name, err = text("name", in.Name, 1, maxName); err != nil {
		return err
	}
	if tooLong(in.Description, maxDescription) {
		return kernel.Invalid("description", "is too long")
	}
	if in.Visibility == "" {
		in.Visibility = "private"
	}
	if in.Mode == "" {
		in.Mode = api.ModeKanban
	}
	return checkShape(in.Visibility, in.Mode, in.Icon, in.Color, in.StartOn, in.DueOn)
}

func checkShape(visibility, mode, icon, color string, start, due *kernel.Date) error {
	switch {
	case visibility != "private" && visibility != api.VisibilityTenant:
		return kernel.Invalid("visibility", "must be private or tenant")
	case mode != api.ModeKanban && mode != api.ModeScrum:
		return kernel.Invalid("mode", "must be kanban or scrum")
	case tooLong(icon, maxIcon):
		return kernel.Invalid("icon", "is too long")
	case !colorPattern.MatchString(color):
		return kernel.Invalid("color", "must be #RRGGBB")
	case start != nil && due != nil && due.Before(*start):
		return kernel.Invalid("due_on", "must not be before start_on")
	}
	return nil
}

// Create makes a project owned by the caller, with the default columns (PR-01).
func (s *Service) Create(ctx context.Context, a api.Actor, in CreateInput) (*Project, error) {
	if !a.CreateProject && !a.ManageAll {
		return nil, kernel.Forbidden("projects:create")
	}
	if err := in.normalise(); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	p := Project{ID: uuid.New(), TenantID: a.TenantID, Key: in.Key, Name: in.Name, Description: in.Description, Status: "active",
		Visibility: in.Visibility, Mode: in.Mode, OwnerUserID: a.UserID, StartOn: in.StartOn, DueOn: in.DueOn, Icon: in.Icon, Color: in.Color,
		TimeTrackingEnabled: true, Version: 1, CreatedBy: a.UserID, CreatedAt: now, UpdatedAt: now, LastActivityAt: now}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		owner := api.RoleOwner
		m := Member{ProjectID: p.ID, UserID: a.UserID, TenantID: a.TenantID, RoleKey: &owner, Active: true, AddedBy: a.UserID, AddedAt: now}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		if err := createDefaultColumns(tx, p); err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.created", "project", p.ID, map[string]any{"key": p.Key})
		return kernel.Emit(ctx, tx, a.TenantID, "project.created", map[string]any{"project_id": p.ID, "key": p.Key, "owner_user_id": a.UserID})
	})
	if isDup(err) {
		return nil, kernel.Conflict("PROJECT_KEY_TAKEN", "a project with this key already exists")
	}
	if err != nil {
		return nil, err
	}
	p.MyRole = api.RoleOwner
	return &p, nil
}

// Get returns a project the caller can read, with their favourite flag and role.
func (s *Service) Get(ctx context.Context, a api.Actor, id uuid.UUID) (*Project, error) {
	acc, err := s.Access(ctx, nil, a, id)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&p).Error; err != nil {
		return nil, err
	}
	p.MyRole = acc.Role
	var n int64
	s.db.WithContext(ctx).Table("project_favorites").Where("project_id = ? AND user_id = ?", id, a.UserID).Count(&n)
	p.Favorite = n > 0
	return &p, nil
}

// Update edits a project (project.update).
func (s *Service) Update(ctx context.Context, a api.Actor, id uuid.UUID, in UpdateInput) (*Project, error) {
	acc, err := s.Access(ctx, nil, a, id)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.ProjectUpdate); err != nil {
		return nil, err
	}
	var out *Project
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		p, err := lockProject(tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if in.Version != nil && *in.Version != p.Version {
			return &kernel.VersionConflict{Current: p.Version}
		}
		if err := in.apply(p); err != nil {
			return err
		}
		p.Version++
		p.UpdatedAt = s.now().UTC()
		if err := tx.Save(p).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "project.updated", "project", id, nil)
		out = p
		return kernel.Emit(ctx, tx, a.TenantID, "project.updated", map[string]any{"project_id": id, "version": p.Version})
	})
	return out, err
}

func (in UpdateInput) apply(p *Project) error {
	var err error
	if in.Name != nil {
		if p.Name, err = text("name", *in.Name, 1, maxName); err != nil {
			return err
		}
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	setStr(&p.Visibility, in.Visibility)
	setStr(&p.Mode, in.Mode)
	setStr(&p.Icon, in.Icon)
	setStr(&p.Color, in.Color)
	if in.StartOn != nil {
		p.StartOn = in.StartOn
	}
	if in.DueOn != nil {
		p.DueOn = in.DueOn
	}
	if in.TimeTrackingEnabled != nil {
		p.TimeTrackingEnabled = *in.TimeTrackingEnabled
	}
	if tooLong(p.Description, maxDescription) {
		return kernel.Invalid("description", "is too long")
	}
	return checkShape(p.Visibility, p.Mode, p.Icon, p.Color, p.StartOn, p.DueOn)
}

func setStr(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}
