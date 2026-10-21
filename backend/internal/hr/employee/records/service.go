package records

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/identityport"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Limits (FR-ER001..FR-ER003, FR-CR001).
const maxContacts = 5

var (
	accountFormat = regexp.MustCompile(`^[A-Za-z0-9]{6,34}$`)
	routingFormat = regexp.MustCompile(`^[A-Za-z0-9]{4,20}$`)
)

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Errors (specification.md FR-ER, FR-CR).
var (
	ErrNotFound         = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrTooMany          = appErr(http.StatusConflict, "CONFLICT", "at most 5 emergency contacts")
	ErrOverlap          = appErr(http.StatusConflict, "OVERLAP", "the period overlaps another prior employment")
	ErrOpenRequest      = appErr(http.StatusConflict, "CONFLICT", "a request of this type is already waiting for approval")
	ErrRequestState     = appErr(http.StatusConflict, "REQUEST_STATE", "the request is not waiting for a decision")
	ErrSelfApproval     = appErr(http.StatusForbidden, "EMPLOYEE_SELF_APPROVAL", "you cannot decide your own change request")
)

// Actor is the authenticated caller; Sensitive is employee:read_sensitive (unmasks bank numbers, FR-CR005).
type Actor struct {
	TenantID, UserID uuid.UUID
	IP, UserAgent    string
	Sensitive        bool
}

// Service implements FR-ER and FR-CR.
type Service struct {
	store Store
	audit identityport.Auditor
	now   func() time.Time
}

// NewService builds the service.
func NewService(store Store, audit identityport.Auditor, now func() time.Time) *Service {
	return &Service{store: store, audit: audit, now: now}
}

// owner resolves the worker: the caller (employeeID nil) or an employee of the tenant.
func (s *Service) owner(ctx context.Context, a Actor, employeeID *uuid.UUID) (*Worker, error) {
	var w *Worker
	var err error
	if employeeID == nil {
		w, err = s.store.WorkerByUser(ctx, a.TenantID, a.UserID)
	} else {
		w, err = s.store.Worker(ctx, a.TenantID, *employeeID)
	}
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrEmployeeNotFound
	}
	return w, nil
}

func (s *Service) log(ctx context.Context, a Actor, action, id string, meta map[string]string) {
	_ = s.audit.Log(ctx, s.store.DB(), a.TenantID.String(), a.UserID.String(), action, "employee_record", id, meta, a.IP, a.UserAgent)
}

func text(field, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || utf8.RuneCountInString(v) > max {
		return "", invalid(field, "is required and at most "+strconv.Itoa(max)+" characters")
	}
	return v, nil
}

func optional(field string, v *string, max int) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if utf8.RuneCountInString(t) > max {
		return nil, invalid(field, "must be at most "+strconv.Itoa(max)+" characters")
	}
	return &t, nil
}

// --- emergency contacts (FR-ER001) ---

// ContactInput is a contact body.
type ContactInput struct {
	Name         string  `json:"name"`
	Relationship string  `json:"relationship"`
	Phone        string  `json:"phone"`
	Email        *string `json:"email"`
	IsPrimary    bool    `json:"is_primary"`
}

func (in ContactInput) apply(c *EmergencyContact) error {
	var err error
	if c.Name, err = text("name", in.Name, 150); err != nil {
		return err
	}
	if c.Relationship, err = text("relationship", in.Relationship, 50); err != nil {
		return err
	}
	if c.Phone, err = text("phone", in.Phone, 30); err != nil || len(c.Phone) < 5 {
		return invalid("phone", "must be 5–30 characters")
	}
	if c.Email, err = optional("email", in.Email, 255); err != nil {
		return err
	}
	if c.Email != nil && !strings.Contains(*c.Email, "@") {
		return invalid("email", "must be an email address")
	}
	return nil
}

// Contacts lists the worker's emergency contacts (primary first).
func (s *Service) Contacts(ctx context.Context, a Actor, employeeID *uuid.UUID) ([]EmergencyContact, error) {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return nil, err
	}
	return s.store.Contacts(ctx, s.store.DB(), a.TenantID, w.ID)
}

// SaveContact creates (id nil) or replaces a contact; the first or an explicit primary becomes the only primary.
func (s *Service) SaveContact(ctx context.Context, a Actor, employeeID, id *uuid.UUID, in ContactInput) (*EmergencyContact, error) {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return nil, err
	}
	var c *EmergencyContact
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		all, err := s.store.Contacts(ctx, tx, a.TenantID, w.ID)
		if err != nil {
			return err
		}
		if c, err = pick(all, id, func(r EmergencyContact) uuid.UUID { return r.ID }); err != nil {
			return err
		}
		if id == nil {
			if len(all) >= maxContacts {
				return ErrTooMany
			}
			c = &EmergencyContact{TenantID: a.TenantID, EmployeeProfileID: w.ID, CreatedAt: s.now().UTC()}
		}
		if err := in.apply(c); err != nil {
			return err
		}
		// Exactly one primary: the requested one, the first contact, or the current primary (moving the primary means
		// marking another contact primary).
		primary := in.IsPrimary || len(all) == 0 || c.IsPrimary
		if primary && !c.IsPrimary {
			if err := s.store.ClearPrimary(ctx, tx, a.TenantID, w.ID); err != nil {
				return err
			}
		}
		c.IsPrimary = primary
		c.UpdatedAt = s.now().UTC()
		if id == nil {
			return s.store.Create(ctx, tx, c)
		}
		return s.store.Save(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "employee.emergency_contact_saved", c.ID.String(), map[string]string{"employee_id": w.ID.String()})
	return c, nil
}

// DeleteContact removes a contact; when it was primary, the oldest remaining one becomes primary.
func (s *Service) DeleteContact(ctx context.Context, a Actor, employeeID *uuid.UUID, id uuid.UUID) error {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return err
	}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		all, err := s.store.Contacts(ctx, tx, a.TenantID, w.ID)
		if err != nil {
			return err
		}
		c, err := pick(all, &id, func(r EmergencyContact) uuid.UUID { return r.ID })
		if err != nil {
			return err
		}
		if err := s.store.Delete(ctx, tx, c); err != nil {
			return err
		}
		if !c.IsPrimary {
			return nil
		}
		for i := range all {
			if all[i].ID != id {
				all[i].IsPrimary = true
				return s.store.Save(ctx, tx, &all[i])
			}
		}
		return nil
	})
	if err == nil {
		s.log(ctx, a, "employee.emergency_contact_deleted", id.String(), map[string]string{"employee_id": w.ID.String()})
	}
	return err
}

// pick returns the row with id (nil id → nil row) or NOT_FOUND.
func pick[T any](rows []T, id *uuid.UUID, idOf func(T) uuid.UUID) (*T, error) {
	if id == nil {
		return nil, nil
	}
	for i := range rows {
		if idOf(rows[i]) == *id {
			return &rows[i], nil
		}
	}
	return nil, ErrNotFound
}

// --- education (FR-ER002) ---

// EducationInput is an education body.
type EducationInput struct {
	Institution   string  `json:"institution"`
	Qualification string  `json:"qualification"`
	FieldOfStudy  *string `json:"field_of_study"`
	StartYear     *int    `json:"start_year"`
	EndYear       *int    `json:"end_year"`
	Grade         *string `json:"grade"`
}

func (s *Service) applyEducation(in EducationInput, e *Education) error {
	var err error
	if e.Institution, err = text("institution", in.Institution, 200); err != nil {
		return err
	}
	if e.Qualification, err = text("qualification", in.Qualification, 150); err != nil {
		return err
	}
	if e.FieldOfStudy, err = optional("field_of_study", in.FieldOfStudy, 150); err != nil {
		return err
	}
	if e.Grade, err = optional("grade", in.Grade, 50); err != nil {
		return err
	}
	maxYear := s.now().Year() + 6
	for _, y := range []*int{in.StartYear, in.EndYear} {
		if y != nil && (*y < 1900 || *y > maxYear) {
			return invalid("year", "must be between 1900 and "+strconv.Itoa(maxYear))
		}
	}
	if in.StartYear != nil && in.EndYear != nil && *in.EndYear < *in.StartYear {
		return invalid("end_year", "must not be before start_year")
	}
	e.StartYear, e.EndYear = in.StartYear, in.EndYear
	return nil
}

// Educations lists education history.
func (s *Service) Educations(ctx context.Context, a Actor, employeeID *uuid.UUID) ([]Education, error) {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return nil, err
	}
	return s.store.Educations(ctx, s.store.DB(), a.TenantID, w.ID)
}

// SaveEducation creates (id nil) or replaces an education record.
func (s *Service) SaveEducation(ctx context.Context, a Actor, employeeID, id *uuid.UUID, in EducationInput) (*Education, error) {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return nil, err
	}
	var e *Education
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		all, err := s.store.Educations(ctx, tx, a.TenantID, w.ID)
		if err != nil {
			return err
		}
		if e, err = pick(all, id, func(r Education) uuid.UUID { return r.ID }); err != nil {
			return err
		}
		if id == nil {
			e = &Education{TenantID: a.TenantID, EmployeeProfileID: w.ID, CreatedAt: s.now().UTC()}
		}
		if err := s.applyEducation(in, e); err != nil {
			return err
		}
		e.UpdatedAt = s.now().UTC()
		if id == nil {
			return s.store.Create(ctx, tx, e)
		}
		return s.store.Save(ctx, tx, e)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "employee.education_saved", e.ID.String(), map[string]string{"employee_id": w.ID.String()})
	return e, nil
}

// DeleteEducation removes an education record.
func (s *Service) DeleteEducation(ctx context.Context, a Actor, employeeID *uuid.UUID, id uuid.UUID) error {
	return s.deleteOne(ctx, a, employeeID, id, "employee.education_deleted", func(tx *gorm.DB, w *Worker) (interface{}, error) {
		all, err := s.store.Educations(ctx, tx, a.TenantID, w.ID)
		if err != nil {
			return nil, err
		}
		return pick(all, &id, func(r Education) uuid.UUID { return r.ID })
	})
}

func (s *Service) deleteOne(ctx context.Context, a Actor, employeeID *uuid.UUID, id uuid.UUID, action string, find func(*gorm.DB, *Worker) (interface{}, error)) error {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return err
	}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		row, err := find(tx, w)
		if err != nil {
			return err
		}
		return s.store.Delete(ctx, tx, row)
	})
	if err == nil {
		s.log(ctx, a, action, id.String(), map[string]string{"employee_id": w.ID.String()})
	}
	return err
}

// --- prior employment (FR-ER003) ---

// PriorInput is a prior-employment body.
type PriorInput struct {
	Employer         string  `json:"employer"`
	Title            string  `json:"title"`
	StartDate        string  `json:"start_date"`
	EndDate          string  `json:"end_date"`
	ReasonForLeaving *string `json:"reason_for_leaving"`
}

func (s *Service) applyPrior(in PriorInput, p *PriorEmployment) error {
	var err error
	if p.Employer, err = text("employer", in.Employer, 200); err != nil {
		return err
	}
	if p.Title, err = text("title", in.Title, 150); err != nil {
		return err
	}
	if p.ReasonForLeaving, err = optional("reason_for_leaving", in.ReasonForLeaving, 500); err != nil {
		return err
	}
	start, err := time.Parse(DateLayout, strings.TrimSpace(in.StartDate))
	if err != nil {
		return invalid("start_date", "must be YYYY-MM-DD")
	}
	end, err := time.Parse(DateLayout, strings.TrimSpace(in.EndDate))
	if err != nil {
		return invalid("end_date", "must be YYYY-MM-DD")
	}
	if end.Before(start) || end.After(s.now().UTC()) {
		return invalid("end_date", "must be between start_date and today")
	}
	p.StartDate, p.EndDate = Date{start}, Date{end}
	return nil
}

// Priors lists prior employment, latest first.
func (s *Service) Priors(ctx context.Context, a Actor, employeeID *uuid.UUID) ([]PriorEmployment, error) {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return nil, err
	}
	return s.store.Priors(ctx, s.store.DB(), a.TenantID, w.ID)
}

// SavePrior creates (id nil) or replaces a prior employment; periods never overlap (FR-ER003).
func (s *Service) SavePrior(ctx context.Context, a Actor, employeeID, id *uuid.UUID, in PriorInput) (*PriorEmployment, error) {
	w, err := s.owner(ctx, a, employeeID)
	if err != nil {
		return nil, err
	}
	var p *PriorEmployment
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		all, err := s.store.Priors(ctx, tx, a.TenantID, w.ID)
		if err != nil {
			return err
		}
		if p, err = pick(all, id, func(r PriorEmployment) uuid.UUID { return r.ID }); err != nil {
			return err
		}
		if id == nil {
			p = &PriorEmployment{TenantID: a.TenantID, EmployeeProfileID: w.ID, CreatedAt: s.now().UTC()}
		}
		if err := s.applyPrior(in, p); err != nil {
			return err
		}
		for _, o := range all {
			if o.ID != p.ID && !o.StartDate.After(p.EndDate.Time) && !p.StartDate.After(o.EndDate.Time) {
				return ErrOverlap
			}
		}
		p.UpdatedAt = s.now().UTC()
		if id == nil {
			return s.store.Create(ctx, tx, p)
		}
		return s.store.Save(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "employee.prior_employment_saved", p.ID.String(), map[string]string{"employee_id": w.ID.String()})
	return p, nil
}

// DeletePrior removes a prior employment.
func (s *Service) DeletePrior(ctx context.Context, a Actor, employeeID *uuid.UUID, id uuid.UUID) error {
	return s.deleteOne(ctx, a, employeeID, id, "employee.prior_employment_deleted", func(tx *gorm.DB, w *Worker) (interface{}, error) {
		all, err := s.store.Priors(ctx, tx, a.TenantID, w.ID)
		if err != nil {
			return nil, err
		}
		return pick(all, &id, func(r PriorEmployment) uuid.UUID { return r.ID })
	})
}

// --- change requests (FR-CR) ---

// ChangeInput is a change-request body.
type ChangeInput struct {
	Type    string  `json:"type"`
	Payload Payload `json:"payload"`
	Reason  *string `json:"reason"`
}

// validate is FR-CR001: required fields per type; other fields are dropped.
func validate(typ string, p Payload) (Payload, error) {
	var out Payload
	var err error
	trim := func(field string, v *string, min, max int) (*string, error) {
		if v == nil {
			return nil, invalid(field, "is required")
		}
		t := strings.TrimSpace(*v)
		if n := utf8.RuneCountInString(t); n < min || n > max {
			return nil, invalid(field, "must be "+strconv.Itoa(min)+"–"+strconv.Itoa(max)+" characters")
		}
		return &t, nil
	}
	switch typ {
	case TypeName:
		if out.FirstName, err = trim("first_name", p.FirstName, 1, 100); err != nil {
			return out, err
		}
		if out.LastName, err = trim("last_name", p.LastName, 1, 100); err != nil {
			return out, err
		}
		if p.DisplayName != nil {
			if out.DisplayName, err = trim("display_name", p.DisplayName, 1, 200); err != nil {
				return out, err
			}
		}
	case TypeAddress:
		if p.CurrentAddress == nil && p.PermanentAddress == nil {
			return out, invalid("payload", "send current_address, permanent_address or both")
		}
		if p.CurrentAddress != nil {
			if out.CurrentAddress, err = trim("current_address", p.CurrentAddress, 1, 1000); err != nil {
				return out, err
			}
		}
		if p.PermanentAddress != nil {
			if out.PermanentAddress, err = trim("permanent_address", p.PermanentAddress, 1, 1000); err != nil {
				return out, err
			}
		}
	case TypeBank:
		if out.BankName, err = trim("bank_name", p.BankName, 1, 100); err != nil {
			return out, err
		}
		if out.BankAccountNumber, err = trim("bank_account_number", p.BankAccountNumber, 6, 34); err != nil || !accountFormat.MatchString(*out.BankAccountNumber) {
			return out, invalid("bank_account_number", "must be 6–34 letters or digits")
		}
		if out.BankRoutingSwift, err = trim("bank_routing_swift", p.BankRoutingSwift, 4, 20); err != nil || !routingFormat.MatchString(*out.BankRoutingSwift) {
			return out, invalid("bank_routing_swift", "must be 4–20 letters or digits")
		}
	default:
		return out, invalid("type", "must be name, address or bank")
	}
	return out, nil
}

// Submit is FR-CR002: the caller asks to change their own data.
func (s *Service) Submit(ctx context.Context, a Actor, in ChangeInput) (*ChangeRequest, error) {
	payload, err := validate(in.Type, in.Payload)
	if err != nil {
		return nil, err
	}
	reason, err := optional("reason", in.Reason, 500)
	if err != nil {
		return nil, err
	}
	w, err := s.owner(ctx, a, nil)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	r := &ChangeRequest{TenantID: a.TenantID, EmployeeProfileID: w.ID, Type: in.Type, Payload: payload, Reason: reason, Status: StatusSubmitted,
		RequestedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		open, err := s.store.OpenRequest(ctx, tx, a.TenantID, w.ID, in.Type)
		if err != nil {
			return err
		}
		if open {
			return ErrOpenRequest
		}
		if err := s.store.Create(ctx, tx, r); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "employee.change_request.submitted", eventOf(r))
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "employee.change_request_submitted", r.ID.String(), eventOf(r))
	return s.masked(r, a), nil
}

func eventOf(r *ChangeRequest) map[string]string {
	return map[string]string{"request_id": r.ID.String(), "employee_id": r.EmployeeProfileID.String(), "type": r.Type, "status": r.Status}
}

// masked hides the bank account number unless the caller may read sensitive data (FR-CR005).
func (s *Service) masked(r *ChangeRequest, a Actor) *ChangeRequest {
	out := *r
	if !a.Sensitive && out.Payload.BankAccountNumber != nil {
		v := *out.Payload.BankAccountNumber
		m := "****" + v[max(0, len(v)-4):]
		out.Payload.BankAccountNumber = &m
	}
	return &out
}

// Mine lists the caller's requests.
func (s *Service) Mine(ctx context.Context, a Actor, status string, page, perPage int) ([]ChangeRequest, int64, error) {
	w, err := s.owner(ctx, a, nil)
	if err != nil {
		return nil, 0, err
	}
	return s.list(ctx, a, RequestFilter{EmployeeID: &w.ID, Status: status}, page, perPage)
}

// List is FR-CR003 for approvers.
func (s *Service) List(ctx context.Context, a Actor, f RequestFilter, page, perPage int) ([]ChangeRequest, int64, error) {
	return s.list(ctx, a, f, page, perPage)
}

func (s *Service) list(ctx context.Context, a Actor, f RequestFilter, page, perPage int) ([]ChangeRequest, int64, error) {
	if f.Status != "" && f.Status != StatusSubmitted && f.Status != StatusApproved && f.Status != StatusRejected && f.Status != StatusCancelled {
		return nil, 0, invalid("status", "must be submitted, approved, rejected or cancelled")
	}
	if f.Type != "" && f.Type != TypeName && f.Type != TypeAddress && f.Type != TypeBank {
		return nil, 0, invalid("type", "must be name, address or bank")
	}
	rows, total, err := s.store.Requests(ctx, s.store.DB(), a.TenantID, f, page, perPage)
	for i := range rows {
		rows[i] = *s.masked(&rows[i], a)
	}
	return rows, total, err
}

// Get returns one request (approvers).
func (s *Service) Get(ctx context.Context, a Actor, id uuid.UUID) (*ChangeRequest, error) {
	r, err := s.store.Request(ctx, s.store.DB(), a.TenantID, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrNotFound
	}
	return s.masked(r, a), nil
}

// Cancel withdraws the caller's own submitted request.
func (s *Service) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*ChangeRequest, error) {
	w, err := s.owner(ctx, a, nil)
	if err != nil {
		return nil, err
	}
	return s.decide(ctx, a, id, StatusCancelled, nil, func(r *ChangeRequest) error {
		if r.EmployeeProfileID != w.ID {
			return ErrNotFound
		}
		return nil
	})
}

// Approve is FR-CR003/FR-CR004: applies the change in the same transaction.
func (s *Service) Approve(ctx context.Context, a Actor, id uuid.UUID, comment *string) (*ChangeRequest, error) {
	return s.decide(ctx, a, id, StatusApproved, comment, s.notSelf(ctx, a))
}

// Reject is FR-CR003.
func (s *Service) Reject(ctx context.Context, a Actor, id uuid.UUID, comment *string) (*ChangeRequest, error) {
	return s.decide(ctx, a, id, StatusRejected, comment, s.notSelf(ctx, a))
}

// notSelf refuses decisions on the approver's own requests (EMPLOYEE_SELF_APPROVAL).
func (s *Service) notSelf(ctx context.Context, a Actor) func(*ChangeRequest) error {
	return func(r *ChangeRequest) error {
		if r.RequestedByUserID == a.UserID {
			return ErrSelfApproval
		}
		w, err := s.store.Worker(ctx, a.TenantID, r.EmployeeProfileID)
		if err != nil {
			return err
		}
		if w != nil && w.UserID == a.UserID {
			return ErrSelfApproval
		}
		return nil
	}
}

func (s *Service) decide(ctx context.Context, a Actor, id uuid.UUID, to string, comment *string, check func(*ChangeRequest) error) (*ChangeRequest, error) {
	c, err := optional("comment", comment, 1000)
	if err != nil {
		return nil, err
	}
	var r *ChangeRequest
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if r, err = s.store.LockRequest(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if r == nil {
			return ErrNotFound
		}
		if err := check(r); err != nil {
			return err
		}
		if r.Status != StatusSubmitted {
			return ErrRequestState
		}
		now := s.now().UTC()
		r.Status, r.UpdatedAt = to, now
		if to != StatusCancelled {
			r.DecidedByUserID, r.DecisionComment, r.DecidedAt = &a.UserID, c, &now
		}
		if to == StatusApproved {
			if err := s.store.Apply(ctx, tx, a.TenantID, r.EmployeeProfileID, r.Type, r.Payload); err != nil {
				return err
			}
		}
		if err := s.store.Save(ctx, tx, r); err != nil {
			return err
		}
		if to == StatusCancelled {
			return nil
		}
		return s.store.Event(ctx, tx, a.TenantID, "employee.change_request."+to, eventOf(r))
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "employee.change_request_"+to, r.ID.String(), eventOf(r))
	return s.masked(r, a), nil
}
