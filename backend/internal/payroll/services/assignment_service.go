package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/validators"
	"gorm.io/gorm"
)

// nominalMonth is the full-month basis for previews and self breakdowns (no proration).
const nominalMonth = 30

// AssignmentService manages CTC assignments (FR-AS001..AS003).
type AssignmentService interface {
	Assign(ctx context.Context, a Actor, req dto.AssignRequest) (*dto.AssignmentResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)
	Mine(ctx context.Context, a Actor) (*dto.MyAssignmentResponse, error)
	Preview(ctx context.Context, tenantID uuid.UUID, req dto.PreviewRequest) (*dto.BreakdownResponse, error)
}

type assignmentService struct{ base }

// NewAssignmentService builds an AssignmentService.
func NewAssignmentService(d Deps) AssignmentService { return &assignmentService{base{d}} }

func (s *assignmentService) Assign(ctx context.Context, a Actor, req dto.AssignRequest) (*dto.AssignmentResponse, error) {
	if err := validators.ValidateCTC(req.AnnualCTC); err != nil {
		return nil, invalid("annual_ctc", err.Error())
	}
	from, err := validators.ParseDate(req.EffectiveFrom)
	if err != nil {
		return nil, invalid("effective_from", err.Error())
	}
	empID, err := parseUUIDField("employee_id", req.EmployeeID)
	if err != nil {
		return nil, err
	}
	structureID, err := parseUUIDField("structure_id", req.StructureID)
	if err != nil {
		return nil, err
	}
	if _, err := s.employeeByID(ctx, a.TenantID, empID); err != nil {
		return nil, err
	}
	next := &models.Assignment{TenantID: a.TenantID, EmployeeProfileID: empID, StructureID: structureID, AnnualCTC: req.AnnualCTC,
		EffectiveFrom: from, CreatedByUserID: a.UserID}
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.assignTx(ctx, tx, next) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"payroll.assignment_created", "payroll_assignment", next.ID.String(),
		map[string]string{"employee_id": empID.String(), "structure_id": structureID.String(), "effective_from": req.EffectiveFrom}}, nil)
	res := mapAssignment(next)
	return &res, nil
}

// assignTx checks the structure, closes the open assignment and inserts the new one (PY-014).
func (s *assignmentService) assignTx(ctx context.Context, tx *gorm.DB, next *models.Assignment) error {
	st, spec, err := s.loadSpec(ctx, tx, next.TenantID, next.StructureID)
	if err != nil {
		return err
	}
	if st.Status != models.StructureActive {
		return ErrStructureInactive
	}
	if _, err := calc.Breakdown(spec, next.AnnualCTC.Monthly()); err != nil {
		return isOverflow(err)
	}
	if err := s.Repos.Assignments.LockEmployee(ctx, tx, next.TenantID, next.EmployeeProfileID); err != nil {
		return err
	}
	cur, err := s.Repos.Assignments.Current(ctx, tx, next.TenantID, next.EmployeeProfileID)
	if err != nil {
		return err
	}
	if cur != nil {
		if !next.EffectiveFrom.After(cur.EffectiveFrom) {
			return ErrBackdated
		}
		end := next.EffectiveFrom.AddDate(0, 0, -1)
		cur.EffectiveTo = &end
		if err := s.Repos.Assignments.Update(ctx, tx, cur); err != nil {
			return err
		}
	}
	return s.Repos.Assignments.Create(ctx, tx, next)
}

func (s *assignmentService) List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error) {
	empID, err := parseUUIDField("employee_id", employeeID)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Assignments.ListByEmployee(ctx, s.Tx.DB(), tenantID, empID, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.AssignmentResponse, len(rows))
	for i := range rows {
		out[i] = mapAssignment(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Mine returns the caller's current assignment with a full-month breakdown (P8).
func (s *assignmentService) Mine(ctx context.Context, a Actor) (*dto.MyAssignmentResponse, error) {
	emp, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, err
	}
	cur, err := s.Repos.Assignments.Current(ctx, s.Tx.DB(), a.TenantID, emp.ID)
	if err != nil || cur == nil {
		return nil, orNotFound(err)
	}
	b, err := s.breakdown(ctx, a.TenantID, cur.StructureID, cur.AnnualCTC)
	if err != nil {
		return nil, err
	}
	return &dto.MyAssignmentResponse{Assignment: mapAssignment(cur), Breakdown: *b}, nil
}

// Preview prices a CTC on a structure without saving (P9).
func (s *assignmentService) Preview(ctx context.Context, tenantID uuid.UUID, req dto.PreviewRequest) (*dto.BreakdownResponse, error) {
	if err := validators.ValidateCTC(req.AnnualCTC); err != nil {
		return nil, invalid("annual_ctc", err.Error())
	}
	id, err := parseUUIDField("structure_id", req.StructureID)
	if err != nil {
		return nil, err
	}
	return s.breakdown(ctx, tenantID, id, req.AnnualCTC)
}

func (s *assignmentService) breakdown(ctx context.Context, tenantID, structureID uuid.UUID, ctc calc.Money) (*dto.BreakdownResponse, error) {
	_, spec, err := s.loadSpec(ctx, s.Tx.DB(), tenantID, structureID)
	if err != nil {
		return nil, err
	}
	r, err := calc.Compute(calc.Input{Spec: spec, AnnualCTC: ctc, DaysInMonth: nominalMonth, PayableHundredths: nominalMonth * 100})
	if err != nil {
		return nil, isOverflow(err)
	}
	b := mapBreakdown(ctc.Monthly(), r)
	return &b, nil
}
