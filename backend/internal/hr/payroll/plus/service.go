package plus

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/kernel"
	leaveAPI "github.com/jaas/jaas/internal/hr/leave/api"
	onboardingAPI "github.com/jaas/jaas/internal/hr/onboarding/api"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"github.com/jaas/jaas/internal/hr/payroll/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Errors (specification.md, Parity).
var (
	ErrNotFound         = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrNotLeaver        = appErr(http.StatusConflict, "NOT_A_LEAVER", "the worker has no exit date and is not terminated")
	ErrNoAssignment     = appErr(http.StatusConflict, "NO_ASSIGNMENT", "the worker has no payroll assignment")
	ErrClearance        = appErr(http.StatusConflict, "CLEARANCE_PENDING", "the exit clearance is not complete")
	ErrState            = appErr(http.StatusConflict, "SETTLEMENT_STATE", "the settlement's status does not allow this")
	ErrSelfApproval     = appErr(http.StatusForbidden, "PAY_SELF_APPROVAL", "the calculator cannot approve a settlement")
	ErrConflict         = appErr(http.StatusConflict, "CONFLICT", "the worker already has an open settlement")
)

var stateCode = regexp.MustCompile(`^[A-Za-z]{2}$`)

// Directory is plus' view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// States resolves a worker work state (two-letter code) on a date; "" when unknown.
type States interface {
	StateOf(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (string, error)
}

// LocationStates reads the state code of an organization location.
type LocationStates interface {
	LocationStateCode(ctx context.Context, tenantID, id uuid.UUID) (string, error)
}

// StatesFromCore resolves the state through the worker core assignment and its organization location (PP-02).
func StatesFromCore(core coreAPI.Directory, locs LocationStates) States {
	return coreStates{core, locs}
}

type coreStates struct {
	core coreAPI.Directory
	locs LocationStates
}

func (c coreStates) StateOf(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (string, error) {
	p, err := c.core.PlacementOn(ctx, tenantID, employeeID, on)
	if err != nil || p == nil || p.LocationID == nil {
		return "", err
	}
	code, err := c.locs.LocationStateCode(ctx, tenantID, *p.LocationID)
	if err != nil || !stateCode.MatchString(code) {
		return "", err
	}
	return strings.ToUpper(code), nil
}

// PT implements payroll services.PTSource: the worker state and the tenant or built-in slabs (PP-02).
type PT struct {
	store  Store
	mu     sync.RWMutex
	states States
}

// NewPT builds the source; states arrive later through SetStates (core is built after payroll).
func NewPT(store Store) *PT { return &PT{store: store} }

// SetStates installs the state lookup.
func (p *PT) SetStates(s States) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states = s
}

// StateOf implements services.PTSource.
func (p *PT) StateOf(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (string, error) {
	p.mu.RLock()
	s := p.states
	p.mu.RUnlock()
	if s == nil {
		return "", nil
	}
	return s.StateOf(ctx, tenantID, employeeID, on)
}

// Slabs implements services.PTSource: tenant slabs of the state, else the built-in defaults, else nil.
func (p *PT) Slabs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, state string) ([]calc.PTSlab, error) {
	rows, err := p.store.PTSlabs(ctx, db, tenantID, state)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return calc.DefaultPTSlabs(state), nil
	}
	out := make([]calc.PTSlab, len(rows))
	for i, r := range rows {
		out[i] = r.Calc()
	}
	return out, nil
}

// Ports are the collaborators plus consumes.
type Ports struct {
	Employees   Directory
	Structures  repositories.StructureRepository
	Assignments repositories.AssignmentRepository
	Leave       leaveAPI.Settlement
	Clearance   onboardingAPI.Clearance
	Audit       identityport.Auditor
}

// Actor is the caller.
type Actor struct {
	TenantID, UserID uuid.UUID
	IP, UserAgent    string
}

// Service implements PP-02 and PP-06..PP-09.
type Service struct {
	store Store
	p     Ports
	pt    *PT
	now   func() time.Time
}

// NewService builds the service.
func NewService(store Store, p Ports, pt *PT, now func() time.Time) *Service {
	return &Service{store: store, p: p, pt: pt, now: now}
}

func (s *Service) today() time.Time {
	n := s.now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

func (s *Service) log(ctx context.Context, a Actor, action, id string) {
	_ = s.p.Audit.Log(ctx, s.store.DB(), a.TenantID.String(), a.UserID.String(), action, "payroll_settlement", id, nil, a.IP, a.UserAgent)
}

// --- PT slabs (PP-09) ---

// SlabInput is one slab of a state.
type SlabInput struct {
	MinGross kernel.Money  `json:"min_gross"`
	MaxGross *kernel.Money `json:"max_gross"`
	Amount   kernel.Money  `json:"amount"`
	Month    *int          `json:"month"`
}

// SlabView is a slab as the API shows it.
type SlabView struct {
	MinGross kernel.Money  `json:"min_gross"`
	MaxGross *kernel.Money `json:"max_gross,omitempty"`
	Amount   kernel.Money  `json:"amount"`
	Month    *int          `json:"month,omitempty"`
}

// SlabsResponse lists tenant slabs and the built-in defaults per state.
type SlabsResponse struct {
	Tenant   map[string][]SlabView `json:"tenant"`
	Defaults map[string][]SlabView `json:"defaults"`
}

func viewOf(c calc.PTSlab) SlabView {
	v := SlabView{MinGross: c.MinGross, Amount: c.Amount}
	if c.MaxGross > 0 {
		m := c.MaxGross
		v.MaxGross = &m
	}
	if c.Month > 0 {
		m := c.Month
		v.Month = &m
	}
	return v
}

// PTSlabs is PP-09: the tenant slabs and the defaults.
func (s *Service) PTSlabs(ctx context.Context, a Actor) (*SlabsResponse, error) {
	rows, err := s.store.AllPTSlabs(ctx, s.store.DB(), a.TenantID)
	if err != nil {
		return nil, err
	}
	out := &SlabsResponse{Tenant: map[string][]SlabView{}, Defaults: map[string][]SlabView{}}
	for _, r := range rows {
		out.Tenant[r.StateCode] = append(out.Tenant[r.StateCode], viewOf(r.Calc()))
	}
	for _, st := range []string{"AP", "GJ", "KA", "MH", "TS", "WB"} {
		for _, c := range calc.DefaultPTSlabs(st) {
			out.Defaults[st] = append(out.Defaults[st], viewOf(c))
		}
	}
	return out, nil
}

// SetPTSlabs replaces a state slabs (PP-09).
func (s *Service) SetPTSlabs(ctx context.Context, a Actor, state string, in []SlabInput) ([]SlabView, error) {
	if !stateCode.MatchString(state) {
		return nil, invalid("state", "must be a two-letter state code")
	}
	state = strings.ToUpper(state)
	slabs, rows := make([]calc.PTSlab, 0, len(in)), make([]models.PTSlab, 0, len(in))
	now := s.now().UTC()
	for _, x := range in {
		c := calc.PTSlab{MinGross: x.MinGross, Amount: x.Amount}
		if x.MaxGross != nil {
			c.MaxGross = *x.MaxGross
		}
		if x.Month != nil {
			c.Month = *x.Month
		}
		slabs = append(slabs, c)
		rows = append(rows, models.PTSlab{TenantID: a.TenantID, StateCode: state, MinGross: x.MinGross, MaxGross: x.MaxGross, Amount: x.Amount, Month: x.Month,
			CreatedByUserID: a.UserID, CreatedAt: now})
	}
	if err := calc.ValidatePTSlabs(slabs); err != nil {
		return nil, appErr(http.StatusBadRequest, "INVALID_SLABS", err.Error())
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.ReplacePTSlabs(ctx, tx, a.TenantID, state, rows) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "payroll.pt_slabs_saved", state)
	out := make([]SlabView, len(slabs))
	for i, c := range slabs {
		out[i] = viewOf(c)
	}
	return out, nil
}

// DeletePTSlabs removes a state tenant slabs, back to the defaults (PP-09).
func (s *Service) DeletePTSlabs(ctx context.Context, a Actor, state string) error {
	if !stateCode.MatchString(state) {
		return invalid("state", "must be a two-letter state code")
	}
	var n int64
	err := s.store.InTx(ctx, func(tx *gorm.DB) (err error) {
		n, err = s.store.DeletePTSlabs(ctx, tx, a.TenantID, strings.ToUpper(state))
		return err
	})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	if err == nil {
		s.log(ctx, a, "payroll.pt_slabs_deleted", strings.ToUpper(state))
	}
	return err
}

// --- settlements (PP-06..PP-08) ---

type computed struct {
	set    *Settlement
	lines  []SettlementLine
	encash UUIDList
}

func (s *Service) employee(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	e, err := s.p.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && e == nil) {
		return nil, ErrEmployeeNotFound
	}
	return e, err
}

// monthly returns the BASIC and gross of the structure at the CTC (full month).
func (s *Service) monthly(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, as *models.Assignment) (basic, gross kernel.Money, err error) {
	st, err := s.p.Structures.FindByID(ctx, db, tenantID, as.StructureID)
	if err != nil || st == nil {
		return 0, 0, orNotFound(err)
	}
	comps, err := s.p.Structures.Components(ctx, db, tenantID, as.StructureID)
	if err != nil {
		return 0, 0, err
	}
	spec := calc.StructureSpec{PF: st.PFEnabled, ESI: st.ESIEnabled, PT: st.PTEnabled, TDS: st.TDSEnabled}
	for _, c := range comps {
		spec.Components = append(spec.Components, calc.ComponentSpec{Code: c.Code, Name: c.Name, Kind: c.Kind, Calc: c.Calc, Value: c.Value, Taxable: c.Taxable})
	}
	lines, err := calc.Breakdown(spec, as.AnnualCTC.Monthly())
	if err != nil {
		return 0, 0, appErr(http.StatusConflict, "STRUCTURE_OVERFLOW", err.Error())
	}
	for _, l := range lines {
		if l.Kind == calc.Earning {
			gross += l.Amount
		}
		if l.Code == calc.BasicCode {
			basic = l.Amount
		}
	}
	return basic, gross, nil
}

// compute builds a settlement for a leaver from the current data (PP-06).
func (s *Service) compute(ctx context.Context, db *gorm.DB, a Actor, e *employeeDTO.EmployeeResponse) (*computed, error) {
	emp := e.Employment
	if emp == nil || emp.JoiningDate.IsZero() || (emp.ExitDate == nil && e.Status != "terminated") {
		return nil, ErrNotLeaver
	}
	last := s.today()
	if emp.ExitDate != nil {
		last = dateOnly(*emp.ExitDate)
	}
	join := dateOnly(emp.JoiningDate)
	assigns, err := s.p.Assignments.ForEmployee(ctx, db, a.TenantID, e.ID, repositories.Period{From: join, To: last})
	if err != nil {
		return nil, err
	}
	if len(assigns) == 0 {
		return nil, ErrNoAssignment
	}
	basic, gross, err := s.monthly(ctx, db, a.TenantID, &assigns[len(assigns)-1])
	if err != nil {
		return nil, err
	}
	balances, err := s.p.Leave.EncashableBalances(ctx, db, a.TenantID, e.ID)
	if err != nil {
		return nil, err
	}
	unpaid, err := s.p.Leave.UnpaidEncashments(ctx, db, a.TenantID, e.ID)
	if err != nil {
		return nil, err
	}
	in := SettleInput{Joining: join, LastDay: last, MonthlyBasic: basic, MonthlyGross: gross, NoticeDays: emp.NoticePeriodDays}
	if emp.ResignationDate != nil {
		r := dateOnly(*emp.ResignationDate)
		in.Resignation = &r
	}
	for _, b := range balances {
		in.LeaveDays += b.Days
	}
	var ids UUIDList
	for _, u := range unpaid {
		in.EncashedDays += u.Days
		ids = append(ids, u.ID)
	}
	r := Settle(in)
	now := s.now().UTC()
	set := &Settlement{TenantID: a.TenantID, EmployeeProfileID: e.ID, Status: StatusDraft, LastWorkingDay: Date{last}, ServiceYears: r.CompletedYears,
		MonthlyBasic: basic, Gratuity: r.Gratuity, LeaveEncashment: r.LeaveEncash, Encashments: r.Encashments, NoticeRecovery: r.NoticeRecovery,
		RecoveryShortfall: r.Shortfall, Net: r.Net, EncashmentIDs: ids, CalculatedByUserID: a.UserID, CalculatedAt: now, CreatedAt: now, UpdatedAt: now}
	var lines []SettlementLine
	add := func(code, name, kind string, amt kernel.Money) {
		if amt > 0 {
			lines = append(lines, SettlementLine{Code: code, Name: name, Kind: kind, Amount: amt, Position: len(lines)})
		}
	}
	add("GRATUITY", "Gratuity", "earning", r.Gratuity)
	add("LEAVE_ENCASHMENT", "Leave encashment", "earning", r.LeaveEncash)
	add("ENCASHMENT_APPROVED", "Approved encashments", "earning", r.Encashments)
	add("NOTICE_RECOVERY", "Notice period recovery", "deduction", r.NoticeRecovery)
	return &computed{set: set, lines: lines, encash: ids}, nil
}

func (s *Service) withLines(ctx context.Context, set *Settlement) (*Settlement, error) {
	lines, err := s.store.Lines(ctx, s.store.DB(), set.ID)
	set.Lines = append([]SettlementLine{}, lines...)
	return set, err
}

// Calculate is PP-06: a new draft, or a refreshed draft when one is open.
func (s *Service) Calculate(ctx context.Context, a Actor, rawEmployee string) (*Settlement, error) {
	id, err := uuid.Parse(rawEmployee)
	if err != nil {
		return nil, invalid("employee_id", "must be a UUID")
	}
	e, err := s.employee(ctx, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	var out *Settlement
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		open, err := s.store.OpenSettlement(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if open != nil && open.Status != StatusDraft {
			return ErrConflict
		}
		c, err := s.compute(ctx, tx, a, e)
		if err != nil {
			return err
		}
		if open != nil {
			c.set.ID, c.set.CreatedAt = open.ID, open.CreatedAt
			if err := s.store.Save(ctx, tx, c.set); err != nil {
				return err
			}
		} else if err := s.store.Create(ctx, tx, c.set); err != nil {
			return err
		}
		for i := range c.lines {
			c.lines[i].SettlementID = c.set.ID
		}
		out = c.set
		return s.store.ReplaceLines(ctx, tx, c.set.ID, c.lines)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "payroll.settlement_calculated", out.ID.String())
	return s.withLines(ctx, out)
}

// Recalculate refreshes a draft (PP-07).
func (s *Service) Recalculate(ctx context.Context, a Actor, id uuid.UUID) (*Settlement, error) {
	set, err := s.store.Settlement(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || set == nil {
		return nil, orNotFound(err)
	}
	if set.Status != StatusDraft {
		return nil, ErrState
	}
	return s.Calculate(ctx, a, set.EmployeeProfileID.String())
}

func (s *Service) transition(ctx context.Context, a Actor, id uuid.UUID, verb string, fn func(tx *gorm.DB, set *Settlement) error) (*Settlement, error) {
	var set *Settlement
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if set, err = s.store.Settlement(ctx, tx, a.TenantID, id, true); err != nil || set == nil {
			return orNotFound(err)
		}
		if err := fn(tx, set); err != nil {
			return err
		}
		set.UpdatedAt = s.now().UTC()
		return s.store.Save(ctx, tx, set)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "payroll.settlement_"+verb, id.String())
	return s.withLines(ctx, set)
}

// Approve is PP-07: a user other than the calculator, after the exit clearance.
func (s *Service) Approve(ctx context.Context, a Actor, id uuid.UUID) (*Settlement, error) {
	return s.transition(ctx, a, id, "approved", func(tx *gorm.DB, set *Settlement) error {
		if set.Status != StatusDraft {
			return ErrState
		}
		if set.CalculatedByUserID == a.UserID {
			return ErrSelfApproval
		}
		done, err := s.p.Clearance.ClearanceCompleted(ctx, tx, a.TenantID, set.EmployeeProfileID)
		if err != nil {
			return err
		}
		if !done {
			return ErrClearance
		}
		now := s.now().UTC()
		set.Status, set.ApprovedByUserID, set.ApprovedAt = StatusApproved, &a.UserID, &now
		return s.store.Event(ctx, tx, a.TenantID, "payroll.settlement.approved", map[string]string{"settlement_id": set.ID.String(), "employee_id": set.EmployeeProfileID.String()})
	})
}

// Pay is PP-07: stamp paid and mark the worker approved encashments paid in HR/Leave in the same transaction.
func (s *Service) Pay(ctx context.Context, a Actor, id uuid.UUID) (*Settlement, error) {
	return s.transition(ctx, a, id, "paid", func(tx *gorm.DB, set *Settlement) error {
		if set.Status != StatusApproved {
			return ErrState
		}
		now := s.now().UTC()
		if err := s.p.Leave.MarkEncashmentsPaid(ctx, tx, a.TenantID, set.EncashmentIDs, now); err != nil {
			return err
		}
		set.Status, set.PaidAt = StatusPaid, &now
		return s.store.Event(ctx, tx, a.TenantID, "payroll.settlement.paid", map[string]string{"settlement_id": set.ID.String(), "employee_id": set.EmployeeProfileID.String()})
	})
}

// Cancel withdraws a draft or approved settlement (PP-07).
func (s *Service) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*Settlement, error) {
	return s.transition(ctx, a, id, "cancelled", func(_ *gorm.DB, set *Settlement) error {
		if set.Status != StatusDraft && set.Status != StatusApproved {
			return ErrState
		}
		set.Status = StatusCancelled
		return nil
	})
}

// Get returns one settlement with its lines.
func (s *Service) Get(ctx context.Context, a Actor, id uuid.UUID) (*Settlement, error) {
	set, err := s.store.Settlement(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || set == nil {
		return nil, orNotFound(err)
	}
	return s.withLines(ctx, set)
}

// List is PP-08.
func (s *Service) List(ctx context.Context, a Actor, employeeID *uuid.UUID, status string) ([]Settlement, error) {
	switch status {
	case "", StatusDraft, StatusApproved, StatusPaid, StatusCancelled:
	default:
		return nil, invalid("status", "must be draft, approved, paid or cancelled")
	}
	rows, err := s.store.Settlements(ctx, s.store.DB(), a.TenantID, employeeID, status, 200)
	if rows == nil {
		rows = []Settlement{}
	}
	return rows, err
}

// Mine is PP-08: the caller own paid settlements.
func (s *Service) Mine(ctx context.Context, a Actor) ([]Settlement, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, ErrEmployeeNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.List(ctx, a, &me.ID, StatusPaid)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if _, err := s.withLines(ctx, &rows[i]); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
