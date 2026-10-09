package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"gorm.io/gorm"
)

// StructureService manages salary structures (FR-ST001..ST003).
type StructureService interface {
	Create(ctx context.Context, a Actor, req dto.CreateStructureRequest) (*dto.StructureResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.StructureResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.StructureResponse, dto.PageMeta, error)
	Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateStructureRequest) (*dto.StructureResponse, error)
	Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.StructureResponse, error)
}

type structureService struct{ base }

// NewStructureService builds a StructureService.
func NewStructureService(d Deps) StructureService { return &structureService{base{d}} }

func (s *structureService) Create(ctx context.Context, a Actor, req dto.CreateStructureRequest) (*dto.StructureResponse, error) {
	st := &models.Structure{TenantID: a.TenantID, Name: strings.TrimSpace(req.Name), Description: req.Description,
		PFEnabled: boolOr(req.PFEnabled, true), ESIEnabled: boolOr(req.ESIEnabled, true), PTEnabled: boolOr(req.PTEnabled, true),
		TDSEnabled: boolOr(req.TDSEnabled, true), Status: models.StructureActive}
	comps := make([]models.Component, len(req.Components))
	for i, c := range req.Components {
		comps[i] = models.Component{Code: strings.ToUpper(strings.TrimSpace(c.Code)), Name: c.Name, Kind: c.Kind, Calc: c.Calc,
			Value: c.Value, Taxable: c.Kind == models.KindEarning && boolOr(c.Taxable, true)}
	}
	if err := calc.ValidateStructure(specFor(st, comps)); err != nil {
		return nil, invalid("components", strings.TrimPrefix(err.Error(), calc.ErrInvalidStructure.Error()+": "))
	}
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.checkName(ctx, tx, st); err != nil {
			return err
		}
		return s.Repos.Structures.Create(ctx, tx, st, comps)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"payroll.structure_created", "payroll_structure", st.ID.String(), map[string]string{"name": st.Name}}, nil)
	res := mapStructure(st, comps)
	return &res, nil
}

func (s *structureService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.StructureResponse, error) {
	st, err := s.Repos.Structures.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || st == nil {
		return nil, orNotFound(err)
	}
	comps, err := s.Repos.Structures.Components(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	res := mapStructure(st, comps)
	return &res, nil
}

func (s *structureService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.StructureResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Structures.List(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.StructureResponse, len(rows))
	for i := range rows {
		comps, err := s.Repos.Structures.Components(ctx, s.Tx.DB(), tenantID, rows[i].ID)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		out[i] = mapStructure(&rows[i], comps)
	}
	return out, page.Meta(total), nil
}

func (s *structureService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateStructureRequest) (*dto.StructureResponse, error) {
	return s.mutate(ctx, a, id, "payroll.structure_updated", func(tx *gorm.DB, st *models.Structure) error {
		if req.Name != nil {
			st.Name = strings.TrimSpace(*req.Name)
		}
		if req.Description != nil {
			st.Description = req.Description
		}
		setBool(&st.PFEnabled, req.PFEnabled)
		setBool(&st.ESIEnabled, req.ESIEnabled)
		setBool(&st.PTEnabled, req.PTEnabled)
		setBool(&st.TDSEnabled, req.TDSEnabled)
		return s.checkName(ctx, tx, st)
	})
}

func (s *structureService) Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.StructureResponse, error) {
	return s.mutate(ctx, a, id, "payroll.structure_deactivated", func(_ *gorm.DB, st *models.Structure) error {
		if st.Status == models.StructureInactive {
			return ErrAlreadyInactive
		}
		st.Status = models.StructureInactive
		return nil
	})
}

// mutate loads, applies and saves a structure inside one transaction, then audits.
func (s *structureService) mutate(ctx context.Context, a Actor, id uuid.UUID, action string, fn func(*gorm.DB, *models.Structure) error) (*dto.StructureResponse, error) {
	var st *models.Structure
	var comps []models.Component
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if st, err = s.Repos.Structures.FindByID(ctx, tx, a.TenantID, id); err != nil || st == nil {
			return orNotFound(err)
		}
		if err := fn(tx, st); err != nil {
			return err
		}
		if err := s.Repos.Structures.Update(ctx, tx, st); err != nil {
			return err
		}
		comps, err = s.Repos.Structures.Components(ctx, tx, a.TenantID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{action, "payroll_structure", st.ID.String(), nil}, nil)
	res := mapStructure(st, comps)
	return &res, nil
}

func (s *structureService) checkName(ctx context.Context, db *gorm.DB, st *models.Structure) error {
	other, err := s.Repos.Structures.FindByName(ctx, db, st.TenantID, st.Name)
	if err != nil {
		return err
	}
	if other != nil && other.ID != st.ID {
		return ErrStructureName
	}
	return nil
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func setBool(dst *bool, p *bool) {
	if p != nil {
		*dst = *p
	}
}

// isOverflow maps the calc overflow to the API error.
func isOverflow(err error) error {
	if errors.Is(err, calc.ErrOverflow) {
		return ErrOverflow
	}
	return err
}
