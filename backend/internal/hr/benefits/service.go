package benefits

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/hr/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Defaults and limits (BN-C01..BN-C05).
const (
	defaultWindowDays = 30
	maxStartAhead     = 90
	maxWindowSpanDays = 120
	maxMonthlyCost    = kernel.Money(100_000_000) // 1,000,000.00 per month
)

var codePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{2,30}$`)
var levelPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,30}$`)

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Errors (specification.md §4).
var (
	ErrNotFound         = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrForbidden        = appErr(http.StatusForbidden, "FORBIDDEN", "you may not do this")
	ErrConflict         = appErr(http.StatusConflict, "CONFLICT", "already exists")
	ErrPlanState        = appErr(http.StatusConflict, "PLAN_STATE", "the plan's status does not allow this")
	ErrNotEligible      = appErr(http.StatusConflict, "NOT_ELIGIBLE", "you are not eligible for this plan")
	ErrWindowClosed     = appErr(http.StatusConflict, "ENROLMENT_WINDOW_CLOSED", "no enrolment window is open for you")
	ErrElectionExists   = appErr(http.StatusConflict, "ELECTION_EXISTS", "you already have a live election for this plan; change or end it")
	ErrElectionState    = appErr(http.StatusConflict, "ELECTION_STATE", "the election's status does not allow this")
	ErrDependantInUse   = appErr(http.StatusConflict, "DEPENDANT_IN_USE", "an active election covers this dependant")
)

// Directory is benefits' view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// Ports are the collaborators benefits consumes.
type Ports struct {
	Employees Directory
	Core      coreAPI.Directory
	Audit     identityport.Auditor
}

// Actor is the caller; the flags are benefits:read / manage / read_sensitive.
type Actor struct {
	TenantID, UserID                     uuid.UUID
	IP, UserAgent                        string
	CanRead, CanManage, CanSensitiveRead bool
}

// Service implements benefits (BN-C01..BN-C11).
type Service struct {
	store Store
	p     Ports
	now   func() time.Time
}

// NewService builds the service.
func NewService(store Store, p Ports, now func() time.Time) *Service {
	return &Service{store: store, p: p, now: now}
}

func (s *Service) log(ctx context.Context, a Actor, action, resource, id string, meta map[string]string) {
	_ = s.p.Audit.Log(ctx, s.store.DB(), a.TenantID.String(), a.UserID.String(), action, resource, id, meta, a.IP, a.UserAgent)
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) today() Date { return Date{midnight(s.now().UTC())} }

func parseDate(field, raw string) (Date, error) {
	t, err := time.Parse(DateLayout, strings.TrimSpace(raw))
	if err != nil {
		return Date{}, invalid(field, "must be YYYY-MM-DD")
	}
	return Date{t}, nil
}

func required(field, v string, max int) (string, error) {
	t := strings.TrimSpace(v)
	if t == "" {
		return "", invalid(field, "is required")
	}
	if utf8.RuneCountInString(t) > max {
		return "", invalid(field, "is too long")
	}
	return t, nil
}

func optional(field string, v *string, max int) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := required(field, *v, max)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) me(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	e, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && e == nil) {
		return nil, ErrEmployeeNotFound
	}
	return e, err
}

func (s *Service) worker(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	e, err := s.p.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && e == nil) {
		return nil, ErrEmployeeNotFound
	}
	return e, err
}

// --- settings (BN-C04) ---

// SettingsInput is the settings body.
type SettingsInput struct {
	NewHireDays   *int `json:"new_hire_days"`
	LifeEventDays *int `json:"life_event_days"`
}

func (s *Service) settings(ctx context.Context, tenantID uuid.UUID) (Settings, error) {
	st, err := s.store.Settings(ctx, s.store.DB(), tenantID)
	if err != nil {
		return Settings{}, err
	}
	if st == nil {
		return Settings{TenantID: tenantID, NewHireDays: defaultWindowDays, LifeEventDays: defaultWindowDays}, nil
	}
	return *st, nil
}

// GetSettings returns the window lengths or the defaults.
func (s *Service) GetSettings(ctx context.Context, a Actor) (*Settings, error) {
	st, err := s.settings(ctx, a.TenantID)
	return &st, err
}

// PutSettings sets the new-hire and life-event window lengths in days (0 to 365).
func (s *Service) PutSettings(ctx context.Context, a Actor, in SettingsInput) (*Settings, error) {
	st, err := s.settings(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	for field, v := range map[string]*int{"new_hire_days": in.NewHireDays, "life_event_days": in.LifeEventDays} {
		if v != nil && (*v < 0 || *v > 365) {
			return nil, invalid(field, "must be between 0 and 365")
		}
	}
	if in.NewHireDays != nil {
		st.NewHireDays = *in.NewHireDays
	}
	if in.LifeEventDays != nil {
		st.LifeEventDays = *in.LifeEventDays
	}
	st.UpdatedByUserID, st.UpdatedAt = a.UserID, s.now().UTC()
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SaveSettings(ctx, tx, &st) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.settings.update", "benefit_settings", a.TenantID.String(), nil)
	return &st, nil
}

// --- windows (BN-C04) ---

// WindowInput is an open-enrolment window body.
type WindowInput struct {
	Name     string `json:"name"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
}

// CreateWindow opens a tenant-wide enrolment window.
func (s *Service) CreateWindow(ctx context.Context, a Actor, in WindowInput) (*Window, error) {
	name, err := required("name", in.Name, 100)
	if err != nil {
		return nil, err
	}
	from, err := parseDate("starts_on", in.StartsOn)
	if err != nil {
		return nil, err
	}
	to, err := parseDate("ends_on", in.EndsOn)
	if err != nil {
		return nil, err
	}
	if to.Before(from.Time) || to.Sub(from.Time) > maxWindowSpanDays*24*time.Hour {
		return nil, invalid("ends_on", "must be on or after starts_on and within 120 days")
	}
	w := &Window{TenantID: a.TenantID, Name: name, StartsOn: from, EndsOn: to, Status: WindowActive, CreatedByUserID: a.UserID, CreatedAt: s.now().UTC()}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, w) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.window.create", "benefit_window", w.ID.String(), nil)
	return w, nil
}

// CancelWindow cancels a window; it is idempotent.
func (s *Service) CancelWindow(ctx context.Context, a Actor, id uuid.UUID) (*Window, error) {
	var out *Window
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		w, err := s.store.Window(ctx, tx, a.TenantID, id, true)
		if err != nil || w == nil {
			return orNotFound(err)
		}
		out = w
		if w.Status == WindowCancelled {
			return nil
		}
		w.Status = WindowCancelled
		return s.store.Save(ctx, tx, w)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.window.cancel", "benefit_window", id.String(), nil)
	return out, nil
}

// Windows lists the tenant's open-enrolment windows, newest first.
func (s *Service) Windows(ctx context.Context, a Actor) ([]Window, error) {
	return s.store.Windows(ctx, s.store.DB(), a.TenantID)
}

// --- plans (BN-C01..BN-C03) ---

// PlanInput creates (code, name, category, plan_year, effective_from and levels required) or patches a plan.
type PlanInput struct {
	Code          *string      `json:"code"`
	Name          *string      `json:"name"`
	Category      *string      `json:"category"`
	Provider      *string      `json:"provider"`
	PlanYear      *int         `json:"plan_year"`
	Eligibility   *Eligibility `json:"eligibility"`
	EffectiveFrom *string      `json:"effective_from"`
	Levels        []Level      `json:"levels"`
}

// VersionInput adds a cost version.
type VersionInput struct {
	EffectiveFrom string  `json:"effective_from"`
	Levels        []Level `json:"levels"`
}

var categories = map[string]bool{"health": true, "life": true, "accident": true, "retirement": true, "wellness": true, "other": true}

func validateEligibility(e *Eligibility) error {
	for field, list := range map[string][]string{"worker_types": e.WorkerTypes, "employment_types": e.EmploymentTypes} {
		if len(list) > 10 {
			return invalid("eligibility."+field, "at most 10")
		}
		for i, v := range list {
			v = strings.ToLower(strings.TrimSpace(v))
			if v == "" || utf8.RuneCountInString(v) > 30 {
				return invalid("eligibility."+field, "each entry is 1 to 30 characters")
			}
			list[i] = v
		}
	}
	if len(e.GradeIDs) > 50 || len(e.LocationIDs) > 50 {
		return invalid("eligibility", "at most 50 grades or locations")
	}
	if e.MinTenureDays < 0 || e.MinTenureDays > 3650 {
		return invalid("eligibility.min_tenure_days", "must be between 0 and 3650")
	}
	return nil
}

func validateLevels(levels []Level) ([]Level, error) {
	if len(levels) < 1 || len(levels) > 10 {
		return nil, invalid("levels", "1 to 10 coverage levels")
	}
	seen := map[string]bool{}
	out := make([]Level, 0, len(levels))
	for _, l := range levels {
		l.Code = strings.TrimSpace(l.Code)
		if !levelPattern.MatchString(l.Code) {
			return nil, invalid("levels.code", "1 to 30 letters, digits, _ or -")
		}
		key := strings.ToLower(l.Code)
		if seen[key] {
			return nil, invalid("levels.code", "a level code appears twice")
		}
		seen[key] = true
		name, err := required("levels.name", l.Name, 100)
		if err != nil {
			return nil, err
		}
		l.Name = name
		if l.EmployeeCost < 0 || l.EmployeeCost > maxMonthlyCost || l.EmployerCost < 0 || l.EmployerCost > maxMonthlyCost {
			return nil, invalid("levels", "monthly costs must be between 0 and 1,000,000.00")
		}
		if l.MaxDependants < 0 || l.MaxDependants > 10 {
			return nil, invalid("levels.max_dependants", "must be between 0 and 10")
		}
		out = append(out, l)
	}
	return out, nil
}

func (s *Service) withVersions(ctx context.Context, db *gorm.DB, p *Plan) (*Plan, error) {
	vs, err := s.store.Versions(ctx, db, p.TenantID, []uuid.UUID{p.ID})
	p.Versions = vs
	return p, err
}

// CreatePlan is BN-C01: a draft plan with its first cost version.
func (s *Service) CreatePlan(ctx context.Context, a Actor, in PlanInput) (*Plan, error) {
	if in.Code == nil || in.Name == nil || in.Category == nil || in.PlanYear == nil || in.EffectiveFrom == nil {
		return nil, invalid("code", "code, name, category, plan_year, effective_from and levels are required")
	}
	code := strings.TrimSpace(*in.Code)
	if !codePattern.MatchString(code) {
		return nil, invalid("code", "2 to 30 letters, digits, _ or -")
	}
	name, err := required("name", *in.Name, 150)
	if err != nil {
		return nil, err
	}
	if !categories[*in.Category] {
		return nil, invalid("category", "must be health, life, accident, retirement, wellness or other")
	}
	if *in.PlanYear < 2000 || *in.PlanYear > 2100 {
		return nil, invalid("plan_year", "must be between 2000 and 2100")
	}
	provider, err := optional("provider", in.Provider, 100)
	if err != nil {
		return nil, err
	}
	var elig Eligibility
	if in.Eligibility != nil {
		elig = *in.Eligibility
		if err := validateEligibility(&elig); err != nil {
			return nil, err
		}
	}
	from, err := parseDate("effective_from", *in.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	levels, err := validateLevels(in.Levels)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	p := &Plan{ID: uuid.New(), TenantID: a.TenantID, Code: strings.ToUpper(code), Name: name, Category: *in.Category, Provider: provider, PlanYear: *in.PlanYear,
		Eligibility: elig, Status: PlanDraft, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Lock(ctx, tx, "plan-code:"+p.Code); err != nil {
			return err
		}
		taken, err := s.store.PlanCodeTaken(ctx, tx, a.TenantID, p.Code)
		if err != nil {
			return err
		}
		if taken {
			return ErrConflict
		}
		if err := s.store.Create(ctx, tx, p); err != nil {
			return err
		}
		return s.store.Create(ctx, tx, &PlanVersion{TenantID: a.TenantID, PlanID: p.ID, Version: 1, EffectiveFrom: from, Levels: levels, CreatedByUserID: a.UserID, CreatedAt: now})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.plan.create", "benefit_plan", p.ID.String(), nil)
	return s.withVersions(ctx, s.store.DB(), p)
}

// UpdatePlan patches name, provider and eligibility; costs change only through AddVersion (BN-C02).
func (s *Service) UpdatePlan(ctx context.Context, a Actor, id uuid.UUID, in PlanInput) (*Plan, error) {
	if in.Code != nil || in.Category != nil || in.PlanYear != nil || in.EffectiveFrom != nil || in.Levels != nil {
		return nil, invalid("levels", "code, category, plan year and costs cannot be edited; add a version for new costs")
	}
	var out *Plan
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		p, err := s.store.Plan(ctx, tx, a.TenantID, id, true)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		if p.Status == PlanRetired {
			return ErrPlanState
		}
		if in.Name != nil {
			if p.Name, err = required("name", *in.Name, 150); err != nil {
				return err
			}
		}
		if in.Provider != nil {
			if p.Provider, err = optional("provider", in.Provider, 100); err != nil {
				return err
			}
		}
		if in.Eligibility != nil {
			e := *in.Eligibility
			if err := validateEligibility(&e); err != nil {
				return err
			}
			p.Eligibility = e
		}
		p.UpdatedAt = s.now().UTC()
		out = p
		return s.store.Save(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.plan.update", "benefit_plan", id.String(), nil)
	return s.withVersions(ctx, s.store.DB(), out)
}

// AddVersion is BN-C02: new costs from a future date; the previous version ends where this one starts.
func (s *Service) AddVersion(ctx context.Context, a Actor, id uuid.UUID, in VersionInput) (*Plan, error) {
	from, err := parseDate("effective_from", in.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	levels, err := validateLevels(in.Levels)
	if err != nil {
		return nil, err
	}
	if from.Before(s.today().Time) {
		return nil, invalid("effective_from", "must not be in the past")
	}
	var out *Plan
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		p, err := s.store.Plan(ctx, tx, a.TenantID, id, true)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		if p.Status == PlanRetired {
			return ErrPlanState
		}
		vs, err := s.store.Versions(ctx, tx, a.TenantID, []uuid.UUID{id})
		if err != nil {
			return err
		}
		last := vs[len(vs)-1]
		if !from.After(last.EffectiveFrom.Time) {
			return invalid("effective_from", "must be after the current version starts on "+last.EffectiveFrom.Format(DateLayout))
		}
		last.EffectiveTo = &from
		if err := s.store.Save(ctx, tx, &last); err != nil {
			return err
		}
		out = p
		return s.store.Create(ctx, tx, &PlanVersion{TenantID: a.TenantID, PlanID: id, Version: last.Version + 1, EffectiveFrom: from, Levels: levels,
			CreatedByUserID: a.UserID, CreatedAt: s.now().UTC()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.plan.version", "benefit_plan", id.String(), nil)
	return s.withVersions(ctx, s.store.DB(), out)
}

func (s *Service) movePlan(ctx context.Context, a Actor, id uuid.UUID, to string, from ...string) (*Plan, error) {
	var out *Plan
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		p, err := s.store.Plan(ctx, tx, a.TenantID, id, true)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		ok := false
		for _, f := range from {
			ok = ok || p.Status == f
		}
		if !ok {
			return ErrPlanState
		}
		p.Status, p.UpdatedAt = to, s.now().UTC()
		out = p
		if err := s.store.Save(ctx, tx, p); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "benefits.plan."+to, map[string]string{"plan_id": id.String(), "status": to})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.plan."+to, "benefit_plan", id.String(), nil)
	return s.withVersions(ctx, s.store.DB(), out)
}

// ActivatePlan moves draft → active: workers can now elect it.
func (s *Service) ActivatePlan(ctx context.Context, a Actor, id uuid.UUID) (*Plan, error) {
	return s.movePlan(ctx, a, id, PlanActive, PlanDraft)
}

// RetirePlan moves draft | active → retired: no new elections; existing ones continue until ended.
func (s *Service) RetirePlan(ctx context.Context, a Actor, id uuid.UUID) (*Plan, error) {
	return s.movePlan(ctx, a, id, PlanRetired, PlanDraft, PlanActive)
}

// GetPlan returns a plan with its versions.
func (s *Service) GetPlan(ctx context.Context, a Actor, id uuid.UUID) (*Plan, error) {
	p, err := s.store.Plan(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	return s.withVersions(ctx, s.store.DB(), p)
}

// Plans lists plans.
func (s *Service) Plans(ctx context.Context, a Actor, status string, page, perPage int) ([]Plan, int64, error) {
	switch status {
	case "", PlanDraft, PlanActive, PlanRetired:
	default:
		return nil, 0, invalid("status", "must be draft, active or retired")
	}
	return s.store.Plans(ctx, s.store.DB(), a.TenantID, status, page, perPage)
}
