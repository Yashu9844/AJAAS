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
	"gorm.io/gorm"
)

// Calibration limits (FR-CA001).
const (
	minReason = 10
	maxReason = 1000
	minScore  = score.Fixed(100)
	maxScore  = score.Fixed(500)
)

var potentialLevels = map[string]bool{"low": true, "medium": true, "high": true}

// CalibrationService implements calibration (PM-018..PM-022).
type CalibrationService struct{ base }

// NewCalibrationService builds the service.
func NewCalibrationService(d Deps) *CalibrationService { return &CalibrationService{base{d}} }

// Calibrate is FR-CA001: an append-only override of the employee's final rating while the cycle is in review.
func (s *CalibrationService) Calibrate(ctx context.Context, a Actor, cycleID uuid.UUID, req dto.CalibrateRequest) (*dto.CalibrationResponse, error) {
	employeeID, err := parseUUIDField("employee_id", req.EmployeeID)
	if err != nil {
		return nil, err
	}
	reason, err := text("reason", req.Reason, maxReason)
	if err != nil || len([]rune(reason)) < minReason {
		return nil, invalid("reason", "must be 10–1000 characters")
	}
	if req.OverallScore == nil && req.Potential == nil {
		return nil, invalid("body", "send overall_score, potential or both")
	}
	if req.OverallScore != nil && (*req.OverallScore < minScore || *req.OverallScore > maxScore) {
		return nil, invalid("overall_score", "must be between 1.00 and 5.00")
	}
	if req.Potential != nil && !potentialLevels[*req.Potential] {
		return nil, invalid("potential", "must be low, medium or high")
	}
	var c *models.Calibration
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		cy, err := s.Repos.Cycles.LockByID(ctx, tx, a.TenantID, cycleID)
		if err != nil || cy == nil {
			return orNotFound(err)
		}
		if cy.Status != lifecycle.CycleReview {
			return ErrCycleState
		}
		cur, err := s.finalRating(ctx, tx, a.TenantID, cycleID, employeeID)
		if err != nil {
			return err
		}
		if cur == nil {
			return ErrNotRated
		}
		c = &models.Calibration{TenantID: a.TenantID, CycleID: cycleID, EmployeeID: employeeID, BeforeScore: cur.OverallScore,
			BeforePotential: cur.Potential, AfterScore: cur.OverallScore, AfterPotential: cur.Potential, Reason: reason,
			CalibratedByUserID: a.UserID, CreatedAt: s.Now().UTC()}
		if req.OverallScore != nil {
			c.AfterScore = *req.OverallScore
		}
		if req.Potential != nil {
			c.AfterPotential = *req.Potential
		}
		if c.AfterScore == c.BeforeScore && c.AfterPotential == c.BeforePotential {
			return invalid("body", "the calibration changes nothing")
		}
		if err := s.Repos.Calibrations.Create(ctx, tx, c); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.RatingCalibrated, events.Payload{CycleID: cycleID, EmployeeID: &employeeID})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.rating_calibrated", "performance_calibration", c.ID.String(),
		map[string]string{"cycle_id": cycleID.String(), "employee_id": employeeID.String()}}, row)
	out := calibrationResponse(c)
	return &out, nil
}

// List is FR-CA002: the cycle's calibrations, newest first.
func (s *CalibrationService) List(ctx context.Context, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]dto.CalibrationResponse, dto.PageMeta, error) {
	cy, err := s.Repos.Cycles.FindByID(ctx, s.Tx.DB(), tenantID, cycleID)
	if err != nil || cy == nil {
		return nil, dto.PageMeta{}, orNotFound(err)
	}
	rows, total, err := s.Repos.Calibrations.List(ctx, s.Tx.DB(), tenantID, cycleID, employeeID, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.CalibrationResponse, len(rows))
	for i := range rows {
		out[i] = calibrationResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// finalRating is PM-020 for one employee: the latest calibration, else the latest submitted manager review; nil when
// neither exists.
func (b base) finalRating(ctx context.Context, db *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*dto.FinalRating, error) {
	cal, err := b.Repos.Calibrations.Latest(ctx, db, tenantID, cycleID, employeeID)
	if err != nil {
		return nil, err
	}
	if cal != nil {
		return &dto.FinalRating{OverallScore: cal.AfterScore, Potential: cal.AfterPotential, Calibrated: true}, nil
	}
	reviews, _, err := b.Repos.Reviews.List(ctx, db, tenantID, repositories.ReviewFilter{CycleID: &cycleID, EmployeeID: &employeeID,
		Status: models.ReviewSubmitted, Type: models.ReviewManager}, everything)
	if err != nil {
		return nil, err
	}
	return managerRating(latestReviews(reviews)[employeeID]), nil
}

// latestReviews keeps each employee's most recently submitted review.
func latestReviews(reviews []models.Review) map[uuid.UUID]*models.Review {
	latest := map[uuid.UUID]*models.Review{}
	for i := range reviews {
		r := &reviews[i]
		if cur, ok := latest[r.EmployeeID]; !ok || r.SubmittedAt.After(*cur.SubmittedAt) {
			latest[r.EmployeeID] = r
		}
	}
	return latest
}

func managerRating(r *models.Review) *dto.FinalRating {
	if r == nil || r.OverallScore == nil || r.Potential == nil {
		return nil
	}
	return &dto.FinalRating{OverallScore: *r.OverallScore, Potential: *r.Potential}
}

func calibrationResponse(c *models.Calibration) dto.CalibrationResponse {
	return dto.CalibrationResponse{ID: c.ID, CycleID: c.CycleID, EmployeeID: c.EmployeeID, BeforeScore: c.BeforeScore, BeforePotential: c.BeforePotential,
		AfterScore: c.AfterScore, AfterPotential: c.AfterPotential, Reason: c.Reason, CalibratedByUserID: c.CalibratedByUserID, CreatedAt: c.CreatedAt}
}
