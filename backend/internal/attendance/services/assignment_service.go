package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// AssignmentService binds employees to shifts (FR-SA001..FR-SA003).
type AssignmentService interface {
	Assign(ctx context.Context, a Actor, shiftID uuid.UUID, req dto.AssignShiftRequest) (*dto.AssignmentResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)
}

type assignmentService struct{ base }

// NewAssignmentService builds an AssignmentService.
func NewAssignmentService(d Deps) AssignmentService { return &assignmentService{base{d}} }

func parseRange(req dto.AssignShiftRequest) (time.Time, *time.Time, error) {
	from, err := validators.ParseDate(req.EffectiveFrom)
	if err != nil {
		return time.Time{}, nil, invalid("effective_from", err.Error())
	}
	if req.EffectiveTo == nil {
		return from, nil, nil
	}
	to, err := validators.ParseDate(*req.EffectiveTo)
	if err != nil {
		return time.Time{}, nil, invalid("effective_to", err.Error())
	}
	if to.Before(from) {
		return time.Time{}, nil, invalid("effective_to", "must not be before effective_from")
	}
	return from, &to, nil
}

// Assign creates an assignment; an earlier open-ended one is closed the day before (AT-011).
func (s *assignmentService) Assign(ctx context.Context, a Actor, shiftID uuid.UUID, req dto.AssignShiftRequest) (*dto.AssignmentResponse, error) {
	employeeID, err := parseUUIDField("employee_id", req.EmployeeID)
	if err != nil {
		return nil, err
	}
	from, to, err := parseRange(req)
	if err != nil {
		return nil, err
	}
	shift, err := s.Repos.Shifts.FindByID(ctx, s.Tx.DB(), a.TenantID, shiftID)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if shift.Status != models.ShiftActive {
		return nil, ErrShiftInactive
	}
	if _, err := s.Employees.GetByID(ctx, a.TenantID, employeeID); err != nil {
		if errors.Is(err, sharedErrors.ErrNotFound) {
			return nil, sharedErrors.ErrNotFound
		}
		return nil, err
	}
	created := &models.ShiftAssignment{TenantID: a.TenantID, EmployeeProfileID: employeeID, ShiftID: shiftID,
		EffectiveFrom: from, EffectiveTo: to, CreatedByUserID: &a.UserID}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.Repos.Records.LockEmployee(ctx, tx, a.TenantID, employeeID); err != nil {
			return err
		}
		if err := s.closeOrReject(ctx, tx, created); err != nil {
			return err
		}
		return s.Repos.Assignments.Create(ctx, tx, created)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, "shift.assigned", "shift_assignment", created.ID.String(),
		map[string]string{"employee_id": employeeID.String(), "shift_id": shiftID.String(), "effective_from": req.EffectiveFrom}, nil)
	res := mapAssignment(created)
	return &res, nil
}

// closeOrReject auto-closes one earlier open-ended assignment; any other overlap is a conflict.
func (s *assignmentService) closeOrReject(ctx context.Context, tx *gorm.DB, next *models.ShiftAssignment) error {
	overlaps, err := s.Repos.Assignments.Overlapping(ctx, tx, next.TenantID, next.EmployeeProfileID, next.EffectiveFrom, next.EffectiveTo)
	if err != nil {
		return err
	}
	for i := range overlaps {
		prev := &overlaps[i]
		if prev.EffectiveTo != nil || !prev.EffectiveFrom.Before(next.EffectiveFrom) {
			return ErrAssignmentOverlap
		}
		end := next.EffectiveFrom.AddDate(0, 0, -1)
		prev.EffectiveTo = &end
		if err := s.Repos.Assignments.Update(ctx, tx, prev); err != nil {
			return err
		}
	}
	return nil
}

func (s *assignmentService) List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error) {
	id, err := parseUUIDField("employee_id", employeeID)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Assignments.ListByEmployee(ctx, s.Tx.DB(), tenantID, id, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.AssignmentResponse, len(rows))
	for i := range rows {
		out[i] = mapAssignment(&rows[i])
	}
	return out, page.Meta(total), nil
}
