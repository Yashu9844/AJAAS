package services

import (
	"context"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/events"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"gorm.io/gorm"
)

// Continuous-feedback limits (FB-001, FB-004).
const (
	maxMessage    = 2000
	maxQuestion   = 500
	maxResponders = 10
)

// FeedbackService implements continuous feedback (F1–F9, FB-001..FB-006).
type FeedbackService struct{ base }

// NewFeedbackService builds the service.
func NewFeedbackService(d Deps) *FeedbackService { return &FeedbackService{base{d}} }

// Give is F1: feedback to a colleague, optionally answering a request (FB-001, FB-005).
func (s *FeedbackService) Give(ctx context.Context, a Actor, req dto.GiveFeedbackRequest) (*dto.FeedbackResponse, error) {
	msg, err := text("message", req.Message, maxMessage)
	if err != nil {
		return nil, err
	}
	if req.Visibility == models.VisibilityPublic && req.Kind != models.FeedbackPraise {
		return nil, invalid("visibility", "only praise can be public")
	}
	to, err := parseUUIDField("to_employee_id", req.ToEmployeeID)
	if err != nil {
		return nil, err
	}
	requestID, err := parseOptionalUUID("request_id", req.RequestID)
	if err != nil {
		return nil, err
	}
	self, err := s.workingSelf(ctx, a)
	if err != nil {
		return nil, err
	}
	if to == self.ID {
		return nil, invalid("to_employee_id", "you cannot give feedback to yourself")
	}
	if err := s.working(ctx, a.TenantID, to); err != nil {
		return nil, err
	}
	f := &models.Feedback{TenantID: a.TenantID, FromEmployeeID: self.ID, ToEmployeeID: to, Kind: req.Kind, Visibility: req.Visibility,
		Message: msg, RequestID: requestID}
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		f.CreatedAt = s.Now().UTC()
		if err := s.Repos.Feedback.Create(ctx, tx, f); err != nil {
			return err
		}
		if requestID != nil {
			if err := s.answer(ctx, tx, a, self.ID, *requestID, f); err != nil {
				return err
			}
		}
		var err error
		row, err = s.writeOutbox(ctx, tx, a, events.FeedbackGiven, events.FeedbackPayload{FeedbackID: &f.ID, RequestID: requestID,
			FromID: f.FromEmployeeID, ToID: f.ToEmployeeID, Kind: f.Kind, Visibility: f.Visibility})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.feedback_given", "performance_feedback", f.ID.String(),
		map[string]string{"to_employee_id": to.String(), "kind": f.Kind, "visibility": f.Visibility}}, row)
	out := feedbackResponse(f)
	return &out, nil
}

// answer is FB-005: the caller answers their pending request about the recipient.
func (s *FeedbackService) answer(ctx context.Context, tx *gorm.DB, a Actor, self, requestID uuid.UUID, f *models.Feedback) error {
	r, err := s.Repos.Feedback.LockRequest(ctx, tx, a.TenantID, requestID)
	if err != nil || r == nil || r.ResponderEmployeeID != self {
		return orNotFound(err)
	}
	if r.Status != models.RequestPending {
		return ErrRequestState
	}
	if r.AboutEmployeeID != f.ToEmployeeID {
		return invalid("to_employee_id", "must be the employee the request is about")
	}
	r.Status, r.FeedbackID, r.UpdatedAt = models.RequestAnswered, &f.ID, s.Now().UTC()
	return s.Repos.Feedback.UpdateRequest(ctx, tx, r)
}

// ListReceived is F2.
func (s *FeedbackService) ListReceived(ctx context.Context, a Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return s.list(ctx, a.TenantID, repositories.FeedbackFilter{ToID: &self.ID}, page)
}

// ListGiven is F3.
func (s *FeedbackService) ListGiven(ctx context.Context, a Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return s.list(ctx, a.TenantID, repositories.FeedbackFilter{FromID: &self.ID}, page)
}

// ListFor is F4: feedback an employee received, for their direct manager or performance:read (FB-002).
func (s *FeedbackService) ListFor(ctx context.Context, a Actor, employeeID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	if !a.CanRead && !a.CanManage {
		ok, err := s.managesEmployee(ctx, a, employeeID)
		if err != nil || !ok {
			return nil, dto.PageMeta{}, orNotFound(err)
		}
	}
	return s.list(ctx, a.TenantID, repositories.FeedbackFilter{ToID: &employeeID}, page)
}

// ListPublic is F5: the tenant's praise wall.
func (s *FeedbackService) ListPublic(ctx context.Context, tenantID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	return s.list(ctx, tenantID, repositories.FeedbackFilter{PublicOnly: true}, page)
}

func (s *FeedbackService) list(ctx context.Context, tenantID uuid.UUID, f repositories.FeedbackFilter, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Feedback.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.FeedbackResponse, len(rows))
	for i := range rows {
		out[i] = feedbackResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Request is F6 (FB-004): ask 1–10 working colleagues for feedback about yourself or a direct report.
func (s *FeedbackService) Request(ctx context.Context, a Actor, req dto.RequestFeedbackRequest) ([]dto.FeedbackRequestResponse, error) {
	question, err := text("question", req.Question, maxQuestion)
	if err != nil {
		return nil, err
	}
	self, err := s.workingSelf(ctx, a)
	if err != nil {
		return nil, err
	}
	about, err := s.subject(ctx, a, self, req.AboutEmployeeID)
	if err != nil {
		return nil, err
	}
	responders, err := s.responders(ctx, a.TenantID, self.ID, about, req.ResponderEmployeeIDs)
	if err != nil {
		return nil, err
	}
	created := make([]models.FeedbackRequest, 0, len(responders))
	var rows []*models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		created, rows = created[:0], rows[:0]
		for _, responder := range responders {
			exists, err := s.Repos.Feedback.PendingExists(ctx, tx, a.TenantID, self.ID, responder, about)
			if err != nil {
				return err
			}
			if exists {
				return ErrDuplicateRequest
			}
			now := s.Now().UTC()
			r := models.FeedbackRequest{TenantID: a.TenantID, RequesterEmployeeID: self.ID, ResponderEmployeeID: responder, AboutEmployeeID: about,
				Question: question, Status: models.RequestPending, CreatedAt: now, UpdatedAt: now}
			if err := s.Repos.Feedback.CreateRequest(ctx, tx, &r); err != nil {
				return err
			}
			row, err := s.writeOutbox(ctx, tx, a, events.FeedbackRequested, events.FeedbackPayload{RequestID: &r.ID, FromID: responder, ToID: about})
			if err != nil {
				return err
			}
			created, rows = append(created, r), append(rows, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]dto.FeedbackRequestResponse, len(created))
	for i := range created {
		s.afterCommit(ctx, a, auditEntry{"performance.feedback_requested", "performance_feedback_request", created[i].ID.String(),
			map[string]string{"responder_employee_id": created[i].ResponderEmployeeID.String(), "about_employee_id": about.String()}}, rows[i])
		out[i] = requestResponse(&created[i])
	}
	return out, nil
}

// subject resolves about_employee_id: the requester, or one of their direct reports (else 404).
func (s *FeedbackService) subject(ctx context.Context, a Actor, self *employeeDTO.EmployeeResponse, raw *string) (uuid.UUID, error) {
	about, err := parseOptionalUUID("about_employee_id", raw)
	if err != nil || about == nil || *about == self.ID {
		return self.ID, err
	}
	ok, err := s.managesEmployee(ctx, a, *about)
	if err != nil || !ok {
		return uuid.Nil, orNotFound(err)
	}
	return *about, nil
}

// responders validates FB-004: 1–10 distinct working colleagues, never the requester or the subject.
func (s *FeedbackService) responders(ctx context.Context, tenantID, requester, about uuid.UUID, raw []string) ([]uuid.UUID, error) {
	if len(raw) == 0 || len(raw) > maxResponders {
		return nil, invalid("responder_employee_ids", "ask 1–10 colleagues")
	}
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := parseUUIDField("responder_employee_ids", r)
		if err != nil {
			return nil, err
		}
		if seen[id] || id == requester || id == about {
			return nil, invalid("responder_employee_ids", "must be distinct colleagues other than you and the subject")
		}
		seen[id] = true
		if err := s.working(ctx, tenantID, id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// ListAssigned is F7: requests the caller must answer.
func (s *FeedbackService) ListAssigned(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error) {
	return s.listRequests(ctx, a, status, page, func(id uuid.UUID) repositories.RequestFilter { return repositories.RequestFilter{ResponderID: &id} })
}

// ListMine is F8: requests the caller made.
func (s *FeedbackService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error) {
	return s.listRequests(ctx, a, status, page, func(id uuid.UUID) repositories.RequestFilter { return repositories.RequestFilter{RequesterID: &id} })
}

func (s *FeedbackService) listRequests(ctx context.Context, a Actor, status string, page dto.Page, by func(uuid.UUID) repositories.RequestFilter) ([]dto.FeedbackRequestResponse, dto.PageMeta, error) {
	if status != "" && status != models.RequestPending && status != models.RequestAnswered && status != models.RequestDeclined {
		return nil, dto.PageMeta{}, invalid("status", "must be pending, answered or declined")
	}
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	f := by(self.ID)
	f.Status = status
	rows, total, err := s.Repos.Feedback.ListRequests(ctx, s.Tx.DB(), a.TenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.FeedbackRequestResponse, len(rows))
	for i := range rows {
		out[i] = requestResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Decline is F9: the responder declines a pending request.
func (s *FeedbackService) Decline(ctx context.Context, a Actor, id uuid.UUID) (*dto.FeedbackRequestResponse, error) {
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, ErrNotFound
	}
	var r *models.FeedbackRequest
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if r, err = s.Repos.Feedback.LockRequest(ctx, tx, a.TenantID, id); err != nil || r == nil || r.ResponderEmployeeID != self.ID {
			return orNotFound(err)
		}
		if r.Status != models.RequestPending {
			return ErrRequestState
		}
		r.Status, r.UpdatedAt = models.RequestDeclined, s.Now().UTC()
		return s.Repos.Feedback.UpdateRequest(ctx, tx, r)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.feedback_request_declined", "performance_feedback_request", r.ID.String(), nil}, nil)
	out := requestResponse(r)
	return &out, nil
}

// workingSelf is the caller's profile, which must be working (FB-001).
func (s *FeedbackService) workingSelf(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, err
	}
	if !workingStatuses[self.Status] {
		return nil, ErrEmployeeState
	}
	return self, nil
}

// working checks that an employee of the tenant exists and is working.
func (s *FeedbackService) working(ctx context.Context, tenantID, id uuid.UUID) error {
	emp, err := s.employee(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if !workingStatuses[emp.Status] {
		return ErrEmployeeState
	}
	return nil
}

func feedbackResponse(f *models.Feedback) dto.FeedbackResponse {
	return dto.FeedbackResponse{ID: f.ID, FromEmployeeID: f.FromEmployeeID, ToEmployeeID: f.ToEmployeeID, Kind: f.Kind, Visibility: f.Visibility,
		Message: f.Message, RequestID: f.RequestID, CreatedAt: f.CreatedAt}
}

func requestResponse(r *models.FeedbackRequest) dto.FeedbackRequestResponse {
	return dto.FeedbackRequestResponse{ID: r.ID, RequesterEmployeeID: r.RequesterEmployeeID, ResponderEmployeeID: r.ResponderEmployeeID,
		AboutEmployeeID: r.AboutEmployeeID, Question: r.Question, Status: r.Status, FeedbackID: r.FeedbackID, CreatedAt: r.CreatedAt}
}
