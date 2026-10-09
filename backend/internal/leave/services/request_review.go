package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/events"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
)

// transition is one locked state change of a request; apply mutates balance/attendance and the request.
type transition struct {
	id         uuid.UUID
	ownerEmpID *uuid.UUID // set for owner actions: foreign requests → 404
	reviewer   bool       // approve/reject: pending only, no self-review
	apply      func(tx *gorm.DB, r *models.Request, bal *models.Balance) error
	routingKey string
	action     string
}

// run loads, locks and re-reads the request, applies the transition and writes the outbox row (LV-014).
func (s *requestService) run(ctx context.Context, a Actor, tr transition) (*dto.LeaveRequestResponse, error) {
	var r *models.Request
	var typ *models.LeaveType
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if r, err = s.lockedRequest(ctx, tx, a, tr); err != nil {
			return err
		}
		if typ, err = s.Repos.Types.FindByID(ctx, tx, a.TenantID, r.LeaveTypeID); err != nil || typ == nil {
			return orNotFound(err)
		}
		bal, _, err := s.Repos.Balances.FindOrCreate(ctx, tx, keyFor(a.TenantID, r.EmployeeProfileID, typ, r.StartDate.Year()))
		if err != nil {
			return err
		}
		if err := tr.apply(tx, r, bal); err != nil {
			return err
		}
		if err := s.Repos.Requests.Update(ctx, tx, r); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, tr.routingKey, requestPayload(r, typ))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{tr.action, "leave_request", r.ID.String(), requestMeta(r)}, []*models.OutboxEvent{row})
	res := mapRequest(r, typ.Code)
	return &res, nil
}

func (s *requestService) lockedRequest(ctx context.Context, tx *gorm.DB, a Actor, tr transition) (*models.Request, error) {
	r, err := s.Repos.Requests.FindByID(ctx, tx, a.TenantID, tr.id)
	if err != nil || r == nil || (tr.ownerEmpID != nil && r.EmployeeProfileID != *tr.ownerEmpID) {
		return nil, orNotFound(err)
	}
	if err := s.Repos.Balances.LockEmployee(ctx, tx, a.TenantID, r.EmployeeProfileID); err != nil {
		return nil, err
	}
	if r, err = s.Repos.Requests.FindByID(ctx, tx, a.TenantID, tr.id); err != nil || r == nil {
		return nil, orNotFound(err)
	}
	if tr.reviewer && r.Status != models.StatusPending {
		return nil, ErrNotPending
	}
	if tr.reviewer && r.RequestedByUserID == a.UserID {
		return nil, ErrSelfApproval // LV-010
	}
	return r, nil
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

// Approve moves reserved days to used and marks attendance (FR-LR003, LV-015).
func (s *requestService) Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error) {
	return s.run(ctx, a, transition{id: id, reviewer: true, routingKey: events.Approved, action: "leave.approved",
		apply: func(tx *gorm.DB, r *models.Request, bal *models.Balance) error {
			if err := s.settle(ctx, tx, a, r, bal); err != nil {
				return err
			}
			dates, err := s.leaveDates(ctx, tx, r)
			if err != nil {
				return err
			}
			if len(dates) > 0 {
				if err := s.Attendance.MarkLeave(ctx, tx, r.TenantID, r.EmployeeProfileID, dates); err != nil {
					return err
				}
			}
			s.review(r, a, models.StatusApproved, req.Comment)
			return nil
		}})
}

// Reject releases the reservation; a comment is required (FR-LR004, LV-010).
func (s *requestService) Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error) {
	if req.Comment == nil {
		return nil, ErrRejectNeedsComment
	}
	return s.run(ctx, a, transition{id: id, reviewer: true, routingKey: events.Rejected, action: "leave.rejected",
		apply: func(tx *gorm.DB, r *models.Request, bal *models.Balance) error {
			actor := a.UserID
			if err := s.move(ctx, tx, bal, movement{Kind: models.KindRelease, Days: -r.TotalDays, RequestID: &r.ID, Actor: &actor}); err != nil {
				return err
			}
			s.review(r, a, models.StatusRejected, req.Comment)
			return nil
		}})
}

// Cancel is the owner's withdrawal (FR-LR005, LV-011).
func (s *requestService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveRequestResponse, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, err
	}
	return s.run(ctx, a, transition{id: id, ownerEmpID: &emp.ID, routingKey: events.Cancelled, action: "leave.cancelled",
		apply: func(tx *gorm.DB, r *models.Request, bal *models.Balance) error {
			if err := s.unwind(ctx, tx, a, r, bal); err != nil {
				return err
			}
			now := s.Now()
			r.Status, r.CancelledAt = models.StatusCancelled, &now
			return nil
		}})
}

// settle turns the reservation into usage: release −d, consume +d.
func (s *requestService) settle(ctx context.Context, tx *gorm.DB, a Actor, r *models.Request, bal *models.Balance) error {
	actor := a.UserID
	if err := s.move(ctx, tx, bal, movement{Kind: models.KindRelease, Days: -r.TotalDays, RequestID: &r.ID, Actor: &actor}); err != nil {
		return err
	}
	return s.move(ctx, tx, bal, movement{Kind: models.KindConsume, Days: r.TotalDays, RequestID: &r.ID, Actor: &actor})
}

// unwind reverses whatever the request holds: a reservation, or future approved usage + attendance marks.
func (s *requestService) unwind(ctx context.Context, tx *gorm.DB, a Actor, r *models.Request, bal *models.Balance) error {
	actor := a.UserID
	switch r.Status {
	case models.StatusPending:
		return s.move(ctx, tx, bal, movement{Kind: models.KindRelease, Days: -r.TotalDays, RequestID: &r.ID, Actor: &actor})
	case models.StatusApproved:
		if !r.StartDate.After(s.today()) {
			return ErrAlreadyStarted
		}
		if err := s.move(ctx, tx, bal, movement{Kind: models.KindReversal, Days: -r.TotalDays, RequestID: &r.ID, Actor: &actor}); err != nil {
			return err
		}
		dates, err := s.leaveDates(ctx, tx, r)
		if err != nil || len(dates) == 0 {
			return err
		}
		return s.Attendance.ClearLeave(ctx, tx, r.TenantID, r.EmployeeProfileID, dates)
	}
	return ErrNotCancellable
}

// leaveDates are the full working days of a request for attendance (LV-015); half days mark nothing (D4-09).
func (s *requestService) leaveDates(ctx context.Context, tx *gorm.DB, r *models.Request) ([]time.Time, error) {
	if r.HalfDay != nil {
		return nil, nil
	}
	holidays, err := s.blockingHolidays(ctx, tx, r.TenantID, r.StartDate, r.EndDate)
	if err != nil {
		return nil, err
	}
	return calc.CountDays(calc.CountInput{Start: r.StartDate, End: r.EndDate, Holidays: holidays}).WorkingDates, nil
}

func (s *requestService) review(r *models.Request, a Actor, status string, comment *string) {
	now, reviewer := s.Now(), a.UserID
	r.Status, r.ReviewerUserID, r.ReviewedAt, r.ReviewComment = status, &reviewer, &now, comment
}
