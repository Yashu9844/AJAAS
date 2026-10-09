package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/validators"
	"gorm.io/gorm"
)

// TypeService manages leave types (FR-LT001..LT004).
type TypeService interface {
	Create(ctx context.Context, a Actor, req dto.CreateLeaveTypeRequest) (*dto.LeaveTypeResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveTypeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.LeaveTypeResponse, dto.PageMeta, error)
	Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateLeaveTypeRequest) (*dto.LeaveTypeResponse, error)
	Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveTypeResponse, error)
}

type typeService struct{ base }

// NewTypeService builds a TypeService.
func NewTypeService(d Deps) TypeService { return &typeService{base{d}} }

func (s *typeService) Create(ctx context.Context, a Actor, req dto.CreateLeaveTypeRequest) (*dto.LeaveTypeResponse, error) {
	code, err := validators.NormalizeTypeCode(req.Code)
	if err != nil {
		return nil, invalid("code", err.Error())
	}
	if err := checkAmounts(req.AnnualAllowance, req.CarryForwardLimit); err != nil {
		return nil, err
	}
	t := &models.LeaveType{TenantID: a.TenantID, Name: req.Name, Code: code, IsPaid: boolOr(req.IsPaid, true),
		AnnualAllowance: req.AnnualAllowance, Accrual: strOr(req.Accrual, models.AccrualAnnual),
		CarryForwardLimit: req.CarryForwardLimit, MaxConsecutiveDays: req.MaxConsecutiveDays,
		MinNoticeDays: intOr(req.MinNoticeDays, 0), AllowHalfDay: boolOr(req.AllowHalfDay, true),
		SandwichRule: boolOr(req.SandwichRule, false), ApplicableGender: strOr(req.ApplicableGender, models.GenderAll),
		Status: models.TypeActive}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.checkUnique(ctx, tx, t); err != nil {
			return err
		}
		return s.Repos.Types.Create(ctx, tx, t)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.type_created", "leave_type", t.ID.String(), map[string]string{"code": t.Code}}, nil)
	res := mapType(t)
	return &res, nil
}

func (s *typeService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveTypeResponse, error) {
	t, err := s.find(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	res := mapType(t)
	return &res, nil
}

func (s *typeService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.LeaveTypeResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Types.List(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.LeaveTypeResponse, len(rows))
	for i := range rows {
		out[i] = mapType(&rows[i])
	}
	return out, page.Meta(total), nil
}

func (s *typeService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateLeaveTypeRequest) (*dto.LeaveTypeResponse, error) {
	var t *models.LeaveType
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if t, err = s.find(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if err := applyTypeUpdate(t, req); err != nil {
			return err
		}
		if err := s.checkUnique(ctx, tx, t); err != nil {
			return err
		}
		return s.Repos.Types.Update(ctx, tx, t)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.type_updated", "leave_type", t.ID.String(), map[string]string{"code": t.Code}}, nil)
	res := mapType(t)
	return &res, nil
}

func (s *typeService) Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveTypeResponse, error) {
	var t *models.LeaveType
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if t, err = s.find(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if t.Status == models.TypeInactive {
			return ErrTypeAlreadyOff
		}
		t.Status = models.TypeInactive
		return s.Repos.Types.Update(ctx, tx, t)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.type_deactivated", "leave_type", t.ID.String(), nil}, nil)
	res := mapType(t)
	return &res, nil
}

func (s *typeService) find(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.LeaveType, error) {
	t, err := s.Repos.Types.FindByID(ctx, db, tenantID, id)
	if err == nil && t == nil {
		return nil, ErrNotFound
	}
	return t, err
}

// checkUnique enforces FR-LT004 name/code uniqueness (DB unique indexes are the backstop).
func (s *typeService) checkUnique(ctx context.Context, db *gorm.DB, t *models.LeaveType) error {
	byName, err := s.Repos.Types.FindByName(ctx, db, t.TenantID, t.Name)
	if err != nil {
		return err
	}
	if byName != nil && byName.ID != t.ID {
		return ErrTypeNameTaken
	}
	byCode, err := s.Repos.Types.FindByCode(ctx, db, t.TenantID, t.Code)
	if err != nil {
		return err
	}
	if byCode != nil && byCode.ID != t.ID {
		return ErrTypeCodeTaken
	}
	return nil
}

// applyTypeUpdate copies set fields (FR-LT003; code is immutable). max_consecutive_days 0 clears the limit.
func applyTypeUpdate(t *models.LeaveType, req dto.UpdateLeaveTypeRequest) error {
	setStr(&t.Name, req.Name)
	setBool(&t.IsPaid, req.IsPaid)
	setStr(&t.Accrual, req.Accrual)
	setStr(&t.ApplicableGender, req.ApplicableGender)
	setBool(&t.AllowHalfDay, req.AllowHalfDay)
	setBool(&t.SandwichRule, req.SandwichRule)
	if req.MinNoticeDays != nil {
		t.MinNoticeDays = *req.MinNoticeDays
	}
	if req.MaxConsecutiveDays != nil {
		t.MaxConsecutiveDays = req.MaxConsecutiveDays
		if *req.MaxConsecutiveDays == 0 {
			t.MaxConsecutiveDays = nil
		}
	}
	if req.AnnualAllowance != nil {
		t.AnnualAllowance = *req.AnnualAllowance
	}
	if req.CarryForwardLimit != nil {
		t.CarryForwardLimit = *req.CarryForwardLimit
	}
	return checkAmounts(t.AnnualAllowance, t.CarryForwardLimit)
}

func checkAmounts(allowance, carry calc.Days) error {
	if validators.ValidateAllowance(allowance) != nil {
		return invalid("annual_allowance", validators.ErrAmount.Error())
	}
	if validators.ValidateAllowance(carry) != nil {
		return invalid("carry_forward_limit", validators.ErrAmount.Error())
	}
	return nil
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func strOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func setStr(dst *string, p *string) {
	if p != nil {
		*dst = *p
	}
}

func setBool(dst *bool, p *bool) {
	if p != nil {
		*dst = *p
	}
}
