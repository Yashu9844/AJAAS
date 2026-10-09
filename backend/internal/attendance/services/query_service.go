package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/calc"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/repositories"
	"github.com/jaas/jaas/internal/attendance/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// Date-range caps for list endpoints (FR-AR003/FR-AR004).
const (
	selfRangeMaxDays  = 62
	adminRangeMaxDays = 366
)

var recordStatuses = map[string]bool{
	models.StatusPresent: true, models.StatusHalfDay: true, models.StatusAbsent: true,
	models.StatusOnLeave: true, models.StatusHoliday: true, models.StatusWeekOff: true,
}

// RecordQuery holds raw admin filters (A7); validated by the service.
type RecordQuery struct {
	EmployeeID, From, To, Status string
}

// QueryService serves attendance reads (FR-AR003..FR-AR005).
type QueryService interface {
	Today(ctx context.Context, a Actor) (*dto.TodayResponse, error)
	Mine(ctx context.Context, a Actor, from, to string) ([]dto.RecordResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, q RecordQuery, page dto.Page) ([]dto.RecordResponse, dto.PageMeta, error)
	Detail(ctx context.Context, tenantID, id uuid.UUID) (*dto.RecordDetailResponse, error)
	Summary(ctx context.Context, tenantID uuid.UUID, date string) (*dto.SummaryResponse, error)
}

type queryService struct{ base }

// NewQueryService builds a QueryService.
func NewQueryService(d Deps) QueryService { return &queryService{base{d}} }

// Today shows the open session's record if any, else today's (AT-005) record and shift.
func (s *queryService) Today(ctx context.Context, a Actor) (*dto.TodayResponse, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, err
	}
	db, now := s.Tx.DB(), s.Now().UTC()
	last, err := s.Repos.Punches.LastForEmployee(ctx, db, a.TenantID, emp.ID)
	if err != nil {
		return nil, err
	}
	var rec *models.AttendanceRecord
	var shift *models.Shift
	open := last != nil && last.PunchType == models.PunchIn && now.Sub(last.PunchTime) <= calc.SessionAbandonAfter
	date := time.Time{}
	if open {
		if rec, err = s.Repos.Records.FindByID(ctx, db, a.TenantID, last.AttendanceRecordID); err != nil {
			return nil, err
		}
		date = rec.AttendanceDate
		if shift, err = s.shiftByID(ctx, db, a.TenantID, rec.ShiftID); err != nil {
			return nil, err
		}
	} else {
		if date, shift, err = s.resolveDay(ctx, db, a.TenantID, emp.ID, now); err != nil {
			return nil, err
		}
		if rec, err = s.Repos.Records.FindByDate(ctx, db, a.TenantID, emp.ID, date); err != nil {
			return nil, err
		}
	}
	res := &dto.TodayResponse{AttendanceDate: date.Format(validators.DateLayout), OpenSession: open, Shift: mapShift(shift)}
	if rec != nil {
		r := mapRecord(rec)
		res.Record = &r
	}
	return res, nil
}

func (s *queryService) Mine(ctx context.Context, a Actor, from, to string) ([]dto.RecordResponse, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, err
	}
	f, t, err := validators.DateRange(from, to, utcDay(s.Now()), selfRangeMaxDays)
	if err != nil {
		return nil, invalid("from/to", err.Error())
	}
	rows, err := s.Repos.Records.ListForEmployee(ctx, s.Tx.DB(), a.TenantID, emp.ID, f, t)
	if err != nil {
		return nil, err
	}
	out := make([]dto.RecordResponse, len(rows))
	for i := range rows {
		out[i] = mapRecord(&rows[i])
	}
	return out, nil
}

func (s *queryService) List(ctx context.Context, tenantID uuid.UUID, q RecordQuery, page dto.Page) ([]dto.RecordResponse, dto.PageMeta, error) {
	var filter repositories.RecordFilter
	if q.EmployeeID != "" {
		id, err := parseUUIDField("employee_id", q.EmployeeID)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		filter.EmployeeID = &id
	}
	if q.Status != "" && !recordStatuses[q.Status] {
		return nil, dto.PageMeta{}, invalid("status", "unknown attendance status")
	}
	f, t, err := validators.DateRange(q.From, q.To, utcDay(s.Now()), adminRangeMaxDays)
	if err != nil {
		return nil, dto.PageMeta{}, invalid("from/to", err.Error())
	}
	filter.From, filter.To, filter.Status = f, t, q.Status
	rows, total, err := s.Repos.Records.List(ctx, s.Tx.DB(), tenantID, filter, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.RecordResponse, len(rows))
	for i := range rows {
		out[i] = mapRecord(&rows[i])
	}
	return out, page.Meta(total), nil
}

func (s *queryService) Detail(ctx context.Context, tenantID, id uuid.UUID) (*dto.RecordDetailResponse, error) {
	rec, err := s.Repos.Records.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, sharedErrors.ErrNotFound
	}
	ps, err := s.Repos.Punches.AllForRecord(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	out := &dto.RecordDetailResponse{Record: mapRecord(rec), Punches: make([]dto.PunchResponse, len(ps))}
	for i := range ps {
		out.Punches[i] = mapPunch(&ps[i])
	}
	return out, nil
}

// Summary computes AT-020 from one aggregate query plus the Module 2 workforce count.
func (s *queryService) Summary(ctx context.Context, tenantID uuid.UUID, date string) (*dto.SummaryResponse, error) {
	day := utcDay(s.Now())
	if date != "" {
		d, err := validators.ParseDate(date)
		if err != nil {
			return nil, invalid("date", err.Error())
		}
		day = d
	}
	counts, err := s.Repos.Records.CountByStatus(ctx, s.Tx.DB(), tenantID, day)
	if err != nil {
		return nil, err
	}
	total, err := s.Employees.CountWorking(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	res := &dto.SummaryResponse{Date: day.Format(validators.DateLayout), TotalEmployees: total,
		Present: counts.ByStatus[models.StatusPresent], Late: counts.Late, HalfDay: counts.ByStatus[models.StatusHalfDay],
		OnLeave: counts.ByStatus[models.StatusOnLeave]}
	if absent := total - res.Present - res.HalfDay - res.OnLeave; absent > 0 {
		res.Absent = absent
	}
	return res, nil
}
