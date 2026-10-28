package learning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/identityport"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// DefaultZone is the time zone of a tenant that has not set one (LN-C09).
const DefaultZone = "Asia/Kolkata"

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Errors (specification.md §4).
var (
	ErrNotFound          = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound  = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrForbidden         = appErr(http.StatusForbidden, "FORBIDDEN", "you may not do this")
	ErrCourseState       = appErr(http.StatusConflict, "COURSE_STATE", "the course's status does not allow this")
	ErrPlanState         = appErr(http.StatusConflict, "PLAN_STATE", "the plan is archived")
	ErrSelfEnrolOff      = appErr(http.StatusConflict, "SELF_ENROL_OFF", "the course does not allow self-enrolment")
	ErrAlreadyEnrolled   = appErr(http.StatusConflict, "ALREADY_ENROLLED", "you already have a live enrolment in this course")
	ErrEnrolmentState    = appErr(http.StatusConflict, "ENROLMENT_STATE", "the enrolment's status does not allow this")
	ErrCompletionPending = appErr(http.StatusConflict, "COMPLETION_PENDING", "a completion is already waiting for verification")
	ErrCompletionState   = appErr(http.StatusConflict, "COMPLETION_STATE", "the completion has already been decided")
	ErrSelfVerify        = appErr(http.StatusForbidden, "LEARNING_SELF_VERIFY", "you cannot verify your own completion")
)

// Directory is learning's view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, filter employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error)
}

// Reports resolves a manager's direct reports (HR/Organization mappings).
type Reports interface {
	DirectReportUserIDs(ctx context.Context, tenantID, managerUserID uuid.UUID) ([]uuid.UUID, error)
}

// Ports are the collaborators learning consumes.
type Ports struct {
	Employees Directory
	Core      coreAPI.Directory
	Reports   Reports
	Audit     identityport.Auditor
}

// Actor is the caller; CanRead / CanManage are learning:read / learning:manage.
type Actor struct {
	TenantID, UserID   uuid.UUID
	IP, UserAgent      string
	CanRead, CanManage bool
}

// Service implements learning (LN-C01..LN-C18).
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

func parseID(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, invalid(field, "must be a UUID")
	}
	return id, nil
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

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// zone returns the tenant's learning time zone name (LN-C09).
func (s *Service) zone(ctx context.Context, tenantID uuid.UUID) (string, error) {
	st, err := s.store.Settings(ctx, s.store.DB(), tenantID)
	if err != nil {
		return "", err
	}
	if st == nil {
		return DefaultZone, nil
	}
	return st.TimeZone, nil
}

// today is the tenant's calendar date now.
func (s *Service) today(ctx context.Context, tenantID uuid.UUID) (Date, error) {
	name, err := s.zone(ctx, tenantID)
	if err != nil {
		return Date{}, err
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc = time.UTC
	}
	return Date{midnight(s.now().In(loc))}, nil
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

// --- settings (LN-C09) ---

// SettingsInput is the settings body.
type SettingsInput struct {
	TimeZone string `json:"time_zone"`
}

// GetSettings returns the tenant's settings or the defaults.
func (s *Service) GetSettings(ctx context.Context, a Actor) (*Settings, error) {
	st, err := s.store.Settings(ctx, s.store.DB(), a.TenantID)
	if err != nil || st != nil {
		return st, err
	}
	return &Settings{TenantID: a.TenantID, TimeZone: DefaultZone}, nil
}

// PutSettings sets the tenant's IANA time zone.
func (s *Service) PutSettings(ctx context.Context, a Actor, in SettingsInput) (*Settings, error) {
	name, err := required("time_zone", in.TimeZone, 64)
	if err != nil {
		return nil, err
	}
	if _, lerr := time.LoadLocation(name); lerr != nil || strings.EqualFold(name, "local") {
		return nil, invalid("time_zone", "must be an IANA time zone name such as Asia/Kolkata")
	}
	row := &Settings{TenantID: a.TenantID, TimeZone: name, UpdatedByUserID: a.UserID, UpdatedAt: s.now().UTC()}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SaveSettings(ctx, tx, row) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.settings.update", "learning_settings", a.TenantID.String(), map[string]string{"time_zone": name})
	return row, nil
}

// --- courses (LN-C01..LN-C03) ---

var opaqueRef = regexp.MustCompile(`^[A-Za-z0-9._~/@-]+$`)

// httpsURL accepts only absolute https URLs with a host and no user-info.
func httpsURL(field, raw string, max int) (string, error) {
	t := strings.TrimSpace(raw)
	if t == "" || utf8.RuneCountInString(t) > max {
		return "", invalid(field, "is required and at most "+itoa(max)+" characters")
	}
	for _, r := range t {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", invalid(field, "must not contain spaces or control characters")
		}
	}
	u, err := url.Parse(t)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", invalid(field, "must be an https URL")
	}
	return t, nil
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// reference accepts an https URL or an opaque reference (no scheme, no colon).
func reference(field, raw string, max int, allowURL bool) (string, error) {
	t := strings.TrimSpace(raw)
	if strings.Contains(t, ":") {
		if !allowURL {
			return "", invalid(field, "must be an opaque reference, not a URL")
		}
		return httpsURL(field, t, max)
	}
	if t == "" || utf8.RuneCountInString(t) > max || !opaqueRef.MatchString(t) {
		return "", invalid(field, "must be an https URL or a reference of letters, digits and . _ ~ / @ -")
	}
	return t, nil
}

// CourseInput creates (title and content_kind required) or patches a course.
type CourseInput struct {
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	Provider     *string   `json:"provider"`
	ContentKind  *string   `json:"content_kind"`
	ContentURL   *string   `json:"content_url"`
	DocumentRef  *string   `json:"document_ref"`
	ThumbnailRef *string   `json:"thumbnail_ref"`
	DurationMins *int      `json:"duration_mins"`
	Tags         *[]string `json:"tags"`
	Verification *string   `json:"verification"`
	SelfEnrol    *bool     `json:"self_enrol"`
}

// applyCourse copies the given fields onto c and validates the whole course.
func applyCourse(c *Course, in CourseInput) error {
	if in.Title != nil {
		c.Title = *in.Title
	}
	if in.Description != nil {
		c.Description = in.Description
	}
	if in.Provider != nil {
		c.Provider = in.Provider
	}
	if in.ContentKind != nil {
		c.ContentKind = *in.ContentKind
	}
	if in.ContentURL != nil {
		c.ContentURL = in.ContentURL
	}
	if in.DocumentRef != nil {
		c.DocumentRef = in.DocumentRef
	}
	if in.ThumbnailRef != nil {
		c.ThumbnailRef = in.ThumbnailRef
	}
	if in.DurationMins != nil {
		c.DurationMins = *in.DurationMins
	}
	if in.Tags != nil {
		c.Tags = Tags(*in.Tags)
	}
	if in.Verification != nil {
		c.Verification = *in.Verification
	}
	if in.SelfEnrol != nil {
		c.SelfEnrol = *in.SelfEnrol
	}
	return validateCourse(c)
}

func validateCourse(c *Course) error {
	var err error
	if c.Title, err = required("title", c.Title, 150); err != nil {
		return err
	}
	if c.Description, err = optional("description", c.Description, 2000); err != nil {
		return err
	}
	if c.Provider, err = optional("provider", c.Provider, 100); err != nil {
		return err
	}
	switch c.ContentKind {
	case KindLink:
		c.DocumentRef = nil
		if c.ContentURL == nil {
			return invalid("content_url", "is required for a link")
		}
		u, err := httpsURL("content_url", *c.ContentURL, 500)
		if err != nil {
			return err
		}
		c.ContentURL = &u
	case KindDocument:
		c.ContentURL = nil
		if c.DocumentRef == nil {
			return invalid("document_ref", "is required for a document")
		}
		r, err := reference("document_ref", *c.DocumentRef, 200, false)
		if err != nil {
			return err
		}
		c.DocumentRef = &r
	default:
		return invalid("content_kind", "must be link or document")
	}
	if c.ThumbnailRef != nil {
		if strings.TrimSpace(*c.ThumbnailRef) == "" {
			c.ThumbnailRef = nil
		} else {
			r, err := reference("thumbnail_ref", *c.ThumbnailRef, 500, true)
			if err != nil {
				return err
			}
			c.ThumbnailRef = &r
		}
	}
	if c.DurationMins < 0 || c.DurationMins > 10000 {
		return invalid("duration_mins", "must be between 0 and 10000")
	}
	if len(c.Tags) > 10 {
		return invalid("tags", "at most 10")
	}
	seen := map[string]bool{}
	tags := Tags{}
	for _, t := range c.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || utf8.RuneCountInString(t) > 30 {
			return invalid("tags", "each tag is 1 to 30 characters")
		}
		if !seen[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	c.Tags = tags
	if c.Verification == "" {
		c.Verification = VerifySelf
	}
	if c.Verification != VerifySelf && c.Verification != VerifyManager && c.Verification != VerifyHR {
		return invalid("verification", "must be self, manager or hr")
	}
	return nil
}

// contentSnapshot is the versioned part of a course (LN-C02).
func contentSnapshot(c *Course) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{"title": c.Title, "description": c.Description, "provider": c.Provider, "content_kind": c.ContentKind,
		"content_url": c.ContentURL, "document_ref": c.DocumentRef, "duration_mins": c.DurationMins, "tags": c.Tags})
	return b
}

func (s *Service) addVersion(ctx context.Context, tx *gorm.DB, a Actor, c *Course) error {
	return s.store.Create(ctx, tx, &CourseVersion{TenantID: a.TenantID, CourseID: c.ID, Version: c.Version, Snapshot: contentSnapshot(c),
		ChangedByUserID: a.UserID, CreatedAt: s.now().UTC()})
}

// CreateCourse is LN-C01: a draft course at version 1.
func (s *Service) CreateCourse(ctx context.Context, a Actor, in CourseInput) (*Course, error) {
	if in.Title == nil || in.ContentKind == nil {
		return nil, invalid("title", "title and content_kind are required")
	}
	c := &Course{TenantID: a.TenantID, Status: CourseDraft, Version: 1, CreatedByUserID: a.UserID, Verification: VerifySelf}
	if err := applyCourse(c, in); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Create(ctx, tx, c); err != nil {
			return err
		}
		return s.addVersion(ctx, tx, a, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.course.create", "learning_course", c.ID.String(), nil)
	return c, nil
}

// UpdateCourse is LN-C02: content changes bump the version.
func (s *Service) UpdateCourse(ctx context.Context, a Actor, id uuid.UUID, in CourseInput) (*Course, error) {
	var out *Course
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.store.Course(ctx, tx, a.TenantID, id, true)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		if c.Status == CourseRetired {
			return ErrCourseState
		}
		before := contentSnapshot(c)
		if err := applyCourse(c, in); err != nil {
			return err
		}
		if string(before) != string(contentSnapshot(c)) {
			c.Version++
			if err := s.addVersion(ctx, tx, a, c); err != nil {
				return err
			}
		}
		c.UpdatedAt = s.now().UTC()
		out = c
		return s.store.Save(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.course.update", "learning_course", id.String(), map[string]string{"version": itoa(out.Version)})
	return out, nil
}

func (s *Service) moveCourse(ctx context.Context, a Actor, id uuid.UUID, to string, from ...string) (*Course, error) {
	var out *Course
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.store.Course(ctx, tx, a.TenantID, id, true)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		ok := false
		for _, f := range from {
			ok = ok || c.Status == f
		}
		if !ok {
			return ErrCourseState
		}
		c.Status, c.UpdatedAt = to, s.now().UTC()
		if err := s.store.Save(ctx, tx, c); err != nil {
			return err
		}
		out = c
		return s.store.Event(ctx, tx, a.TenantID, "learning.course."+to, map[string]string{"course_id": c.ID.String(), "status": to})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.course."+to, "learning_course", id.String(), nil)
	return out, nil
}

// PublishCourse moves draft → published (LN-C03).
func (s *Service) PublishCourse(ctx context.Context, a Actor, id uuid.UUID) (*Course, error) {
	return s.moveCourse(ctx, a, id, CoursePublished, CourseDraft)
}

// RetireCourse moves draft | published → retired; enrolments and completions stay (LN-C03).
func (s *Service) RetireCourse(ctx context.Context, a Actor, id uuid.UUID) (*Course, error) {
	return s.moveCourse(ctx, a, id, CourseRetired, CourseDraft, CoursePublished)
}

// Courses lists the catalogue for managers and readers.
func (s *Service) Courses(ctx context.Context, a Actor, status string, page, perPage int) ([]Course, int64, error) {
	switch status {
	case "", CourseDraft, CoursePublished, CourseRetired:
	default:
		return nil, 0, invalid("status", "must be draft, published or retired")
	}
	return s.store.Courses(ctx, s.store.DB(), a.TenantID, status, page, perPage)
}

// Catalog lists published courses for any learner.
func (s *Service) Catalog(ctx context.Context, a Actor, page, perPage int) ([]Course, int64, error) {
	return s.store.Published(ctx, s.store.DB(), a.TenantID, page, perPage)
}

// GetCourse returns a course.
func (s *Service) GetCourse(ctx context.Context, a Actor, id uuid.UUID) (*Course, error) {
	c, err := s.store.Course(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	return c, nil
}

// Versions lists a course's content snapshots, newest first.
func (s *Service) Versions(ctx context.Context, a Actor, id uuid.UUID) ([]CourseVersion, error) {
	if _, err := s.GetCourse(ctx, a, id); err != nil {
		return nil, err
	}
	return s.store.Versions(ctx, s.store.DB(), a.TenantID, id)
}

// --- plans (LN-C04) ---

// PlanInput creates or patches a plan.
type PlanInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

// ItemInput is one plan item.
type ItemInput struct {
	CourseID  string `json:"course_id"`
	Mandatory *bool  `json:"mandatory"`
}

func (s *Service) planWithItems(ctx context.Context, db *gorm.DB, p *Plan) (*Plan, error) {
	items, err := s.store.PlanItems(ctx, db, p.TenantID, p.ID)
	p.Items = items
	return p, err
}

func (s *Service) applyPlan(ctx context.Context, tx *gorm.DB, p *Plan, in PlanInput) error {
	if in.Name != nil {
		n, err := required("name", *in.Name, 150)
		if err != nil {
			return err
		}
		taken, err := s.store.PlanNameTaken(ctx, tx, p.TenantID, n, p.ID)
		if err != nil {
			return err
		}
		if taken {
			return appErr(http.StatusConflict, "CONFLICT", "a plan with this name exists")
		}
		p.Name = n
	}
	if in.Description != nil {
		d, err := optional("description", in.Description, 1000)
		if err != nil {
			return err
		}
		p.Description = d
	}
	if in.Status != nil {
		if *in.Status != PlanActive && *in.Status != PlanArchived {
			return invalid("status", "must be active or archived")
		}
		p.Status = *in.Status
	}
	return nil
}

// CreatePlan is LN-C04.
func (s *Service) CreatePlan(ctx context.Context, a Actor, in PlanInput) (*Plan, error) {
	if in.Name == nil {
		return nil, invalid("name", "is required")
	}
	now := s.now().UTC()
	p := &Plan{ID: uuid.New(), TenantID: a.TenantID, Status: PlanActive, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.applyPlan(ctx, tx, p, PlanInput{Name: in.Name, Description: in.Description}); err != nil {
			return err
		}
		return s.store.Create(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.plan.create", "learning_plan", p.ID.String(), nil)
	return p, nil
}

// UpdatePlan renames, re-describes or archives a plan.
func (s *Service) UpdatePlan(ctx context.Context, a Actor, id uuid.UUID, in PlanInput) (*Plan, error) {
	var out *Plan
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		p, err := s.store.Plan(ctx, tx, a.TenantID, id, true)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		if err := s.applyPlan(ctx, tx, p, in); err != nil {
			return err
		}
		p.UpdatedAt = s.now().UTC()
		out = p
		return s.store.Save(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.plan.update", "learning_plan", id.String(), nil)
	return s.planWithItems(ctx, s.store.DB(), out)
}

// SetItems replaces a plan's ordered items.
func (s *Service) SetItems(ctx context.Context, a Actor, id uuid.UUID, in []ItemInput) (*Plan, error) {
	if len(in) < 1 || len(in) > 50 {
		return nil, invalid("items", "1 to 50 items")
	}
	rows := make([]PlanItem, 0, len(in))
	ids := make([]uuid.UUID, 0, len(in))
	seen := map[uuid.UUID]bool{}
	for i, it := range in {
		cid, err := parseID("course_id", it.CourseID)
		if err != nil {
			return nil, err
		}
		if seen[cid] {
			return nil, invalid("items", "a course appears twice")
		}
		seen[cid] = true
		ids = append(ids, cid)
		rows = append(rows, PlanItem{TenantID: a.TenantID, PlanID: id, CourseID: cid, Position: i + 1, Mandatory: it.Mandatory == nil || *it.Mandatory})
	}
	var out *Plan
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		p, err := s.store.Plan(ctx, tx, a.TenantID, id, true)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		if p.Status != PlanActive {
			return ErrPlanState
		}
		courses, err := s.store.CoursesByIDs(ctx, tx, a.TenantID, ids)
		if err != nil {
			return err
		}
		if len(courses) != len(ids) {
			return ErrNotFound
		}
		for _, c := range courses {
			if c.Status == CourseRetired {
				return ErrCourseState
			}
		}
		out = p
		p.UpdatedAt = s.now().UTC()
		if err := s.store.Save(ctx, tx, p); err != nil {
			return err
		}
		return s.store.ReplaceItems(ctx, tx, a.TenantID, id, rows)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.plan.items", "learning_plan", id.String(), map[string]string{"items": itoa(len(rows))})
	return s.planWithItems(ctx, s.store.DB(), out)
}

// GetPlan returns a plan with its items.
func (s *Service) GetPlan(ctx context.Context, a Actor, id uuid.UUID) (*Plan, error) {
	p, err := s.store.Plan(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	return s.planWithItems(ctx, s.store.DB(), p)
}

// Plans lists plans.
func (s *Service) Plans(ctx context.Context, a Actor, status string, page, perPage int) ([]Plan, int64, error) {
	if status != "" && status != PlanActive && status != PlanArchived {
		return nil, 0, invalid("status", "must be active or archived")
	}
	return s.store.Plans(ctx, s.store.DB(), a.TenantID, status, page, perPage)
}
