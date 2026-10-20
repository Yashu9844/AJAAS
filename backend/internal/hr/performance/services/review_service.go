package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/events"
	"github.com/jaas/jaas/internal/hr/performance/lifecycle"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/hr/performance/score"
	"github.com/jaas/jaas/internal/hr/performance/validators"
	"gorm.io/gorm"
)

// ReviewService implements FR-RV (V1–V6).
type ReviewService struct{ base }

// NewReviewService builds the service.
func NewReviewService(d Deps) *ReviewService { return &ReviewService{base{d}} }

// Assign is V1 (FR-RV001, PM-011).
func (s *ReviewService) Assign(ctx context.Context, a Actor, req dto.AssignReviewerRequest) (*dto.ReviewResponse, error) {
	cycleID, err := parseUUIDField("cycle_id", req.CycleID)
	if err != nil {
		return nil, err
	}
	subject, err := parseUUIDField("employee_id", req.EmployeeID)
	if err != nil {
		return nil, err
	}
	reviewer, err := parseUUIDField("reviewer_employee_id", req.ReviewerEmployeeID)
	if err != nil {
		return nil, err
	}
	if subject == reviewer {
		return nil, invalid("reviewer_employee_id", "must differ from employee_id")
	}
	if _, err := s.employee(ctx, a.TenantID, subject); err != nil {
		return nil, err
	}
	rev, err := s.employee(ctx, a.TenantID, reviewer)
	if err != nil {
		return nil, err
	}
	if !workingStatuses[rev.Status] {
		return nil, ErrEmployeeState
	}
	var r *models.Review
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.Repos.Cycles.LockByID(ctx, tx, a.TenantID, cycleID)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		if !lifecycle.ReviewsOpen(c.Status) {
			return ErrCycleState
		}
		exists, err := s.Repos.Reviews.Exists(ctx, tx, a.TenantID, cycleID, subject, reviewer, req.Type)
		if err != nil {
			return err
		}
		if exists {
			return ErrDuplicateReview
		}
		now := s.Now().UTC()
		r = &models.Review{TenantID: a.TenantID, CycleID: cycleID, EmployeeID: subject, ReviewerEmployeeID: reviewer, Type: req.Type,
			Status: models.ReviewPending, Ratings: models.Ratings{}, CreatedAt: now, UpdatedAt: now}
		return s.Repos.Reviews.Create(ctx, tx, r)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.reviewer_assigned", "performance_review", r.ID.String(),
		map[string]string{"cycle_id": r.CycleID.String(), "employee_id": r.EmployeeID.String(), "type": r.Type}}, nil)
	out := reviewResponse(r, false)
	return &out, nil
}

// List is V2 (performance:read).
func (s *ReviewService) List(ctx context.Context, tenantID uuid.UUID, f repositories.ReviewFilter, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error) {
	if err := reviewStatus(f.Status); err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Reviews.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return reviewResponses(rows, false), page.Meta(total), nil
}

// ListAssigned is V3: reviews the caller must write.
func (s *ReviewService) ListAssigned(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error) {
	if err := reviewStatus(status); err != nil {
		return nil, dto.PageMeta{}, err
	}
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Reviews.List(ctx, s.Tx.DB(), a.TenantID, repositories.ReviewFilter{ReviewerID: &self.ID, Status: status}, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return reviewResponses(rows, false), page.Meta(total), nil
}

// ListMine is V4 (PM-014, PM-021): the caller's submitted reviews of completed cycles with the cycle's final rating;
// peer reviewers stay anonymous and calibration reasons are never included.
func (s *ReviewService) ListMine(ctx context.Context, a Actor, cycleID *uuid.UUID) ([]dto.ReviewResponse, error) {
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, err
	}
	rows, _, err := s.Repos.Reviews.List(ctx, s.Tx.DB(), a.TenantID, repositories.ReviewFilter{CycleID: cycleID, EmployeeID: &self.ID,
		Status: models.ReviewSubmitted}, dto.Page{Page: 1, PerPage: reviewsOfMine})
	if err != nil {
		return nil, err
	}
	completed := map[uuid.UUID]bool{}
	visible := make([]models.Review, 0, len(rows))
	for _, r := range rows {
		done, seen := completed[r.CycleID]
		if !seen {
			c, err := s.Repos.Cycles.FindByID(ctx, s.Tx.DB(), a.TenantID, r.CycleID)
			if err != nil {
				return nil, err
			}
			done = c != nil && c.Status == lifecycle.CycleCompleted
			completed[r.CycleID] = done
		}
		if done {
			visible = append(visible, r)
		}
	}
	out := reviewResponses(visible, true)
	finals := map[uuid.UUID]*dto.FinalRating{}
	for i := range out {
		cid := out[i].CycleID
		if _, ok := finals[cid]; !ok {
			if finals[cid], err = s.finalRating(ctx, s.Tx.DB(), a.TenantID, cid, self.ID); err != nil {
				return nil, err
			}
		}
		out[i].Final = finals[cid]
	}
	return out, nil
}

// Get is V5: the reviewer or performance:read; anyone else 404.
func (s *ReviewService) Get(ctx context.Context, a Actor, id uuid.UUID) (*dto.ReviewResponse, error) {
	r, err := s.Repos.Reviews.FindByID(ctx, s.Tx.DB(), a.TenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	if !a.CanRead && !a.CanManage {
		self, err := s.optionalSelf(ctx, a)
		if err != nil {
			return nil, err
		}
		if self == nil || self.ID != r.ReviewerEmployeeID {
			return nil, ErrNotFound
		}
	}
	out := reviewResponse(r, false)
	return &out, nil
}

// Submit is V6 (FR-RV003, PM-012): only the assigned reviewer, once, while the cycle is in review.
func (s *ReviewService) Submit(ctx context.Context, a Actor, id uuid.UUID, req dto.SubmitReviewRequest) (*dto.ReviewResponse, error) {
	strengths, err := optionalText("strengths", req.Strengths, maxLongText)
	if err != nil {
		return nil, err
	}
	improvements, err := optionalText("improvements", req.Improvements, maxLongText)
	if err != nil {
		return nil, err
	}
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, ErrNotFound
	}
	var r *models.Review
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if r, err = s.Repos.Reviews.LockByID(ctx, tx, a.TenantID, id); err != nil || r == nil || r.ReviewerEmployeeID != self.ID {
			return orNotFound(err)
		}
		if r.Status != models.ReviewPending {
			return ErrReviewState
		}
		c, err := s.Repos.Cycles.LockByID(ctx, tx, a.TenantID, r.CycleID)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		if !lifecycle.ReviewsOpen(c.Status) {
			return ErrCycleState
		}
		if err := s.fill(r, c.Criteria, req); err != nil {
			return err
		}
		r.Strengths, r.Improvements = strengths, improvements
		if err := s.Repos.Reviews.Update(ctx, tx, r); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.ReviewSubmitted, events.Payload{CycleID: r.CycleID, ReviewID: &r.ID, EmployeeID: &r.EmployeeID,
			Status: r.Status, ReviewType: r.Type})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.review_submitted", "performance_review", r.ID.String(),
		map[string]string{"cycle_id": r.CycleID.String(), "type": r.Type}}, row)
	out := reviewResponse(r, false)
	return &out, nil
}

// fill validates ratings and potential against the cycle (PM-012) and marks the review submitted.
func (s *ReviewService) fill(r *models.Review, criteria []string, req dto.SubmitReviewRequest) error {
	if err := validators.Ratings(criteria, req.Ratings); err != nil {
		return invalid("ratings", err.Error())
	}
	if err := validators.Potential(r.Type, req.Potential); err != nil {
		return invalid("potential", err.Error())
	}
	r.Ratings = validators.Canonical(criteria, req.Ratings)
	scores := make([]int, len(r.Ratings))
	for i, rt := range r.Ratings {
		scores[i] = rt.Score
	}
	overall := score.Overall(scores)
	now := s.Now().UTC()
	r.OverallScore, r.Potential, r.Status, r.SubmittedAt, r.UpdatedAt = &overall, req.Potential, models.ReviewSubmitted, &now, now
	return nil
}

func reviewStatus(s string) error {
	if s != "" && s != models.ReviewPending && s != models.ReviewSubmitted && s != models.ReviewExpired {
		return invalid("status", "must be pending, submitted or expired")
	}
	return nil
}
