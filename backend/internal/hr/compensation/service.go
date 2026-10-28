package compensation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	payrollAPI "github.com/jaas/jaas/internal/hr/payroll/api"
	performanceAPI "github.com/jaas/jaas/internal/hr/performance/api"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Limits and thresholds (CO-C10, CO-C23).
const (
	autoApproveRaise = kernel.Money(1000) // 10.00% in hundredths of a percent
	maxBase          = kernel.Money(1_000_000_000_00)
	maxMeritPct      = kernel.Fixed(5000)
	compaLow         = kernel.Fixed(90)
	compaHigh        = kernel.Fixed(110)
)

var changeReasons = map[string]bool{"hire": true, "promotion": true, "market": true, "correction": true}

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Errors (specification.md §3).
var (
	ErrNotFound         = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrGrade            = appErr(http.StatusConflict, "INACTIVE_REFERENCE", "the grade must be an active core grade")
	ErrBackdated        = appErr(http.StatusConflict, "BACKDATED", "the date must be after the start of the current record or range")
	ErrNoChange         = appErr(http.StatusConflict, "NO_CHANGE", "the new base equals the current base")
	ErrPending          = appErr(http.StatusConflict, "CONFLICT", "the worker already has a pending change")
	ErrChangeState      = appErr(http.StatusConflict, "CHANGE_STATE", "the change is not pending")
	ErrCycleState       = appErr(http.StatusConflict, "CYCLE_STATE", "the merit cycle's status does not allow this")
	ErrSelfApproval     = appErr(http.StatusForbidden, "COMP_SELF_APPROVAL", "you cannot approve your own change or cycle")
	ErrBudget           = appErr(http.StatusConflict, "BUDGET_EXCEEDED", "the department's merit budget would be exceeded")
)

// Directory is compensation's view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error)
}

// Ports are the collaborators compensation consumes (design.md).
type Ports struct {
	Employees Directory
	Core      coreAPI.Directory
	Ratings   performanceAPI.Ratings
	Payroll   payrollAPI.CTC
	Audit     identityport.Auditor
}

// Actor is the caller; CanRead / CanManage are compensation:read / compensation:manage.
type Actor struct {
	TenantID, UserID   uuid.UUID
	IP, UserAgent      string
	CanRead, CanManage bool
}

// Service implements compensation (CO-C01..CO-C26).
type Service struct {
	store Store
	p     Ports
	now   func() time.Time
}

// NewService builds the service.
func NewService(store Store, p Ports, now func() time.Time) *Service {
	return &Service{store: store, p: p, now: now}
}

func (s *Service) today() time.Time { return day(s.now()) }

func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func parseDate(field, raw string) (Date, error) {
	t, err := time.Parse(DateLayout, strings.TrimSpace(raw))
	if err != nil {
		return Date{}, invalid(field, "must be YYYY-MM-DD")
	}
	return Date{t}, nil
}

func optional(field string, v *string, max int) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if utf8.RuneCountInString(t) > max {
		return nil, invalid(field, "is too long")
	}
	return &t, nil
}

func (s *Service) log(ctx context.Context, a Actor, action, resource, id string, meta map[string]string) {
	_ = s.p.Audit.Log(ctx, s.store.DB(), a.TenantID.String(), a.UserID.String(), action, resource, id, meta, a.IP, a.UserAgent)
}

func (s *Service) worker(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	e, err := s.p.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && e == nil) {
		return nil, ErrEmployeeNotFound
	}
	return e, err
}

// --- pay ranges (CO-C01, CO-C02) ---

// RangeInput is a pay-range body.
type RangeInput struct {
	GradeID       string       `json:"grade_id"`
	MinAmount     kernel.Money `json:"min_amount"`
	MidAmount     kernel.Money `json:"mid_amount"`
	MaxAmount     kernel.Money `json:"max_amount"`
	EffectiveFrom string       `json:"effective_from"`
}

// CreateRange is CO-C01: the previous open range of the grade closes at the new start.
func (s *Service) CreateRange(ctx context.Context, a Actor, in RangeInput) (*PayRange, error) {
	grade, err := uuid.Parse(in.GradeID)
	if err != nil {
		return nil, invalid("grade_id", "must be a UUID")
	}
	from, err := parseDate("effective_from", in.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	if in.MinAmount <= 0 || in.MinAmount > in.MidAmount || in.MidAmount > in.MaxAmount || in.MaxAmount > maxBase {
		return nil, invalid("amounts", "must satisfy 0 < min ≤ mid ≤ max")
	}
	exists, active, err := s.p.Core.GradeState(ctx, a.TenantID, grade)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if !active {
		return nil, ErrGrade
	}
	r := &PayRange{TenantID: a.TenantID, GradeID: grade, MinAmount: in.MinAmount, MidAmount: in.MidAmount, MaxAmount: in.MaxAmount,
		EffectiveFrom: from, CreatedByUserID: a.UserID, CreatedAt: s.now().UTC()}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Lock(ctx, tx, "range:"+grade.String()); err != nil {
			return err
		}
		ranges, err := s.store.Ranges(ctx, tx, a.TenantID, &grade)
		if err != nil {
			return err
		}
		for i := range ranges {
			if ranges[i].EffectiveTo == nil {
				if !from.After(ranges[i].EffectiveFrom.Time) {
					return ErrBackdated
				}
				ranges[i].EffectiveTo = &from
				if err := s.store.Save(ctx, tx, &ranges[i]); err != nil {
					return err
				}
			}
		}
		return s.store.Create(ctx, tx, r)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "compensation.range_created", "comp_pay_range", r.ID.String(), map[string]string{"grade_id": grade.String()})
	return r, nil
}

// Ranges lists pay ranges, optionally for one grade.
func (s *Service) Ranges(ctx context.Context, a Actor, grade *uuid.UUID) ([]PayRange, error) {
	return s.store.Ranges(ctx, s.store.DB(), a.TenantID, grade)
}

// Position is CO-C02: compa-ratio and penetration of a base for the worker's grade on a date.
type Position struct {
	GradeID     *uuid.UUID    `json:"grade_id,omitempty"`
	RangeID     *uuid.UUID    `json:"range_id,omitempty"`
	CompaRatio  *kernel.Fixed `json:"compa_ratio,omitempty"`
	Penetration *kernel.Fixed `json:"range_penetration,omitempty"`
	CompaBand   string        `json:"compa_band"`
}

// position computes CO-C02 exactly (hundredths, half away from zero).
func (s *Service) position(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, base kernel.Money, on time.Time) (Position, error) {
	out := Position{CompaBand: "unknown"}
	pl, err := s.p.Core.PlacementOn(ctx, tenantID, employeeID, on)
	if err != nil || pl == nil || pl.GradeID == nil {
		return out, err
	}
	out.GradeID = pl.GradeID
	ranges, err := s.store.Ranges(ctx, db, tenantID, pl.GradeID)
	if err != nil {
		return out, err
	}
	for i := range ranges {
		r := ranges[i]
		if !r.EffectiveFrom.After(on) && (r.EffectiveTo == nil || r.EffectiveTo.After(on)) {
			compa := kernel.Fixed(kernel.DivRound(int64(base)*100, int64(r.MidAmount)))
			out.RangeID, out.CompaRatio = &r.ID, &compa
			if r.MaxAmount > r.MinAmount {
				pen := kernel.Fixed(kernel.DivRound(int64(base-r.MinAmount)*10000, int64(r.MaxAmount-r.MinAmount)))
				out.Penetration = &pen
			}
			out.CompaBand = compaBand(compa)
		}
	}
	return out, nil
}

func compaBand(c kernel.Fixed) string {
	switch {
	case c < compaLow:
		return "below"
	case c > compaHigh:
		return "above"
	}
	return "within"
}

// --- records (CO-C11) ---

// applyRecord closes the open record at the new start and inserts the new one, then syncs payroll (CO-C11).
func (s *Service) applyRecord(ctx context.Context, tx *gorm.DB, a Actor, r *Record) error {
	if err := s.store.Lock(ctx, tx, "worker:"+r.EmployeeProfileID.String()); err != nil {
		return err
	}
	rows, err := s.store.Records(ctx, tx, a.TenantID, r.EmployeeProfileID)
	if err != nil {
		return err
	}
	for i := range rows {
		if rows[i].EffectiveTo == nil {
			if !r.EffectiveFrom.After(rows[i].EffectiveFrom.Time) {
				return ErrBackdated
			}
			from := r.EffectiveFrom
			rows[i].EffectiveTo = &from
			if err := s.store.Save(ctx, tx, &rows[i]); err != nil {
				return err
			}
		}
	}
	synced, err := s.p.Payroll.SetCTC(ctx, tx, a.TenantID, r.EmployeeProfileID, r.AnnualBase, r.EffectiveFrom.Time, a.UserID)
	if err != nil {
		return err
	}
	r.PayrollSynced, r.CreatedByUserID, r.CreatedAt = synced, a.UserID, s.now().UTC()
	if err := s.store.Create(ctx, tx, r); err != nil {
		return err
	}
	return s.store.Event(ctx, tx, a.TenantID, "hr.compensation.changed", map[string]string{"record_id": r.ID.String(),
		"employee_id": r.EmployeeProfileID.String(), "effective_from": r.EffectiveFrom.Format(DateLayout), "reason": r.Reason})
}

// current returns the record in effect on d, or nil.
func current(rows []Record, d time.Time) *Record {
	for i := range rows {
		if rows[i].Covers(d) {
			return &rows[i]
		}
	}
	return nil
}

// --- off-cycle changes (CO-C10, CO-C13) ---

// ChangeInput is an off-cycle change body.
type ChangeInput struct {
	EmployeeID    string       `json:"employee_id"`
	AnnualBase    kernel.Money `json:"annual_base"`
	EffectiveFrom string       `json:"effective_from"`
	Reason        string       `json:"reason"`
	Comment       *string      `json:"comment"`
}

// CreateChange is CO-C10: first records and raises up to 10% apply at once; everything else waits for approval.
func (s *Service) CreateChange(ctx context.Context, a Actor, in ChangeInput) (*Change, error) {
	emp, err := uuid.Parse(in.EmployeeID)
	if err != nil {
		return nil, invalid("employee_id", "must be a UUID")
	}
	if in.AnnualBase <= 0 || in.AnnualBase > maxBase {
		return nil, invalid("annual_base", "must be positive")
	}
	from, err := parseDate("effective_from", in.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	if !changeReasons[in.Reason] {
		return nil, invalid("reason", "must be hire, promotion, market or correction")
	}
	comment, err := optional("comment", in.Comment, 1000)
	if err != nil {
		return nil, err
	}
	if _, err := s.worker(ctx, a.TenantID, emp); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	c := &Change{TenantID: a.TenantID, EmployeeProfileID: emp, AnnualBase: in.AnnualBase, EffectiveFrom: from, Reason: in.Reason, Comment: comment,
		Status: ChangePending, RequestedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Lock(ctx, tx, "worker:"+emp.String()); err != nil {
			return err
		}
		pending, _, err := s.store.Changes(ctx, tx, a.TenantID, ChangeFilter{EmployeeID: &emp, Status: ChangePending}, 1, 1)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			return ErrPending
		}
		rows, err := s.store.Records(ctx, tx, a.TenantID, emp)
		if err != nil {
			return err
		}
		var open *Record
		for i := range rows {
			if rows[i].EffectiveTo == nil {
				open = &rows[i]
			}
		}
		if open != nil && open.AnnualBase == in.AnnualBase {
			return ErrNoChange
		}
		auto := open == nil || (in.AnnualBase > open.AnnualBase && in.AnnualBase <= open.AnnualBase+open.AnnualBase.Percent(autoApproveRaise))
		if auto {
			c.Status, c.DecidedAt = ChangeApproved, &now
		}
		if err := s.store.Create(ctx, tx, c); err != nil {
			return err
		}
		if auto {
			return s.applyRecord(ctx, tx, a, &Record{TenantID: a.TenantID, EmployeeProfileID: emp, AnnualBase: in.AnnualBase, EffectiveFrom: from,
				Reason: in.Reason, ChangeID: &c.ID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "compensation.change_"+c.Status, "comp_change", c.ID.String(), map[string]string{"employee_id": emp.String(), "reason": c.Reason})
	return c, nil
}

// Changes lists off-cycle changes.
func (s *Service) Changes(ctx context.Context, a Actor, f ChangeFilter, page, perPage int) ([]Change, int64, error) {
	if f.Status != "" && f.Status != ChangePending && f.Status != ChangeApproved && f.Status != ChangeRejected && f.Status != ChangeCancelled {
		return nil, 0, invalid("status", "must be pending, approved, rejected or cancelled")
	}
	return s.store.Changes(ctx, s.store.DB(), a.TenantID, f, page, perPage)
}

// GetChange returns one change.
func (s *Service) GetChange(ctx context.Context, a Actor, id uuid.UUID) (*Change, error) {
	c, err := s.store.Change(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrNotFound
	}
	return c, nil
}

// DecideChange approves (and applies), rejects or cancels a pending change (CO-C13). Approvers are never the requester;
// only the requester or compensation:manage cancels.
func (s *Service) DecideChange(ctx context.Context, a Actor, id uuid.UUID, to string, comment *string) (*Change, error) {
	cm, err := optional("comment", comment, 1000)
	if err != nil {
		return nil, err
	}
	var c *Change
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if c, err = s.store.Change(ctx, tx, a.TenantID, id, true); err != nil {
			return err
		}
		if c == nil {
			return ErrNotFound
		}
		if c.Status != ChangePending {
			return ErrChangeState
		}
		switch {
		case to == ChangeCancelled && c.RequestedByUserID != a.UserID && !a.CanManage:
			return ErrNotFound
		case to != ChangeCancelled && c.RequestedByUserID == a.UserID:
			return ErrSelfApproval
		}
		now := s.now().UTC()
		c.Status, c.UpdatedAt = to, now
		if to != ChangeCancelled {
			c.DecidedByUserID, c.DecisionComment, c.DecidedAt = &a.UserID, cm, &now
		}
		if err := s.store.Save(ctx, tx, c); err != nil {
			return err
		}
		if to == ChangeApproved {
			return s.applyRecord(ctx, tx, a, &Record{TenantID: a.TenantID, EmployeeProfileID: c.EmployeeProfileID, AnnualBase: c.AnnualBase,
				EffectiveFrom: c.EffectiveFrom, Reason: c.Reason, ChangeID: &c.ID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "compensation.change_"+to, "comp_change", c.ID.String(), map[string]string{"employee_id": c.EmployeeProfileID.String()})
	return c, nil
}

// --- worker views (CO-C12) ---

// WorkerView is CO-C12.
type WorkerView struct {
	EmployeeID uuid.UUID `json:"employee_id"`
	Current    *Record   `json:"current"`
	Position   Position  `json:"position"`
	History    []Record  `json:"history"`
	Pending    []Change  `json:"pending_changes"`
}

// Worker is CO-C12 for compensation:read.
func (s *Service) Worker(ctx context.Context, a Actor, employeeID uuid.UUID) (*WorkerView, error) {
	if _, err := s.worker(ctx, a.TenantID, employeeID); err != nil {
		return nil, err
	}
	rows, err := s.store.Records(ctx, s.store.DB(), a.TenantID, employeeID)
	if err != nil {
		return nil, err
	}
	pending, _, err := s.store.Changes(ctx, s.store.DB(), a.TenantID, ChangeFilter{EmployeeID: &employeeID, Status: ChangePending}, 1, 100)
	if err != nil {
		return nil, err
	}
	v := &WorkerView{EmployeeID: employeeID, History: append([]Record{}, rows...), Pending: append([]Change{}, pending...), Position: Position{CompaBand: "unknown"}}
	if v.Current = current(rows, s.today()); v.Current != nil {
		if v.Position, err = s.position(ctx, s.store.DB(), a.TenantID, employeeID, v.Current.AnnualBase, s.today()); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// Me is CO-C12 for the caller.
func (s *Service) Me(ctx context.Context, a Actor) (*WorkerView, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, ErrEmployeeNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Worker(ctx, a, me.ID)
}
