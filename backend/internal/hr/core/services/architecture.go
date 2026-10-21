package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"gorm.io/gorm"
)

// Architecture limits (CO-001).
const (
	maxName              = 150
	maxDescription       = 2000
	defaultProbationDays = 90
)

// ArchitectureService implements job families, grades and job profiles (CO-001..CO-003).
type ArchitectureService struct{ base }

// NewArchitectureService builds the service.
func NewArchitectureService(d Deps) *ArchitectureService { return &ArchitectureService{base{d}} }

// CreateFamily creates a job family.
func (s *ArchitectureService) CreateFamily(ctx context.Context, a Actor, req dto.CreateFamilyRequest) (*dto.FamilyResponse, error) {
	c, err := code(req.Code)
	if err != nil {
		return nil, err
	}
	name, err := text("name", req.Name, maxName)
	if err != nil {
		return nil, err
	}
	desc, err := optionalText("description", req.Description, maxDescription)
	if err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	f := &models.JobFamily{TenantID: a.TenantID, Code: c, Name: name, Description: desc, Status: models.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := s.create(ctx, a, f, &models.JobFamily{}, c, "job_family"); err != nil {
		return nil, err
	}
	out := familyResponse(f)
	return &out, nil
}

// CreateGrade creates a grade with a unique rank.
func (s *ArchitectureService) CreateGrade(ctx context.Context, a Actor, req dto.CreateGradeRequest) (*dto.GradeResponse, error) {
	c, err := code(req.Code)
	if err != nil {
		return nil, err
	}
	name, err := text("name", req.Name, maxName)
	if err != nil {
		return nil, err
	}
	if req.Rank < 1 {
		return nil, invalid("rank", "must be at least 1")
	}
	now := s.Now().UTC()
	g := &models.Grade{TenantID: a.TenantID, Code: c, Name: name, Rank: req.Rank, Status: models.StatusActive, CreatedAt: now, UpdatedAt: now}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		taken, err := s.Repo.RankTaken(ctx, tx, a.TenantID, g.Rank, uuid.Nil)
		if err != nil {
			return err
		}
		if taken {
			return ErrRankTaken
		}
		return s.insert(ctx, tx, a.TenantID, g, &models.Grade{}, c)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.grade_created", "core_grade", g.ID.String(), map[string]interface{}{"code": g.Code, "rank": g.Rank}})
	out := gradeResponse(g)
	return &out, nil
}

// CreateProfile creates a job profile; its family and default grade must be active (CO-003).
func (s *ArchitectureService) CreateProfile(ctx context.Context, a Actor, req dto.CreateProfileRequest) (*dto.ProfileResponse, error) {
	c, err := code(req.Code)
	if err != nil {
		return nil, err
	}
	title, err := text("title", req.Title, maxName)
	if err != nil {
		return nil, err
	}
	desc, err := optionalText("description", req.Description, maxDescription)
	if err != nil {
		return nil, err
	}
	family, err := parseUUIDField("family_id", req.FamilyID)
	if err != nil {
		return nil, err
	}
	grade, err := parseOptionalUUID("default_grade_id", req.DefaultGradeID)
	if err != nil {
		return nil, err
	}
	if !lifecycle.Categories[req.Category] {
		return nil, invalid("category", "must be staff, management or executive")
	}
	days := defaultProbationDays
	if req.ProbationDays != nil {
		days = *req.ProbationDays
	}
	if days < 0 || days > 365 {
		return nil, invalid("probation_days", "must be 0–365")
	}
	now := s.Now().UTC()
	p := &models.JobProfile{TenantID: a.TenantID, Code: c, Title: title, FamilyID: family, DefaultGradeID: grade, Category: req.Category,
		ProbationDays: days, Description: desc, Status: models.StatusActive, CreatedAt: now, UpdatedAt: now}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.activeRefs(ctx, tx, a.TenantID, &p.FamilyID, p.DefaultGradeID); err != nil {
			return err
		}
		return s.insert(ctx, tx, a.TenantID, p, &models.JobProfile{}, c)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.job_profile_created", "core_job_profile", p.ID.String(), map[string]string{"code": p.Code}})
	out := profileResponse(p)
	return &out, nil
}

// activeRefs checks that a family and grade (when given) exist and are active (CO-003).
func (s *ArchitectureService) activeRefs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, family, grade *uuid.UUID) error {
	if family != nil {
		f, err := s.Repo.Family(ctx, db, tenantID, *family)
		if err != nil || f == nil {
			return orNotFound(err)
		}
		if f.Status != models.StatusActive {
			return ErrInactiveReference
		}
	}
	if grade != nil {
		g, err := s.Repo.Grade(ctx, db, tenantID, *grade)
		if err != nil || g == nil {
			return orNotFound(err)
		}
		if g.Status != models.StatusActive {
			return ErrInactiveReference
		}
	}
	return nil
}

func (s *ArchitectureService) create(ctx context.Context, a Actor, row, model interface{}, c, resource string) error {
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.insert(ctx, tx, a.TenantID, row, model, c) })
	if err == nil {
		s.afterCommit(ctx, a, auditEntry{"core." + resource + "_created", "core_" + resource, idOf(row), map[string]string{"code": c}})
	}
	return err
}

// insert checks the code is free (CO-002) and inserts the row.
func (s *ArchitectureService) insert(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, row, model interface{}, c string) error {
	taken, err := s.Repo.CodeTaken(ctx, tx, model, tenantID, c, uuid.Nil)
	if err != nil {
		return err
	}
	if taken {
		return ErrCodeTaken
	}
	return s.Repo.Create(ctx, tx, row)
}

func idOf(row interface{}) string {
	switch v := row.(type) {
	case *models.JobFamily:
		return v.ID.String()
	case *models.Grade:
		return v.ID.String()
	case *models.JobProfile:
		return v.ID.String()
	}
	return ""
}

// Families lists job families.
func (s *ArchitectureService) Families(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.FamilyResponse, dto.PageMeta, error) {
	if err := statusFilter(status); err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repo.Families(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.FamilyResponse, len(rows))
	for i := range rows {
		out[i] = familyResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Grades lists grades by rank.
func (s *ArchitectureService) Grades(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.GradeResponse, dto.PageMeta, error) {
	if err := statusFilter(status); err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repo.Grades(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.GradeResponse, len(rows))
	for i := range rows {
		out[i] = gradeResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Profiles lists job profiles.
func (s *ArchitectureService) Profiles(ctx context.Context, tenantID uuid.UUID, f repositories.ProfileFilter, page dto.Page) ([]dto.ProfileResponse, dto.PageMeta, error) {
	if err := statusFilter(f.Status); err != nil {
		return nil, dto.PageMeta{}, err
	}
	if f.Category != "" && !lifecycle.Categories[f.Category] {
		return nil, dto.PageMeta{}, invalid("category", "must be staff, management or executive")
	}
	rows, total, err := s.Repo.Profiles(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.ProfileResponse, len(rows))
	for i := range rows {
		out[i] = profileResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Profile gets one job profile.
func (s *ArchitectureService) Profile(ctx context.Context, tenantID, id uuid.UUID) (*dto.ProfileResponse, error) {
	p, err := s.Repo.Profile(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	out := profileResponse(p)
	return &out, nil
}

// UpdateFamily patches name, description and status.
func (s *ArchitectureService) UpdateFamily(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateArchitectureRequest) (*dto.FamilyResponse, error) {
	var f *models.JobFamily
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if f, err = s.Repo.Family(ctx, tx, a.TenantID, id); err != nil || f == nil {
			return orNotFound(err)
		}
		if err := patchCommon(&f.Name, &f.Description, &f.Status, req, req.Name); err != nil {
			return err
		}
		f.UpdatedAt = s.Now().UTC()
		return s.Repo.Save(ctx, tx, f)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.job_family_updated", "core_job_family", f.ID.String(), map[string]string{"status": f.Status}})
	out := familyResponse(f)
	return &out, nil
}

// UpdateGrade patches name, rank and status.
func (s *ArchitectureService) UpdateGrade(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateArchitectureRequest) (*dto.GradeResponse, error) {
	var g *models.Grade
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if g, err = s.Repo.Grade(ctx, tx, a.TenantID, id); err != nil || g == nil {
			return orNotFound(err)
		}
		var none *string
		if err := patchCommon(&g.Name, &none, &g.Status, req, req.Name); err != nil {
			return err
		}
		if req.Rank != nil && *req.Rank != g.Rank {
			taken, err := s.Repo.RankTaken(ctx, tx, a.TenantID, *req.Rank, g.ID)
			if err != nil {
				return err
			}
			if taken {
				return ErrRankTaken
			}
			g.Rank = *req.Rank
		}
		g.UpdatedAt = s.Now().UTC()
		return s.Repo.Save(ctx, tx, g)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.grade_updated", "core_grade", g.ID.String(), map[string]interface{}{"status": g.Status, "rank": g.Rank}})
	out := gradeResponse(g)
	return &out, nil
}

// UpdateProfile patches title, description, family, default grade, category, probation days and status.
func (s *ArchitectureService) UpdateProfile(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateArchitectureRequest) (*dto.ProfileResponse, error) {
	var p *models.JobProfile
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if p, err = s.Repo.Profile(ctx, tx, a.TenantID, id); err != nil || p == nil {
			return orNotFound(err)
		}
		if err := patchCommon(&p.Title, &p.Description, &p.Status, req, req.Title); err != nil {
			return err
		}
		family, err := parseOptionalUUID("family_id", req.FamilyID)
		if err != nil {
			return err
		}
		grade, err := parseOptionalUUID("default_grade_id", req.DefaultGradeID)
		if err != nil {
			return err
		}
		if err := s.activeRefs(ctx, tx, a.TenantID, family, grade); err != nil {
			return err
		}
		if family != nil {
			p.FamilyID = *family
		}
		if grade != nil {
			p.DefaultGradeID = grade
		}
		if req.Category != nil {
			p.Category = *req.Category
		}
		if req.ProbationDays != nil {
			p.ProbationDays = *req.ProbationDays
		}
		p.UpdatedAt = s.Now().UTC()
		return s.Repo.Save(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.job_profile_updated", "core_job_profile", p.ID.String(), map[string]string{"status": p.Status}})
	out := profileResponse(p)
	return &out, nil
}

// patchCommon applies the name/title, description and status shared by the architecture records.
func patchCommon(name *string, desc **string, status *string, req dto.UpdateArchitectureRequest, newName *string) error {
	if newName != nil {
		n, err := text("name", *newName, maxName)
		if err != nil {
			return err
		}
		*name = n
	}
	if req.Description != nil {
		d, err := optionalText("description", req.Description, maxDescription)
		if err != nil {
			return err
		}
		*desc = d
	}
	if req.Status != nil {
		if err := statusFilter(*req.Status); err != nil || *req.Status == "" {
			return invalid("status", "must be active or inactive")
		}
		*status = *req.Status
	}
	return nil
}

func statusFilter(status string) error {
	if status != "" && status != models.StatusActive && status != models.StatusInactive {
		return invalid("status", "must be active or inactive")
	}
	return nil
}

func familyResponse(f *models.JobFamily) dto.FamilyResponse {
	return dto.FamilyResponse{ID: f.ID, Code: f.Code, Name: f.Name, Description: f.Description, Status: f.Status, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt}
}

func gradeResponse(g *models.Grade) dto.GradeResponse {
	return dto.GradeResponse{ID: g.ID, Code: g.Code, Name: g.Name, Rank: g.Rank, Status: g.Status, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
}

func profileResponse(p *models.JobProfile) dto.ProfileResponse {
	return dto.ProfileResponse{ID: p.ID, Code: p.Code, Title: p.Title, FamilyID: p.FamilyID, DefaultGradeID: p.DefaultGradeID, Category: p.Category,
		ProbationDays: p.ProbationDays, Description: p.Description, Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}
