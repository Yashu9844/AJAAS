package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/events"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/validators"
	"gorm.io/gorm"
)

// RequestQuery is R5's filter.
type RequestQuery struct {
	EmployeeID, Status, From, To string
}

// RequestService runs the leave request lifecycle (FR-LR001..LR006).
type RequestService interface {
	Preview(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.PreviewResponse, error)
	Apply(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.LeaveRequestResponse, error)
	ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)
	List(ctx context.Context, tenantID uuid.UUID, q RequestQuery, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveRequestResponse, error)
	Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveRequestResponse, error)
	Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)
	Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)
}

type requestService struct{ base }

// NewRequestService builds a RequestService.
func NewRequestService(d Deps) RequestService { return &requestService{base{d}} }

// quote is a fully validated, priced request plus its locked balance.
type quote struct {
	typ        *models.LeaveType
	start, end time.Time
	half       *string
	count      calc.Count
	bal        *models.Balance
}

func (s *requestService) Preview(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.PreviewResponse, error) {
	emp, err := s.employeeFor(ctx, a, true)
	if err != nil {
		return nil, err
	}
	var res dto.PreviewResponse
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		q, err := s.quote(ctx, tx, a, quoteInput{emp, req})
		if err != nil {
			return err
		}
		avail := q.bal.Available()
		res = dto.PreviewResponse{TotalDays: q.count.Total, Available: avail, AvailableAfter: avail - q.count.Total,
			WorkingDates: make([]string, len(q.count.WorkingDates))}
		for i, d := range q.count.WorkingDates {
			res.WorkingDates[i] = d.Format(validators.DateLayout)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *requestService) Apply(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.LeaveRequestResponse, error) {
	emp, err := s.employeeFor(ctx, a, true)
	if err != nil {
		return nil, err
	}
	var r *models.Request
	var q *quote
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if q, err = s.quote(ctx, tx, a, quoteInput{emp, req}); err != nil {
			return err
		}
		r = &models.Request{TenantID: a.TenantID, EmployeeProfileID: emp.ID, LeaveTypeID: q.typ.ID, StartDate: q.start,
			EndDate: q.end, HalfDay: q.half, TotalDays: q.count.Total, Reason: req.Reason, Status: models.StatusPending,
			RequestedByUserID: a.UserID}
		if err := s.Repos.Requests.Create(ctx, tx, r); err != nil {
			return err
		}
		actor := a.UserID
		if err := s.move(ctx, tx, q.bal, movement{Kind: models.KindReserve, Days: r.TotalDays, RequestID: &r.ID, Actor: &actor}); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.Applied, requestPayload(r, q.typ))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.applied", "leave_request", r.ID.String(), requestMeta(r)}, []*models.OutboxEvent{row})
	res := mapRequest(r, q.typ.Code)
	return &res, nil
}

type quoteInput struct {
	emp *employeeDTO.EmployeeResponse
	req dto.ApplyLeaveRequest
}

// quote validates LV-002..LV-009 under the employee lock (LV-014) and prices the range.
func (s *requestService) quote(ctx context.Context, tx *gorm.DB, a Actor, in quoteInput) (*quote, error) {
	if err := s.Repos.Balances.LockEmployee(ctx, tx, a.TenantID, in.emp.ID); err != nil {
		return nil, err
	}
	q := &quote{}
	var err error
	if q.typ, err = s.applicableType(ctx, tx, a.TenantID, in); err != nil {
		return nil, err
	}
	if err := s.parseRange(q, in.req); err != nil {
		return nil, err
	}
	if q.count, err = s.price(ctx, tx, a.TenantID, q); err != nil {
		return nil, err
	}
	if err := s.checkOverlap(ctx, tx, a.TenantID, in.emp.ID, q); err != nil {
		return nil, err
	}
	e := entitlement{EmployeeID: in.emp.ID, Joining: joiningOf(in.emp), Year: q.start.Year()}
	if q.bal, err = s.ensure(ctx, tx, a, q.typ, e); err != nil {
		return nil, err
	}
	if q.typ.IsPaid && q.bal.Available() < q.count.Total {
		return nil, ErrInsufficient // LV-009; unpaid types skip the check
	}
	return q, nil
}

func (s *requestService) applicableType(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in quoteInput) (*models.LeaveType, error) {
	typ, err := s.typeByID(ctx, tx, tenantID, in.req.LeaveTypeID)
	switch {
	case err != nil:
		return nil, err
	case typ.Status != models.TypeActive:
		return nil, ErrTypeInactive
	case !validators.GenderApplies(typ.ApplicableGender, in.emp.Gender):
		return nil, ErrTypeNotApplicable
	}
	return typ, nil
}

// parseRange applies LV-003, LV-004 and LV-005.
func (s *requestService) parseRange(q *quote, req dto.ApplyLeaveRequest) error {
	var err error
	if q.start, err = validators.ParseDate(req.StartDate); err != nil {
		return invalid("start_date", err.Error())
	}
	if q.end, err = validators.ParseDate(req.EndDate); err != nil {
		return invalid("end_date", err.Error())
	}
	year := s.today().Year()
	if q.start.After(q.end) || q.start.Year() != year || q.end.Year() != year {
		return ErrInvalidRange
	}
	if req.HalfDay != "" {
		if !q.typ.AllowHalfDay || !q.start.Equal(q.end) {
			return invalid("half_day", "needs a half-day leave type and start_date == end_date")
		}
		half := req.HalfDay
		q.half = &half
	}
	if !validators.NoticeOK(q.start, s.today(), q.typ.MinNoticeDays) {
		return ErrNotice
	}
	return nil
}

// price counts days with the tenant calendar (LV-006, LV-007).
func (s *requestService) price(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, q *quote) (calc.Count, error) {
	holidays, err := s.blockingHolidays(ctx, tx, tenantID, q.start, q.end)
	if err != nil {
		return calc.Count{}, err
	}
	c := calc.CountDays(calc.CountInput{Start: q.start, End: q.end, Half: q.half != nil, Sandwich: q.typ.SandwichRule, Holidays: holidays})
	if c.Total == 0 {
		return c, ErrNoWorkingDays
	}
	if max := q.typ.MaxConsecutiveDays; max != nil && c.Total > calc.FromWhole(*max) {
		return c, ErrTooLong
	}
	return c, nil
}

// blockingHolidays returns non-optional holiday dates in range (optional ones are working days, D4-05).
func (b base) blockingHolidays(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]time.Time, error) {
	rows, err := b.Repos.Holidays.ListRange(ctx, tx, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	var out []time.Time
	for _, h := range rows {
		if !h.IsOptional {
			out = append(out, h.HolidayDate)
		}
	}
	return out, nil
}

// checkOverlap applies LV-008: opposite halves of the same single day may coexist.
func (s *requestService) checkOverlap(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, q *quote) error {
	existing, err := s.Repos.Requests.Overlapping(ctx, tx, tenantID, employeeID, q.start, q.end)
	if err != nil {
		return err
	}
	for _, r := range existing {
		oppositeHalves := q.half != nil && r.HalfDay != nil && *r.HalfDay != *q.half
		if !oppositeHalves {
			return ErrOverlap
		}
	}
	return nil
}
