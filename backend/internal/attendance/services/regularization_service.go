package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/calc"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/events"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/repositories"
	"github.com/jaas/jaas/internal/attendance/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Regularization limits (AT-014).
const (
	regularizationWindowDays = 30
	maxRegularizedSpan       = 20 * time.Hour
)

var regStatuses = map[string]bool{models.RegPending: true, models.RegApproved: true, models.RegRejected: true, models.RegCancelled: true}

// RegularizationService handles timesheet corrections (FR-RG001..FR-RG003, AT-014..AT-018).
type RegularizationService interface {
	Create(ctx context.Context, a Actor, req dto.CreateRegularizationRequest) (*dto.RegularizationResponse, error)
	ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)
	List(ctx context.Context, tenantID uuid.UUID, status, employeeID string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)
	Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.RegularizationResponse, error)
	Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)
	Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)
}

type regularizationService struct{ base }

// NewRegularizationService builds a RegularizationService.
func NewRegularizationService(d Deps) RegularizationService { return &regularizationService{base{d}} }

func (s *regularizationService) Create(ctx context.Context, a Actor, req dto.CreateRegularizationRequest) (*dto.RegularizationResponse, error) {
	emp, err := s.employeeFor(ctx, a, true)
	if err != nil {
		return nil, err
	}
	date, err := validators.ParseDate(req.AttendanceDate)
	if err != nil {
		return nil, invalid("attendance_date", err.Error())
	}
	shift, err := s.shiftOn(ctx, s.Tx.DB(), a.TenantID, emp.ID, date)
	if err != nil {
		return nil, err
	}
	if err := s.checkWindow(date, req, specFor(shift)); err != nil {
		return nil, err
	}
	g := &models.Regularization{TenantID: a.TenantID, EmployeeProfileID: emp.ID, AttendanceDate: date,
		RequestedPunchIn: req.RequestedPunchIn.UTC(), RequestedPunchOut: req.RequestedPunchOut.UTC(),
		Reason: strings.TrimSpace(req.Reason), Status: models.RegPending, RequestedByUserID: a.UserID}
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		pending, err := s.Repos.Regularizations.FindPending(ctx, tx, a.TenantID, emp.ID, date)
		if err != nil {
			return err
		}
		if pending != nil {
			return ErrRegularizationPend
		}
		if rec, err := s.Repos.Records.FindByDate(ctx, tx, a.TenantID, emp.ID, date); err != nil {
			return err
		} else if rec != nil {
			g.AttendanceRecordID = &rec.ID
		}
		if err := s.Repos.Regularizations.Create(ctx, tx, g); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.RegularizationRequested, regPayload(g))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, "attendance.regularization_requested", "attendance_regularization", g.ID.String(),
		map[string]string{"attendance_date": req.AttendanceDate}, row)
	res := mapRegularization(g)
	return &res, nil
}

// checkWindow enforces AT-014 in the shift's time zone.
func (s *regularizationService) checkWindow(date time.Time, req dto.CreateRegularizationRequest, spec *calc.ShiftSpec) error {
	loc := time.UTC
	if spec != nil {
		loc = spec.Location
	}
	local := s.Now().In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if date.After(today) || date.Before(today.AddDate(0, 0, -regularizationWindowDays)) {
		return ErrRegularizationWindow
	}
	if !req.RequestedPunchOut.After(req.RequestedPunchIn) {
		return invalid("requested_punch_out", "must be after requested_punch_in")
	}
	if req.RequestedPunchOut.Sub(req.RequestedPunchIn) > maxRegularizedSpan {
		return invalid("requested_punch_out", "span must not exceed 20 hours")
	}
	if !calc.AttendanceDate(req.RequestedPunchIn, spec).Equal(date) {
		return invalid("requested_punch_in", "must fall on attendance_date in the shift time zone")
	}
	return nil
}

func (s *regularizationService) list(ctx context.Context, tenantID uuid.UUID, f repositories.RegularizationFilter, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error) {
	if f.Status != "" && !regStatuses[f.Status] {
		return nil, dto.PageMeta{}, invalid("status", "must be pending, approved, rejected or cancelled")
	}
	rows, total, err := s.Repos.Regularizations.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.RegularizationResponse, len(rows))
	for i := range rows {
		out[i] = mapRegularization(&rows[i])
	}
	return out, page.Meta(total), nil
}

func (s *regularizationService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return s.list(ctx, a.TenantID, repositories.RegularizationFilter{EmployeeID: &emp.ID, Status: status}, page)
}

func (s *regularizationService) List(ctx context.Context, tenantID uuid.UUID, status, employeeID string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error) {
	f := repositories.RegularizationFilter{Status: status}
	if employeeID != "" {
		id, err := parseUUIDField("employee_id", employeeID)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		f.EmployeeID = &id
	}
	return s.list(ctx, tenantID, f, page)
}

func (s *regularizationService) loadPending(ctx context.Context, tenantID, id uuid.UUID) (*models.Regularization, error) {
	g, err := s.Repos.Regularizations.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if g.Status != models.RegPending {
		return nil, ErrRegularizationState
	}
	return g, nil
}

// Cancel lets the requester withdraw a pending request; others get 404 (AT-018).
func (s *regularizationService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.RegularizationResponse, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, err
	}
	g, err := s.Repos.Regularizations.FindByID(ctx, s.Tx.DB(), a.TenantID, id)
	if err != nil {
		return nil, err
	}
	if g == nil || g.EmployeeProfileID != emp.ID {
		return nil, sharedErrors.ErrNotFound
	}
	if g.Status != models.RegPending {
		return nil, ErrRegularizationState
	}
	g.Status = models.RegCancelled
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.Repos.Regularizations.Update(ctx, tx, g) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, "attendance.regularization_cancelled", "attendance_regularization", g.ID.String(), nil, nil)
	res := mapRegularization(g)
	return &res, nil
}

// guardSelfReview blocks reviewers acting on their own request (AT-016).
func (s *regularizationService) guardSelfReview(ctx context.Context, a Actor, g *models.Regularization) error {
	if g.RequestedByUserID == a.UserID {
		return ErrSelfApproval
	}
	emp, err := s.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if err != nil && !errors.Is(err, sharedErrors.ErrNotFound) {
		return err
	}
	if emp != nil && emp.ID == g.EmployeeProfileID {
		return ErrSelfApproval
	}
	return nil
}
