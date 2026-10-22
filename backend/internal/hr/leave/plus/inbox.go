package plus

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	leaveDTO "github.com/jaas/jaas/internal/hr/leave/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

const (
	maxInbox        = 200
	maxCalendarDays = 62
)

// InboxItem is one pending leave request for a manager (LP-06).
type InboxItem struct {
	ID           uuid.UUID `json:"id"`
	EmployeeID   uuid.UUID `json:"employee_id"`
	EmployeeName string    `json:"employee_name"`
	LeaveTypeID  uuid.UUID `json:"leave_type_id"`
	StartDate    Date      `json:"start_date"`
	EndDate      Date      `json:"end_date"`
	HalfDay      *string   `json:"half_day,omitempty"`
	TotalDays    calc.Days `json:"total_days"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Service) name(ctx context.Context, tenantID, id uuid.UUID, cache map[uuid.UUID]string) (string, error) {
	if n, ok := cache[id]; ok {
		return n, nil
	}
	e, err := s.p.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) {
		cache[id] = ""
		return "", nil
	}
	if err != nil {
		return "", err
	}
	cache[id] = e.DisplayName
	return e.DisplayName, nil
}

// Inbox is LP-06: pending requests of the caller's reports, or every pending one for tenant-scope approvers.
func (s *Service) Inbox(ctx context.Context, a Actor, all bool) ([]InboxItem, error) {
	if all && !a.CanApproveAll {
		return nil, appErr(http.StatusForbidden, "FORBIDDEN", "scope=all needs leave:approve at tenant scope")
	}
	var ids []uuid.UUID
	if !all {
		reports, err := s.reportIDs(ctx, a)
		if err != nil {
			return nil, err
		}
		for id := range reports {
			ids = append(ids, id)
		}
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.PendingRequests(ctx, s.store.DB(), a.TenantID, ids, all, maxInbox)
	if err != nil {
		return nil, err
	}
	out, names := make([]InboxItem, 0, len(rows)), map[uuid.UUID]string{}
	for _, r := range rows {
		if me != nil && r.EmployeeProfileID == me.ID {
			continue
		}
		n, err := s.name(ctx, a.TenantID, r.EmployeeProfileID, names)
		if err != nil {
			return nil, err
		}
		out = append(out, InboxItem{ID: r.ID, EmployeeID: r.EmployeeProfileID, EmployeeName: n, LeaveTypeID: r.LeaveTypeID, StartDate: Date{r.StartDate},
			EndDate: Date{r.EndDate}, HalfDay: r.HalfDay, TotalDays: r.TotalDays, Reason: r.Reason, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Decide is LP-06: the report's manager (or a tenant-scope approver) decides through LV-010.
func (s *Service) Decide(ctx context.Context, a Actor, id uuid.UUID, approve bool, comment *string) (*leaveDTO.LeaveRequestResponse, error) {
	r, err := s.store.Request(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	if ok, err := s.mayDecide(ctx, a, r.EmployeeProfileID); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}
	req := leaveDTO.ReviewRequest{Comment: comment}
	if approve {
		return s.p.Requests.Approve(ctx, a.leaveActor(), id, req)
	}
	return s.p.Requests.Reject(ctx, a.leaveActor(), id, req)
}

// CalendarEntry is one leave on the team calendar; reasons are never shown (LP-07).
type CalendarEntry struct {
	EmployeeID   uuid.UUID `json:"employee_id"`
	EmployeeName string    `json:"employee_name"`
	LeaveTypeID  uuid.UUID `json:"leave_type_id"`
	StartDate    Date      `json:"start_date"`
	EndDate      Date      `json:"end_date"`
	HalfDay      *string   `json:"half_day,omitempty"`
	TotalDays    calc.Days `json:"total_days"`
	Status       string    `json:"status"`
}

// TeamCalendar is LP-07's response.
type TeamCalendar struct {
	From     Date            `json:"from"`
	To       Date            `json:"to"`
	Leaves   []CalendarEntry `json:"leaves"`
	Holidays []TenantHoliday `json:"holidays"`
}

// Calendar is LP-07: the caller's reports (every worker for leave:read) and the tenant's blocked dates.
func (s *Service) Calendar(ctx context.Context, a Actor, rawFrom, rawTo string) (*TeamCalendar, error) {
	from, err := parseDate("from", rawFrom)
	if err != nil {
		return nil, err
	}
	to, err := parseDate("to", rawTo)
	if err != nil {
		return nil, err
	}
	if to.Before(from) || int(to.Sub(from).Hours()/24) >= maxCalendarDays {
		return nil, ErrRange
	}
	var ids []uuid.UUID
	if !a.CanRead {
		reports, err := s.reportIDs(ctx, a)
		if err != nil {
			return nil, err
		}
		for id := range reports {
			ids = append(ids, id)
		}
	}
	rows, err := s.store.RequestsOverlapping(ctx, s.store.DB(), a.TenantID, ids, a.CanRead, from, to)
	if err != nil {
		return nil, err
	}
	hols, err := s.store.TenantHolidays(ctx, s.store.DB(), a.TenantID, from, to)
	if err != nil {
		return nil, err
	}
	out, names := &TeamCalendar{From: Date{from}, To: Date{to}, Leaves: []CalendarEntry{}, Holidays: append([]TenantHoliday{}, hols...)}, map[uuid.UUID]string{}
	for _, r := range rows {
		n, err := s.name(ctx, a.TenantID, r.EmployeeProfileID, names)
		if err != nil {
			return nil, err
		}
		out.Leaves = append(out.Leaves, CalendarEntry{EmployeeID: r.EmployeeProfileID, EmployeeName: n, LeaveTypeID: r.LeaveTypeID, StartDate: Date{r.StartDate},
			EndDate: Date{r.EndDate}, HalfDay: r.HalfDay, TotalDays: r.TotalDays, Status: r.Status})
	}
	return out, nil
}
