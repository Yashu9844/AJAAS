package plus

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	attendanceAPI "github.com/jaas/jaas/internal/hr/attendance/api"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/leave/services"
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
	ErrNotFound          = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound  = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrEmployeeNotActive = appErr(http.StatusForbidden, "EMPLOYEE_NOT_ACTIVE", "the employee profile is not in a working status")
	ErrLocationNotFound  = appErr(http.StatusNotFound, "LOCATION_NOT_FOUND", "no such organization location")
	ErrConflict          = appErr(http.StatusConflict, "CONFLICT", "the request conflicts with an existing record")
	ErrYearLocked        = appErr(http.StatusConflict, "YEAR_LOCKED", "the leave year cannot change once balances exist")
	ErrInactiveType      = appErr(http.StatusConflict, "INACTIVE_REFERENCE", "the leave type must be active")
	ErrCompOffConfig     = appErr(http.StatusConflict, "COMP_OFF_NOT_CONFIGURED", "the policy has no comp-off leave type")
	ErrWorkingDay        = appErr(http.StatusConflict, "NOT_A_NON_WORKING_DAY", "comp-off is earned on a holiday or weekly off")
	ErrNoWork            = appErr(http.StatusConflict, "NO_WORK_RECORDED", "no attendance work is recorded for that day")
	ErrNotEncashable     = appErr(http.StatusConflict, "NOT_ENCASHABLE", "the leave type has no encashment rule")
	ErrEncashLimit       = appErr(http.StatusConflict, "ENCASH_LIMIT", "the yearly encashment limit would be exceeded")
	ErrInsufficient      = appErr(http.StatusConflict, "INSUFFICIENT_BALANCE", "the balance does not cover the encashment")
	ErrNotPending        = appErr(http.StatusConflict, "REQUEST_STATE", "the request is not pending")
	ErrSelfApproval      = appErr(http.StatusForbidden, "SELF_APPROVAL_FORBIDDEN", "you cannot decide your own request")
	ErrRange             = appErr(http.StatusBadRequest, "INVALID_RANGE", "the range is at most 62 days")
)

// Directory is plus' view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// Reports resolves a manager's direct reports (HR/Organization mappings).
type Reports interface {
	DirectReportUserIDs(ctx context.Context, tenantID, managerUserID uuid.UUID) ([]uuid.UUID, error)
}

// Locations resolves an employee's organization location on a date (HR/Core assignment).
type Locations interface {
	LocationOn(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (*uuid.UUID, error)
}

// FromCore adapts core/api.Directory to Locations.
func FromCore(d coreAPI.Directory) Locations { return coreLocations{d} }

type coreLocations struct{ d coreAPI.Directory }

func (c coreLocations) LocationOn(ctx context.Context, tenantID, employeeID uuid.UUID, on time.Time) (*uuid.UUID, error) {
	p, err := c.d.PlacementOn(ctx, tenantID, employeeID, on)
	if err != nil || p == nil {
		return nil, err
	}
	return p.LocationID, nil
}

// Ports are the collaborators plus consumes.
type Ports struct {
	Employees Directory
	Reports   Reports
	Requests  services.RequestService
	Mover     services.BalanceMover
	Presence  attendanceAPI.Presence
	Audit     identityport.Auditor
}

// Actor is the caller; the flags are leave:read, leave:manage and leave:approve at tenant scope.
type Actor struct {
	TenantID, UserID                  uuid.UUID
	IP, UserAgent                     string
	CanRead, CanManage, CanApproveAll bool
}

func (a Actor) leaveActor() services.Actor {
	return services.Actor{TenantID: a.TenantID, UserID: a.UserID, IP: a.IP, UserAgent: a.UserAgent, CorrelationID: uuid.New()}
}

// Calendar is services.CalendarSource over policy, location holidays and week-off patterns (LP-02..LP-05).
type Calendar struct {
	store Store
	mu    sync.RWMutex
	loc   Locations
}

// NewCalendar builds the calendar; locations arrive later through SetLocations (core is built after leave).
func NewCalendar(store Store) *Calendar { return &Calendar{store: store} }

// SetLocations installs the location source.
func (c *Calendar) SetLocations(l Locations) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loc = l
}

func (c *Calendar) locations() Locations {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loc
}

// YearStartMonth implements services.CalendarSource.
func (c *Calendar) YearStartMonth(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int, error) {
	p, err := c.store.Policy(ctx, db, tenantID)
	if err != nil || p == nil {
		return defaultYearStart, err
	}
	return p.YearStartMonth, nil
}

// For implements services.CalendarSource: the employee's location on `from` decides the extra holidays and weekly off.
func (c *Calendar) For(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]time.Time, func(time.Time) bool, error) {
	loc := c.locations()
	if loc == nil {
		return nil, nil, nil
	}
	id, err := loc.LocationOn(ctx, tenantID, employeeID, from)
	if err != nil || id == nil {
		return nil, nil, err
	}
	rows, err := c.store.LocationHolidays(ctx, db, tenantID, id, from, to)
	if err != nil {
		return nil, nil, err
	}
	var holidays []time.Time
	for _, h := range rows {
		if !h.IsOptional {
			holidays = append(holidays, h.HolidayDate.Time)
		}
	}
	w, err := c.store.WeekOff(ctx, db, tenantID, *id)
	if err != nil || w == nil {
		return holidays, nil, err
	}
	return holidays, maskOf(w.WeekOffMask), nil
}

// DayKind implements leave/api.Holidays for one worker and date: tenant or location holiday, and the location's weekly
// off when it has a pattern (nil otherwise).
func (c *Calendar) DayKind(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (bool, *bool, error) {
	tenant, err := c.store.TenantHolidays(ctx, db, tenantID, date, date)
	if err != nil {
		return false, nil, err
	}
	holiday := false
	for _, h := range tenant {
		holiday = holiday || !h.IsOptional
	}
	extra, weekOff, err := c.For(ctx, db, tenantID, employeeID, date, date)
	if err != nil {
		return false, nil, err
	}
	holiday = holiday || len(extra) > 0
	if weekOff == nil {
		return holiday, nil, nil
	}
	off := weekOff(date)
	return holiday, &off, nil
}

// Service implements the leave parity requirements (LP-01..LP-18).
type Service struct {
	store Store
	p     Ports
	cal   *Calendar
	now   func() time.Time
}

// NewService builds the service.
func NewService(store Store, p Ports, cal *Calendar, now func() time.Time) *Service {
	return &Service{store: store, p: p, cal: cal, now: now}
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

func text(field, v string, max int) (string, error) {
	t := strings.TrimSpace(v)
	if t == "" || utf8.RuneCountInString(t) > max {
		return "", invalid(field, "must be 1–500 characters")
	}
	return t, nil
}

func optionalText(field string, v *string, max int) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := text(field, *v, max)
	return &t, err
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

var workingStatuses = map[string]bool{"active": true, "probation": true, "notice": true}

// self resolves the caller's employee profile, or nil.
func (s *Service) self(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, nil
	}
	return me, err
}

// worker resolves the caller as a working employee (comp-off and encashment requests).
func (s *Service) worker(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	if me == nil {
		return nil, ErrEmployeeNotFound
	}
	if !workingStatuses[me.Status] {
		return nil, ErrEmployeeNotActive
	}
	return me, nil
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

// mayDecide reports whether the caller may decide a request of the employee: tenant-scope approvers, or the manager.
func (s *Service) mayDecide(ctx context.Context, a Actor, employeeID uuid.UUID) (bool, error) {
	if a.CanApproveAll {
		return true, nil
	}
	reports, err := s.reportIDs(ctx, a)
	return reports[employeeID], err
}

// --- policy (LP-01) ---

// PolicyInput replaces the policy.
type PolicyInput struct {
	YearStartMonth     int     `json:"year_start_month"`
	CompOffLeaveTypeID *string `json:"comp_off_leave_type_id"`
	CompOffExpiryDays  int     `json:"comp_off_expiry_days"`
}

// GetPolicy returns the saved policy or the defaults.
func (s *Service) GetPolicy(ctx context.Context, a Actor) (*Policy, error) {
	p, err := s.store.Policy(ctx, s.store.DB(), a.TenantID)
	if err != nil || p != nil {
		return p, err
	}
	return defaults(a.TenantID), nil
}

// SetPolicy is LP-01.
func (s *Service) SetPolicy(ctx context.Context, a Actor, in PolicyInput) (*Policy, error) {
	if in.YearStartMonth < 1 || in.YearStartMonth > 12 {
		return nil, invalid("year_start_month", "must be 1–12")
	}
	if in.CompOffExpiryDays < 1 || in.CompOffExpiryDays > 365 {
		return nil, invalid("comp_off_expiry_days", "must be 1–365")
	}
	p := &Policy{TenantID: a.TenantID, YearStartMonth: in.YearStartMonth, CompOffExpiryDays: in.CompOffExpiryDays, UpdatedByUserID: a.UserID, UpdatedAt: s.now().UTC()}
	if in.CompOffLeaveTypeID != nil && *in.CompOffLeaveTypeID != "" {
		id, err := uuid.Parse(*in.CompOffLeaveTypeID)
		if err != nil {
			return nil, invalid("comp_off_leave_type_id", "must be a UUID")
		}
		exists, active, err := s.store.TypeState(ctx, s.store.DB(), a.TenantID, id)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
		if !active {
			return nil, ErrInactiveType
		}
		p.CompOffLeaveTypeID = &id
	}
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		cur, err := s.store.Policy(ctx, tx, a.TenantID)
		if err != nil {
			return err
		}
		curMonth := defaultYearStart
		if cur != nil {
			curMonth = cur.YearStartMonth
		}
		if in.YearStartMonth != curMonth {
			if used, err := s.store.BalancesExist(ctx, tx, a.TenantID); err != nil || used {
				if err != nil {
					return err
				}
				return ErrYearLocked
			}
		}
		return s.store.SavePolicy(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.policy_saved", "leave_policy", a.TenantID.String(), nil)
	return p, nil
}

// --- location calendars (LP-03, LP-04) ---

// LocationHolidayInput creates a location holiday.
type LocationHolidayInput struct {
	LocationID string `json:"location_id"`
	Date       string `json:"date"`
	Name       string `json:"name"`
	IsOptional bool   `json:"is_optional"`
}

func isForeignKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "foreign key")
}

// CreateLocationHoliday is LP-03.
func (s *Service) CreateLocationHoliday(ctx context.Context, a Actor, in LocationHolidayInput) (*LocationHoliday, error) {
	loc, err := uuid.Parse(in.LocationID)
	if err != nil {
		return nil, invalid("location_id", "must be a UUID")
	}
	date, err := parseDate("date", in.Date)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return nil, invalid("name", "must be 1–100 characters")
	}
	h := &LocationHoliday{TenantID: a.TenantID, LocationID: loc, HolidayDate: Date{date}, Name: name, IsOptional: in.IsOptional,
		CreatedByUserID: a.UserID, CreatedAt: s.now().UTC()}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if taken, err := s.store.HolidayTaken(ctx, tx, a.TenantID, loc, date); err != nil || taken {
			if err != nil {
				return err
			}
			return ErrConflict
		}
		return s.store.Create(ctx, tx, h)
	})
	if isForeignKey(err) {
		return nil, ErrLocationNotFound
	}
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.location_holiday_created", "leave_location_holiday", h.ID.String(), map[string]string{"location_id": loc.String(), "date": in.Date})
	return h, nil
}

// LocationHolidays lists holidays in a year of a location (default: the current year, all locations).
func (s *Service) LocationHolidays(ctx context.Context, a Actor, locationID, year string) ([]LocationHoliday, error) {
	var loc *uuid.UUID
	if locationID != "" {
		id, err := uuid.Parse(locationID)
		if err != nil {
			return nil, invalid("location_id", "must be a UUID")
		}
		loc = &id
	}
	y := s.today().Year()
	if year != "" {
		t, err := time.Parse("2006", year)
		if err != nil {
			return nil, invalid("year", "must be YYYY")
		}
		y = t.Year()
	}
	rows, err := s.store.LocationHolidays(ctx, s.store.DB(), a.TenantID, loc, time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC))
	if rows == nil {
		rows = []LocationHoliday{}
	}
	return rows, err
}

// DeleteLocationHoliday removes a location holiday; existing requests keep their snapshot (LV-012).
func (s *Service) DeleteLocationHoliday(ctx context.Context, a Actor, id uuid.UUID) error {
	h, err := s.store.LocationHoliday(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || h == nil {
		return orNotFound(err)
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Delete(ctx, tx, h) }); err != nil {
		return err
	}
	s.log(ctx, a, "leave.location_holiday_deleted", "leave_location_holiday", id.String(), nil)
	return nil
}

// WeekOffView is a location's pattern.
type WeekOffView struct {
	LocationID uuid.UUID `json:"location_id"`
	Days       []int     `json:"week_off_days"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func weekOffView(w *WeekOff) *WeekOffView {
	return &WeekOffView{LocationID: w.LocationID, Days: w.days(), UpdatedAt: w.UpdatedAt}
}

// SetWeekOff is LP-04.
func (s *Service) SetWeekOff(ctx context.Context, a Actor, locationID string, days []int) (*WeekOffView, error) {
	loc, err := uuid.Parse(locationID)
	if err != nil {
		return nil, invalid("location_id", "must be a UUID")
	}
	mask, seen := 0, map[int]bool{}
	for _, d := range days {
		if d < 0 || d > 6 || seen[d] {
			return nil, invalid("week_off_days", "must be distinct weekday numbers 0 (Sunday) to 6")
		}
		seen[d] = true
		mask |= 1 << uint(d)
	}
	w := &WeekOff{TenantID: a.TenantID, LocationID: loc, WeekOffMask: mask, UpdatedByUserID: a.UserID, UpdatedAt: s.now().UTC()}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SaveWeekOff(ctx, tx, w) })
	if isForeignKey(err) {
		return nil, ErrLocationNotFound
	}
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.week_off_saved", "leave_week_off", loc.String(), nil)
	return weekOffView(w), nil
}

// WeekOffs lists every location pattern.
func (s *Service) WeekOffs(ctx context.Context, a Actor) ([]WeekOffView, error) {
	rows, err := s.store.WeekOffs(ctx, s.store.DB(), a.TenantID)
	out := make([]WeekOffView, 0, len(rows))
	for i := range rows {
		out = append(out, *weekOffView(&rows[i]))
	}
	return out, err
}

// DeleteWeekOff removes a location pattern (the default Saturday/Sunday applies again).
func (s *Service) DeleteWeekOff(ctx context.Context, a Actor, locationID string) error {
	loc, err := uuid.Parse(locationID)
	if err != nil {
		return invalid("location_id", "must be a UUID")
	}
	w, err := s.store.WeekOff(ctx, s.store.DB(), a.TenantID, loc)
	if err != nil || w == nil {
		return orNotFound(err)
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Delete(ctx, tx, w) }); err != nil {
		return err
	}
	s.log(ctx, a, "leave.week_off_deleted", "leave_week_off", loc.String(), nil)
	return nil
}
