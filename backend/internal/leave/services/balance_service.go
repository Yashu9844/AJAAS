package services

import (
	"context"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/repositories"
	"github.com/jaas/jaas/internal/leave/validators"
	"gorm.io/gorm"
)

// LedgerQuery is B4's filter; employee_id is required.
type LedgerQuery struct {
	EmployeeID  string
	LeaveTypeID string
}

// BalanceService serves balances, HR adjustments and the ledger (FR-BL001..BL005).
type BalanceService interface {
	Mine(ctx context.Context, a Actor) ([]dto.BalanceResponse, error)
	ForEmployee(ctx context.Context, a Actor, employeeID string) ([]dto.BalanceResponse, error)
	Adjust(ctx context.Context, a Actor, req dto.AdjustBalanceRequest) (*dto.BalanceResponse, error)
	Ledger(ctx context.Context, tenantID uuid.UUID, q LedgerQuery, page dto.Page) ([]dto.LedgerEntryResponse, dto.PageMeta, error)
}

type balanceService struct{ base }

// NewBalanceService builds a BalanceService.
func NewBalanceService(d Deps) BalanceService { return &balanceService{base{d}} }

// allTypes caps at one page of 100 active types per tenant (documented limit).
var allTypes = dto.Page{Page: 1, PerPage: dto.MaxPerPage}

func (s *balanceService) Mine(ctx context.Context, a Actor) ([]dto.BalanceResponse, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, err
	}
	return s.balancesFor(ctx, a, emp)
}

func (s *balanceService) ForEmployee(ctx context.Context, a Actor, employeeID string) ([]dto.BalanceResponse, error) {
	emp, err := s.employeeByID(ctx, a.TenantID, employeeID)
	if err != nil {
		return nil, err
	}
	return s.balancesFor(ctx, a, emp)
}

// balancesFor materializes every active type's current-year balance under the employee lock (FR-BL001).
func (s *balanceService) balancesFor(ctx context.Context, a Actor, emp *employeeDTO.EmployeeResponse) ([]dto.BalanceResponse, error) {
	var out []dto.BalanceResponse
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		out = nil
		if err := s.Repos.Balances.LockEmployee(ctx, tx, a.TenantID, emp.ID); err != nil {
			return err
		}
		types, _, err := s.Repos.Types.List(ctx, tx, a.TenantID, models.TypeActive, allTypes)
		if err != nil {
			return err
		}
		e := entitlement{EmployeeID: emp.ID, Joining: joiningOf(emp), Year: s.today().Year()}
		for i := range types {
			bal, err := s.ensure(ctx, tx, a, &types[i], e)
			if err != nil {
				return err
			}
			out = append(out, mapBalance(bal, &types[i]))
		}
		return nil
	})
	return out, err
}

// Adjust applies an HR correction as an `adjustment` ledger row (FR-BL004, LV-017).
func (s *balanceService) Adjust(ctx context.Context, a Actor, req dto.AdjustBalanceRequest) (*dto.BalanceResponse, error) {
	if err := validators.ValidateAdjustment(req.Days); err != nil {
		return nil, invalid("days", err.Error())
	}
	emp, err := s.employeeByID(ctx, a.TenantID, req.EmployeeID)
	if err != nil {
		return nil, err
	}
	var res dto.BalanceResponse
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		typ, err := s.typeByID(ctx, tx, a.TenantID, req.LeaveTypeID)
		if err != nil {
			return err
		}
		if err := s.Repos.Balances.LockEmployee(ctx, tx, a.TenantID, emp.ID); err != nil {
			return err
		}
		bal, err := s.ensure(ctx, tx, a, typ, entitlement{EmployeeID: emp.ID, Joining: joiningOf(emp), Year: s.today().Year()})
		if err != nil {
			return err
		}
		actor, note := a.UserID, req.Reason
		if err := s.move(ctx, tx, bal, movement{Kind: models.KindAdjustment, Days: req.Days, Actor: &actor, Note: &note}); err != nil {
			return err
		}
		res = mapBalance(bal, typ)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.balance_adjusted", "leave_balance", emp.ID.String(),
		map[string]string{"leave_type_id": res.LeaveTypeID.String(), "days": req.Days.String()}}, nil)
	return &res, nil
}

func (s *balanceService) Ledger(ctx context.Context, tenantID uuid.UUID, q LedgerQuery, page dto.Page) ([]dto.LedgerEntryResponse, dto.PageMeta, error) {
	empID, err := parseUUIDField("employee_id", q.EmployeeID)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	f := repositories.LedgerFilter{EmployeeID: empID}
	if q.LeaveTypeID != "" {
		id, err := parseUUIDField("leave_type_id", q.LeaveTypeID)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		f.LeaveTypeID = &id
	}
	rows, total, err := s.Repos.Ledger.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.LedgerEntryResponse, len(rows))
	for i := range rows {
		out[i] = mapLedger(&rows[i])
	}
	return out, page.Meta(total), nil
}
