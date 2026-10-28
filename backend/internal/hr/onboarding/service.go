package onboarding

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeAPI "github.com/jaas/jaas/internal/hr/employee/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/identityport"
	recruitmentAPI "github.com/jaas/jaas/internal/hr/recruitment/api"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

const maxTemplateTasks = 50

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
	ErrOpenExists       = appErr(http.StatusConflict, "CONFLICT", "the worker already has an open checklist of this kind")
	ErrChecklistState   = appErr(http.StatusConflict, "CHECKLIST_STATE", "the checklist is not open")
	ErrTaskState        = appErr(http.StatusConflict, "TASK_STATE", "the task's status does not allow this")
	ErrTaskBlocking     = appErr(http.StatusConflict, "TASK_BLOCKING", "a blocking task cannot be skipped")
	ErrDocumentRequired = appErr(http.StatusConflict, "DOCUMENT_REQUIRED", "the worker has no verified document of the required type")
	ErrBlockingOpen     = appErr(http.StatusConflict, "BLOCKING_OPEN", "blocking tasks are still open")
	ErrClearancePending = appErr(http.StatusConflict, "CLEARANCE_PENDING", "exit clearance is not complete")
	ErrAlreadyCleared   = appErr(http.StatusConflict, "ALREADY_CLEARED", "this area is already cleared")
	ErrAlreadySubmitted = appErr(http.StatusConflict, "ALREADY_SUBMITTED", "the exit interview was already submitted")
	ErrNotOffboarding   = appErr(http.StatusConflict, "CHECKLIST_STATE", "only an open offboarding checklist has clearance and an exit interview")
)

// Directory is onboarding's view of HR/Employee.
type Directory interface {
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// Ports are the collaborators onboarding consumes.
type Ports struct {
	Employees Directory
	Core      coreAPI.Directory
	Documents employeeAPI.Documents
	Audit     identityport.Auditor
}

// Actor is the caller; the flags are onboarding:read, onboarding:manage and onboarding:exit_interview.
type Actor struct {
	TenantID, UserID              uuid.UUID
	IP, UserAgent                 string
	CanRead, CanManage, CanReadEI bool
}

// Service implements onboarding (OB-01..OB-10).
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

func text(field, v string, max int) (string, error) {
	t := strings.TrimSpace(v)
	if t == "" || utf8.RuneCountInString(t) > max {
		return "", invalid(field, "must be 1–"+strconv.Itoa(max)+" characters")
	}
	return t, nil
}

func optional(field string, v *string, max int) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := text(field, *v, max)
	return &t, err
}

func optionalUUID(field string, v *string) (*uuid.UUID, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*v)
	if err != nil {
		return nil, invalid(field, "must be a UUID")
	}
	return &id, nil
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
func (s *Service) self(ctx context.Context, a Actor) (*uuid.UUID, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &me.ID, nil
}

// --- templates (OB-01) ---

// TemplateInput creates or edits a template; an empty string clears a criterion.
type TemplateInput struct {
	Kind         *string `json:"kind"`
	Name         *string `json:"name"`
	DepartmentID *string `json:"department_id"`
	JobProfileID *string `json:"job_profile_id"`
	LocationID   *string `json:"location_id"`
	WorkerType   *string `json:"worker_type"`
	Status       *string `json:"status"`
}

func (in TemplateInput) apply(t *Template) error {
	if in.Name != nil {
		n, err := text("name", *in.Name, 150)
		if err != nil {
			return err
		}
		t.Name = n
	}
	for _, c := range []struct {
		field string
		raw   *string
		dst   **uuid.UUID
	}{{"department_id", in.DepartmentID, &t.DepartmentID}, {"job_profile_id", in.JobProfileID, &t.JobProfileID}, {"location_id", in.LocationID, &t.LocationID}} {
		if c.raw == nil {
			continue
		}
		id, err := optionalUUID(c.field, c.raw)
		if err != nil {
			return err
		}
		*c.dst = id
	}
	if in.WorkerType != nil {
		switch *in.WorkerType {
		case "":
			t.WorkerType = nil
		case "employee", "contractor", "intern":
			wt := *in.WorkerType
			t.WorkerType = &wt
		default:
			return invalid("worker_type", "must be employee, contractor or intern")
		}
	}
	if in.Status != nil {
		if *in.Status != TemplateActive && *in.Status != TemplateArchived {
			return invalid("status", "must be active or archived")
		}
		t.Status = *in.Status
	}
	return nil
}

// CreateTemplate is OB-01.
func (s *Service) CreateTemplate(ctx context.Context, a Actor, in TemplateInput) (*Template, error) {
	if in.Kind == nil || !kinds[*in.Kind] {
		return nil, invalid("kind", "must be onboarding or offboarding")
	}
	if in.Name == nil {
		return nil, invalid("name", "is required")
	}
	now := s.now().UTC()
	t := &Template{TenantID: a.TenantID, Kind: *in.Kind, Status: TemplateActive, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := in.apply(t); err != nil {
		return nil, err
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, t) }); err != nil {
		return nil, err
	}
	t.Tasks = []TemplateTask{}
	s.log(ctx, a, "onboarding.template_created", "onb_template", t.ID.String(), map[string]string{"kind": t.Kind})
	return t, nil
}

// UpdateTemplate edits a template (the kind never changes).
func (s *Service) UpdateTemplate(ctx context.Context, a Actor, id uuid.UUID, in TemplateInput) (*Template, error) {
	if in.Kind != nil {
		return nil, invalid("kind", "cannot change")
	}
	var t *Template
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if t, err = s.store.Template(ctx, tx, a.TenantID, id, true); err != nil || t == nil {
			return orNotFound(err)
		}
		if err := in.apply(t); err != nil {
			return err
		}
		t.UpdatedAt = s.now().UTC()
		return s.store.Save(ctx, tx, t)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.template_updated", "onb_template", id.String(), nil)
	return s.GetTemplate(ctx, a, id)
}

// GetTemplate returns a template with its tasks.
func (s *Service) GetTemplate(ctx context.Context, a Actor, id uuid.UUID) (*Template, error) {
	t, err := s.store.Template(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || t == nil {
		return nil, orNotFound(err)
	}
	tasks, err := s.store.TemplateTasks(ctx, s.store.DB(), a.TenantID, []uuid.UUID{id})
	t.Tasks = append([]TemplateTask{}, tasks...)
	return t, err
}

// Templates lists templates.
func (s *Service) Templates(ctx context.Context, a Actor, kind, status string) ([]Template, error) {
	if kind != "" && !kinds[kind] {
		return nil, invalid("kind", "must be onboarding or offboarding")
	}
	if status != "" && status != TemplateActive && status != TemplateArchived {
		return nil, invalid("status", "must be active or archived")
	}
	rows, err := s.store.Templates(ctx, s.store.DB(), a.TenantID, kind, status)
	if rows == nil {
		rows = []Template{}
	}
	return rows, err
}

// TaskInput is one template task.
type TaskInput struct {
	Title         string  `json:"title"`
	Description   *string `json:"description"`
	OwnerRole     string  `json:"owner_role"`
	DueOffsetDays int     `json:"due_offset_days"`
	DocumentType  *string `json:"document_type"`
	Blocking      bool    `json:"blocking"`
}

// SetTasks replaces a template's tasks (OB-01).
func (s *Service) SetTasks(ctx context.Context, a Actor, id uuid.UUID, in []TaskInput) (*Template, error) {
	if len(in) == 0 || len(in) > maxTemplateTasks {
		return nil, invalid("tasks", "must hold 1–50 tasks")
	}
	rows := make([]TemplateTask, 0, len(in))
	for i, t := range in {
		title, err := text("title", t.Title, 200)
		if err != nil {
			return nil, err
		}
		desc, err := optional("description", t.Description, 2000)
		if err != nil {
			return nil, err
		}
		doc, err := optional("document_type", t.DocumentType, 50)
		if err != nil {
			return nil, err
		}
		if !ownerRoles[t.OwnerRole] {
			return nil, invalid("owner_role", "must be employee, manager, hr, it or finance")
		}
		if t.DueOffsetDays < -90 || t.DueOffsetDays > 365 {
			return nil, invalid("due_offset_days", "must be between -90 and 365")
		}
		rows = append(rows, TemplateTask{ID: uuid.New(), TenantID: a.TenantID, TemplateID: id, Position: i + 1, Title: title, Description: desc,
			OwnerRole: t.OwnerRole, DueOffsetDays: t.DueOffsetDays, DocumentType: doc, Blocking: t.Blocking})
	}
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		t, err := s.store.Template(ctx, tx, a.TenantID, id, true)
		if err != nil || t == nil {
			return orNotFound(err)
		}
		return s.store.ReplaceTemplateTasks(ctx, tx, a.TenantID, id, rows)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.template_tasks_set", "onb_template", id.String(), nil)
	return s.GetTemplate(ctx, a, id)
}

// --- starting checklists (OB-02, OB-03, OB-04) ---

// startSpec describes one checklist to start.
type startSpec struct {
	employeeID uuid.UUID
	kind       string
	source     string
	sourceID   *uuid.UUID
	anchor     time.Time
	placement  *coreAPI.Placement
	actor      *uuid.UUID
}

// matches reports whether a template's criteria all match the placement (OB-02).
func matches(t *Template, p *coreAPI.Placement) bool {
	if t.specificity() == 0 {
		return true
	}
	if p == nil {
		return false
	}
	eq := func(want, got *uuid.UUID) bool { return want == nil || (got != nil && *want == *got) }
	return eq(t.DepartmentID, p.DepartmentID) && eq(t.JobProfileID, p.JobProfileID) && eq(t.LocationID, p.LocationID) &&
		(t.WorkerType == nil || *t.WorkerType == p.WorkerType)
}

// start creates the checklist unless the source already has one or the worker has an open one of the kind; it returns
// nil, nil when nothing was created (idempotent, OB-03).
func (s *Service) start(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, sp startSpec) (*Checklist, error) {
	if err := s.store.Lock(ctx, tx, "worker:"+sp.employeeID.String()+":"+sp.kind); err != nil {
		return nil, err
	}
	if sp.sourceID != nil {
		if c, err := s.store.ChecklistBySource(ctx, tx, tenantID, sp.kind, *sp.sourceID); err != nil || c != nil {
			return nil, err
		}
	}
	if c, err := s.store.OpenChecklist(ctx, tx, tenantID, sp.employeeID, sp.kind); err != nil || c != nil {
		return nil, err
	}
	templates, err := s.store.Templates(ctx, tx, tenantID, sp.kind, TemplateActive)
	if err != nil {
		return nil, err
	}
	best, chosen := -1, []uuid.UUID{}
	for i := range templates {
		t := &templates[i]
		if !matches(t, sp.placement) {
			continue
		}
		switch n := t.specificity(); {
		case n > best:
			best, chosen = n, []uuid.UUID{t.ID}
		case n == best:
			chosen = append(chosen, t.ID)
		}
	}
	tasks, err := s.store.TemplateTasks(ctx, tx, tenantID, chosen)
	if err != nil {
		return nil, err
	}
	order := map[uuid.UUID]int{}
	for i, id := range chosen {
		order[id] = i
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		if order[tasks[i].TemplateID] != order[tasks[j].TemplateID] {
			return order[tasks[i].TemplateID] < order[tasks[j].TemplateID]
		}
		return tasks[i].Position < tasks[j].Position
	})
	now := s.now().UTC()
	c := &Checklist{TenantID: tenantID, EmployeeProfileID: sp.employeeID, Kind: sp.kind, Source: sp.source, SourceID: sp.sourceID,
		AnchorDate: Date{day(sp.anchor)}, Status: ChecklistOpen, CreatedByUserID: sp.actor, CreatedAt: now, UpdatedAt: now}
	if sp.placement != nil {
		c.DepartmentID, c.ManagerEmployeeID = sp.placement.DepartmentID, sp.placement.ManagerEmployeeID
	}
	if err := s.store.Create(ctx, tx, c); err != nil {
		return nil, err
	}
	for i, tt := range tasks {
		t := &Task{TenantID: tenantID, ChecklistID: c.ID, Position: i + 1, Title: tt.Title, Description: tt.Description, OwnerRole: tt.OwnerRole,
			DueDate: Date{c.AnchorDate.AddDate(0, 0, tt.DueOffsetDays)}, DocumentType: tt.DocumentType, Blocking: tt.Blocking, Status: TaskOpen, UpdatedAt: now}
		switch tt.OwnerRole {
		case RoleEmployee:
			emp := sp.employeeID
			t.AssigneeEmployeeID = &emp
		case RoleManager:
			t.AssigneeEmployeeID = c.ManagerEmployeeID
		}
		if err := s.store.Create(ctx, tx, t); err != nil {
			return nil, err
		}
	}
	if sp.kind == KindOffboarding {
		for _, area := range clearanceSet {
			if err := s.store.Create(ctx, tx, &Clearance{TenantID: tenantID, ChecklistID: c.ID, Area: area}); err != nil {
				return nil, err
			}
		}
	}
	return c, s.store.Event(ctx, tx, tenantID, "hr.onboarding.checklist_started", map[string]string{"checklist_id": c.ID.String(),
		"employee_id": sp.employeeID.String(), "kind": sp.kind, "anchor_date": c.AnchorDate.Format(DateLayout)})
}

// CoreHook is the core/api.ActionHook (OB-03): hires start onboarding, terminations offboarding; a rescinded action
// cancels the checklist it started.
func (s *Service) CoreHook(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, f coreAPI.ActionFact) error {
	kind, anchor := "", f.EffectiveDate
	switch f.Type {
	case "hire", "rehire":
		kind = KindOnboarding
	case "terminate":
		kind = KindOffboarding
		if f.LastWorkingDay != nil {
			anchor = *f.LastWorkingDay
		}
	default:
		return nil
	}
	if f.Rescinded {
		c, err := s.store.ChecklistBySource(ctx, tx, tenantID, kind, f.ActionID)
		if err != nil || c == nil || c.Status != ChecklistOpen {
			return err
		}
		now, reason := s.now().UTC(), "HR action rescinded"
		c.Status, c.CloseReason, c.UpdatedAt = ChecklistCancelled, &reason, now
		return s.store.Save(ctx, tx, c)
	}
	id := f.ActionID
	_, err := s.start(ctx, tx, tenantID, startSpec{employeeID: f.EmployeeID, kind: kind, source: SourceCoreAction, sourceID: &id, anchor: anchor,
		placement: f.Placement})
	return err
}

// RecruitmentHook is the recruitment/api.HireHook (OB-03): a hire starts onboarding from the joining date.
func (s *Service) RecruitmentHook(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, f recruitmentAPI.HireFact) error {
	id := f.CandidateID
	_, err := s.start(ctx, tx, tenantID, startSpec{employeeID: f.EmployeeID, kind: KindOnboarding, source: SourceRecruitment, sourceID: &id,
		anchor: f.JoiningDate})
	return err
}

// StartInput is a manual start.
type StartInput struct {
	EmployeeID string `json:"employee_id"`
	Kind       string `json:"kind"`
	AnchorDate string `json:"anchor_date"`
}

// Start is a manual start by onboarding:manage (OB-03): an open checklist of the kind is a conflict.
func (s *Service) Start(ctx context.Context, a Actor, in StartInput) (*ChecklistView, error) {
	emp, err := uuid.Parse(in.EmployeeID)
	if err != nil {
		return nil, invalid("employee_id", "must be a UUID")
	}
	if !kinds[in.Kind] {
		return nil, invalid("kind", "must be onboarding or offboarding")
	}
	anchor, err := parseDate("anchor_date", in.AnchorDate)
	if err != nil {
		return nil, err
	}
	e, err := s.p.Employees.GetByID(ctx, a.TenantID, emp)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && e == nil) {
		return nil, ErrEmployeeNotFound
	}
	if err != nil {
		return nil, err
	}
	pl, err := s.p.Core.PlacementOn(ctx, a.TenantID, emp, anchor.Time)
	if err != nil {
		return nil, err
	}
	var c *Checklist
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if c, err = s.start(ctx, tx, a.TenantID, startSpec{employeeID: emp, kind: in.Kind, source: SourceManual, anchor: anchor.Time, placement: pl,
			actor: &a.UserID}); err != nil {
			return err
		}
		if c == nil {
			return ErrOpenExists
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.checklist_started", "onb_checklist", c.ID.String(), map[string]string{"kind": c.Kind})
	return s.view(ctx, s.store.DB(), a.TenantID, c)
}
