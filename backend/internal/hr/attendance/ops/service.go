package ops

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	attDTO "github.com/jaas/jaas/internal/hr/attendance/dto"
	attServices "github.com/jaas/jaas/internal/hr/attendance/services"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/identityport"
	leaveAPI "github.com/jaas/jaas/internal/hr/leave/api"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

const (
	maxInbox          = 200
	maxRosterSegments = 12
	maxRosterWorkers  = 100
	maxRosterAssigns  = 200
	maxRosterCycles   = 26
	jobWindowDays     = 31
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
	ErrRosterOverlap    = appErr(http.StatusConflict, "ROSTER_OVERLAP", "the roster overlaps an existing shift assignment")
	ErrRosterArchived   = appErr(http.StatusConflict, "ROSTER_ARCHIVED", "an archived roster cannot be applied")
	ErrNameTaken        = appErr(http.StatusConflict, "CONFLICT", "a roster with this name already exists")
	ErrShiftInactive    = appErr(http.StatusConflict, "INACTIVE_REFERENCE", "every shift of the pattern must be an active shift")
	ErrJobWindow        = appErr(http.StatusBadRequest, "JOB_WINDOW", "the date must be before today and within the last 31 days")
	ErrImportTooLarge   = appErr(http.StatusBadRequest, "IMPORT_TOO_LARGE", "an import holds 1–500 rows")
)

// Directory is ops' view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error)
}

// Reports resolves a manager's direct reports (HR/Organization mappings).
type Reports interface {
	DirectReportUserIDs(ctx context.Context, tenantID, managerUserID uuid.UUID) ([]uuid.UUID, error)
}

// Ports are the collaborators ops consumes.
type Ports struct {
	Employees Directory
	Reports   Reports
	Writer    attServices.TimeWriter
	Regs      attServices.RegularizationService
	Holidays  leaveAPI.Holidays
	Audit     identityport.Auditor
}

// Actor is the caller; CanApproveAll is attendance:approve at tenant scope.
type Actor struct {
	TenantID, UserID uuid.UUID
	IP, UserAgent    string
	CanApproveAll    bool
}

// Service implements the attendance parity requirements (AP-01..AP-12).
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

func parseDate(field, raw string) (time.Time, error) {
	t, err := time.Parse(DateLayout, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, invalid(field, "must be YYYY-MM-DD")
	}
	return t, nil
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

func (s *Service) log(ctx context.Context, a Actor, action, resource, id string, meta map[string]string) {
	_ = s.p.Audit.Log(ctx, s.store.DB(), a.TenantID.String(), a.UserID.String(), action, resource, id, meta, a.IP, a.UserAgent)
}

// self resolves the caller's employee profile, or nil.
func (s *Service) self(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, nil
	}
	return me, err
}

// --- policy (AP-03) ---

// PolicyInput replaces the policy; every field is required except the switch, which defaults to off.
type PolicyInput struct {
	WeekOffDays         []int `json:"week_off_days"`
	OvertimeMinDaily    int   `json:"overtime_min_daily"`
	OvertimeMaxDaily    int   `json:"overtime_max_daily"`
	OvertimeMaxMonthly  int   `json:"overtime_max_monthly"`
	OvertimeRateWeekday int   `json:"overtime_rate_weekday"`
	OvertimeRateWeekOff int   `json:"overtime_rate_week_off"`
	AutoAbsentEnabled   bool  `json:"auto_absent_enabled"`
}

// PolicyView is the policy as the API shows it.
type PolicyView struct {
	Policy
	WeekOffDays []int `json:"week_off_days"`
}

func view(p *Policy) *PolicyView { return &PolicyView{Policy: *p, WeekOffDays: p.weekOffDays()} }

// GetPolicy returns the saved policy or the defaults.
func (s *Service) GetPolicy(ctx context.Context, a Actor) (*PolicyView, error) {
	p, err := s.policy(ctx, s.store.DB(), a.TenantID)
	if err != nil {
		return nil, err
	}
	return view(p), nil
}

func (s *Service) policy(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*Policy, error) {
	p, err := s.store.Policy(ctx, db, tenantID)
	if err != nil || p != nil {
		return p, err
	}
	return defaults(tenantID), nil
}

// SetPolicy is AP-03.
func (s *Service) SetPolicy(ctx context.Context, a Actor, in PolicyInput) (*PolicyView, error) {
	mask, seen := 0, map[int]bool{}
	for _, d := range in.WeekOffDays {
		if d < 0 || d > 6 || seen[d] {
			return nil, invalid("week_off_days", "must be distinct weekday numbers 0 (Sunday) to 6")
		}
		seen[d] = true
		mask |= 1 << uint(d)
	}
	switch {
	case in.OvertimeMinDaily < 0 || in.OvertimeMinDaily > in.OvertimeMaxDaily || in.OvertimeMaxDaily > 1440:
		return nil, invalid("overtime_min_daily", "must satisfy 0 ≤ min ≤ daily max ≤ 1440")
	case in.OvertimeMaxMonthly < 0 || in.OvertimeMaxMonthly > 44640:
		return nil, invalid("overtime_max_monthly", "must be 0–44640 minutes")
	case in.OvertimeRateWeekday < 100 || in.OvertimeRateWeekday > 500 || in.OvertimeRateWeekOff < 100 || in.OvertimeRateWeekOff > 500:
		return nil, invalid("overtime_rate", "rates are hundredths of the hourly rate, 100–500")
	}
	p := &Policy{TenantID: a.TenantID, WeekOffMask: mask, OvertimeMinDaily: in.OvertimeMinDaily, OvertimeMaxDaily: in.OvertimeMaxDaily,
		OvertimeMaxMonthly: in.OvertimeMaxMonthly, OvertimeRateWeekday: in.OvertimeRateWeekday, OvertimeRateWeekOff: in.OvertimeRateWeekOff,
		AutoAbsentEnabled: in.AutoAbsentEnabled, UpdatedByUserID: a.UserID, UpdatedAt: s.now().UTC()}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SavePolicy(ctx, tx, p) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "attendance.policy_saved", "attendance_policy", a.TenantID.String(), nil)
	return view(p), nil
}

// --- inbox (AP-01, AP-02) ---

// InboxItem is one pending regularization for a manager.
type InboxItem struct {
	ID                uuid.UUID `json:"id"`
	EmployeeID        uuid.UUID `json:"employee_id"`
	EmployeeName      string    `json:"employee_name"`
	AttendanceDate    Date      `json:"attendance_date"`
	RequestedPunchIn  time.Time `json:"requested_punch_in"`
	RequestedPunchOut time.Time `json:"requested_punch_out"`
	Reason            string    `json:"reason"`
	CreatedAt         time.Time `json:"created_at"`
}

// reportIDs resolves the caller's direct reports to employee profile ids.
func (s *Service) reportIDs(ctx context.Context, a Actor) (map[uuid.UUID]bool, error) {
	users, err := s.p.Reports.DirectReportUserIDs(ctx, a.TenantID, a.UserID)
	if err != nil {
		return nil, err
	}
	ids := map[uuid.UUID]bool{}
	for _, u := range users {
		e, err := s.p.Employees.GetByUserID(ctx, a.TenantID, u)
		if errors.Is(err, sharedErrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ids[e.ID] = true
	}
	return ids, nil
}

// Inbox is AP-01: pending requests of the caller's reports, or every pending one for tenant-scope approvers.
func (s *Service) Inbox(ctx context.Context, a Actor, all bool) ([]InboxItem, error) {
	if all && !a.CanApproveAll {
		return nil, appErr(http.StatusForbidden, "FORBIDDEN", "scope=all needs attendance:approve at tenant scope")
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
	rows, err := s.store.PendingRegularizations(ctx, s.store.DB(), a.TenantID, ids, all, maxInbox)
	if err != nil {
		return nil, err
	}
	out := make([]InboxItem, 0, len(rows))
	names := map[uuid.UUID]string{}
	for _, r := range rows {
		if me != nil && r.EmployeeProfileID == me.ID {
			continue
		}
		name, known := names[r.EmployeeProfileID]
		if !known {
			if e, err := s.p.Employees.GetByID(ctx, a.TenantID, r.EmployeeProfileID); err == nil && e != nil {
				name = e.DisplayName
			} else if err != nil && !errors.Is(err, sharedErrors.ErrNotFound) {
				return nil, err
			}
			names[r.EmployeeProfileID] = name
		}
		out = append(out, InboxItem{ID: r.ID, EmployeeID: r.EmployeeProfileID, EmployeeName: name, AttendanceDate: Date{r.AttendanceDate},
			RequestedPunchIn: r.RequestedPunchIn, RequestedPunchOut: r.RequestedPunchOut, Reason: r.Reason, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Decide is AP-02: the report's manager (or a tenant-scope approver) approves or rejects through the AT-016 rules.
func (s *Service) Decide(ctx context.Context, a Actor, id uuid.UUID, approve bool, comment *string) (*attDTO.RegularizationResponse, error) {
	reg, err := s.store.Regularization(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || reg == nil {
		return nil, orNotFound(err)
	}
	if !a.CanApproveAll {
		reports, err := s.reportIDs(ctx, a)
		if err != nil {
			return nil, err
		}
		if !reports[reg.EmployeeProfileID] {
			return nil, ErrNotFound
		}
	}
	actor := attServices.Actor{TenantID: a.TenantID, UserID: a.UserID, IP: a.IP, UserAgent: a.UserAgent, CorrelationID: uuid.New()}
	req := attDTO.ReviewRequest{Comment: comment}
	if approve {
		return s.p.Regs.Approve(ctx, actor, id, req)
	}
	return s.p.Regs.Reject(ctx, actor, id, req)
}

// --- overtime report (AP-04) ---

// OvertimeDay is one eligible day.
type OvertimeDay struct {
	Date    Date `json:"date"`
	Minutes int  `json:"minutes"`
	Rate    int  `json:"rate"`
	WeekOff bool `json:"week_off"`
}

// OvertimeLine is one worker's month.
type OvertimeLine struct {
	EmployeeID      uuid.UUID     `json:"employee_id"`
	EligibleMinutes int           `json:"eligible_minutes"`
	WeightedMinutes int           `json:"weighted_minutes"`
	Capped          bool          `json:"capped"`
	Days            []OvertimeDay `json:"days"`
}

// OvertimeReport is the month report.
type OvertimeReport struct {
	Month string         `json:"month"`
	Lines []OvertimeLine `json:"lines"`
}

// Overtime is AP-04: threshold, daily cap, monthly cap (earliest days first) and rate-weighted minutes, in integers.
func (s *Service) Overtime(ctx context.Context, a Actor, month string, employeeID *uuid.UUID) (*OvertimeReport, error) {
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, invalid("month", "must be YYYY-MM")
	}
	p, err := s.policy(ctx, s.store.DB(), a.TenantID)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.OvertimeRows(ctx, s.store.DB(), a.TenantID, first, first.AddDate(0, 1, -1), employeeID)
	if err != nil {
		return nil, err
	}
	rep := &OvertimeReport{Month: month, Lines: []OvertimeLine{}}
	var cur *OvertimeLine
	for _, r := range rows {
		if cur == nil || cur.EmployeeID != r.EmployeeID {
			rep.Lines = append(rep.Lines, OvertimeLine{EmployeeID: r.EmployeeID, Days: []OvertimeDay{}})
			cur = &rep.Lines[len(rep.Lines)-1]
		}
		off := p.weekOff(r.Date)
		minutes, rate := r.OvertimeMinutes, p.OvertimeRateWeekday
		if off {
			minutes, rate = r.WorkMinutes, p.OvertimeRateWeekOff
		}
		if minutes < p.OvertimeMinDaily || minutes <= 0 {
			continue
		}
		minutes = min(minutes, p.OvertimeMaxDaily)
		if room := p.OvertimeMaxMonthly - cur.EligibleMinutes; minutes > room {
			minutes, cur.Capped = max(room, 0), true
		}
		if minutes == 0 {
			continue
		}
		cur.EligibleMinutes += minutes
		cur.WeightedMinutes += minutes * rate / 100
		cur.Days = append(cur.Days, OvertimeDay{Date: Date{r.Date}, Minutes: minutes, Rate: rate, WeekOff: off})
	}
	kept := rep.Lines[:0]
	for _, l := range rep.Lines {
		if l.EligibleMinutes > 0 {
			kept = append(kept, l)
		}
	}
	rep.Lines = kept
	return rep, nil
}

// --- rosters (AP-05, AP-06) ---

// RosterInput creates or edits a roster.
type RosterInput struct {
	Name    *string    `json:"name"`
	Pattern *[]Segment `json:"pattern"`
	Status  *string    `json:"status"`
}

func (s *Service) fillRoster(ctx context.Context, tenantID uuid.UUID, r *Roster, in RosterInput) error {
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || utf8.RuneCountInString(n) > 100 {
			return invalid("name", "must be 1–100 characters")
		}
		r.Name = n
	}
	if in.Status != nil {
		if *in.Status != RosterActive && *in.Status != RosterArchived {
			return invalid("status", "must be active or archived")
		}
		r.Status = *in.Status
	}
	if in.Pattern != nil {
		if len(*in.Pattern) == 0 || len(*in.Pattern) > maxRosterSegments {
			return invalid("pattern", "must hold 1–12 segments")
		}
		for _, seg := range *in.Pattern {
			if seg.Days < 1 || seg.Days > 31 {
				return invalid("pattern", "each segment runs 1–31 days")
			}
			exists, active, err := s.p.Writer.ShiftState(ctx, s.store.DB(), tenantID, seg.ShiftID)
			if err != nil {
				return err
			}
			if !exists {
				return invalid("pattern", "unknown shift")
			}
			if !active {
				return ErrShiftInactive
			}
		}
		r.Pattern = Pattern(*in.Pattern)
	}
	return nil
}

// CreateRoster is AP-05.
func (s *Service) CreateRoster(ctx context.Context, a Actor, in RosterInput) (*Roster, error) {
	if in.Name == nil || in.Pattern == nil {
		return nil, invalid("body", "name and pattern are required")
	}
	now := s.now().UTC()
	r := &Roster{TenantID: a.TenantID, Status: RosterActive, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.fillRoster(ctx, a.TenantID, r, in); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		if taken, err := s.store.RosterNameTaken(ctx, tx, a.TenantID, r.Name, nil); err != nil || taken {
			if err != nil {
				return err
			}
			return ErrNameTaken
		}
		return s.store.Create(ctx, tx, r)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "attendance.roster_created", "attendance_roster", r.ID.String(), nil)
	return r, nil
}

// UpdateRoster edits a roster.
func (s *Service) UpdateRoster(ctx context.Context, a Actor, id uuid.UUID, in RosterInput) (*Roster, error) {
	r, err := s.store.Roster(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	if err := s.fillRoster(ctx, a.TenantID, r, in); err != nil {
		return nil, err
	}
	r.UpdatedAt = s.now().UTC()
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if taken, err := s.store.RosterNameTaken(ctx, tx, a.TenantID, r.Name, &r.ID); err != nil || taken {
			if err != nil {
				return err
			}
			return ErrNameTaken
		}
		return s.store.Save(ctx, tx, r)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "attendance.roster_updated", "attendance_roster", id.String(), nil)
	return r, nil
}

// GetRoster returns one roster.
func (s *Service) GetRoster(ctx context.Context, a Actor, id uuid.UUID) (*Roster, error) {
	r, err := s.store.Roster(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	return r, nil
}

// Rosters lists rosters.
func (s *Service) Rosters(ctx context.Context, a Actor, status string) ([]Roster, error) {
	if status != "" && status != RosterActive && status != RosterArchived {
		return nil, invalid("status", "must be active or archived")
	}
	rows, err := s.store.Rosters(ctx, s.store.DB(), a.TenantID, status)
	if rows == nil {
		rows = []Roster{}
	}
	return rows, err
}

// ApplyInput lays a roster over workers.
type ApplyInput struct {
	EmployeeIDs []string `json:"employee_ids"`
	From        string   `json:"from"`
	Cycles      int      `json:"cycles"`
}

// ApplyResult counts what was created.
type ApplyResult struct {
	Workers     int `json:"workers"`
	Assignments int `json:"assignments"`
}

// ApplyRoster is AP-06: segments end to end as ordinary shift assignments, all or nothing.
func (s *Service) ApplyRoster(ctx context.Context, a Actor, id uuid.UUID, in ApplyInput) (*ApplyResult, error) {
	from, err := parseDate("from", in.From)
	if err != nil {
		return nil, err
	}
	if in.Cycles < 1 || in.Cycles > maxRosterCycles {
		return nil, invalid("cycles", "must be 1–26")
	}
	if len(in.EmployeeIDs) == 0 || len(in.EmployeeIDs) > maxRosterWorkers {
		return nil, invalid("employee_ids", "must hold 1–100 workers")
	}
	workers, seen := []uuid.UUID{}, map[uuid.UUID]bool{}
	for _, raw := range in.EmployeeIDs {
		id, err := uuid.Parse(raw)
		if err != nil || seen[id] {
			return nil, invalid("employee_ids", "must be distinct UUIDs")
		}
		seen[id] = true
		workers = append(workers, id)
	}
	r, err := s.store.Roster(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	if r.Status != RosterActive {
		return nil, ErrRosterArchived
	}
	if len(r.Pattern)*in.Cycles > maxRosterAssigns {
		return nil, invalid("cycles", "a worker may receive at most 200 assignments")
	}
	for _, seg := range r.Pattern {
		if _, active, err := s.p.Writer.ShiftState(ctx, s.store.DB(), a.TenantID, seg.ShiftID); err != nil {
			return nil, err
		} else if !active {
			return nil, ErrShiftInactive
		}
	}
	for _, w := range workers {
		if _, err := s.p.Employees.GetByID(ctx, a.TenantID, w); errors.Is(err, sharedErrors.ErrNotFound) {
			return nil, ErrEmployeeNotFound
		} else if err != nil {
			return nil, err
		}
	}
	res := &ApplyResult{Workers: len(workers)}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		for _, w := range workers {
			cursor := from
			for c := 0; c < in.Cycles; c++ {
				for _, seg := range r.Pattern {
					end := cursor.AddDate(0, 0, seg.Days-1)
					if err := s.p.Writer.AssignShift(ctx, tx, a.TenantID, w, seg.ShiftID, cursor, end, a.UserID); err != nil {
						if errors.Is(err, attServices.ErrAssignmentOverlap) {
							return appErr(http.StatusConflict, ErrRosterOverlap.Code, "the roster overlaps an existing assignment of worker "+w.String())
						}
						return err
					}
					res.Assignments++
					cursor = end.AddDate(0, 0, 1)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "attendance.roster_applied", "attendance_roster", id.String(), map[string]string{"workers": strconv.Itoa(res.Workers), "assignments": strconv.Itoa(res.Assignments)})
	return res, nil
}
