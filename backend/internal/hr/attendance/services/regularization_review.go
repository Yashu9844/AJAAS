package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/attendance/dto"
	"github.com/jaas/jaas/internal/hr/attendance/events"
	"github.com/jaas/jaas/internal/hr/attendance/models"
	"github.com/jaas/jaas/internal/hr/attendance/validators"
	"gorm.io/gorm"
)

// Approve applies the requested window to the record in one transaction (AT-016, AT-017).
func (s *regularizationService) Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error) {
	g, err := s.loadPending(ctx, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardSelfReview(ctx, a, g); err != nil {
		return nil, err
	}
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.applyToRecord(ctx, tx, g); err != nil {
			return err
		}
		s.markReviewed(g, a, models.RegApproved, req.Comment)
		if err := s.Repos.Regularizations.Update(ctx, tx, g); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.RegularizationApproved, regPayload(g))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"attendance.regularization_approved", "attendance_regularization", g.ID.String(),
		map[string]string{"attendance_date": g.AttendanceDate.Format(validators.DateLayout)}}, row)
	res := mapRegularization(g)
	return &res, nil
}

// applyToRecord supersedes existing punches and inserts the requested in/out (AT-017, AT-019).
func (s *regularizationService) applyToRecord(ctx context.Context, tx *gorm.DB, g *models.Regularization) error {
	rec, err := s.setWindow(ctx, tx, g.TenantID, g.EmployeeProfileID, g.AttendanceDate, g.RequestedPunchIn, g.RequestedPunchOut, models.SourceRegularization)
	if err != nil {
		return err
	}
	g.AttendanceRecordID = &rec.ID
	return nil
}

// Reject closes a pending request with a mandatory comment (AT-016).
func (s *regularizationService) Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error) {
	if req.Comment == nil || strings.TrimSpace(*req.Comment) == "" {
		return nil, invalid("comment", "required when rejecting")
	}
	g, err := s.loadPending(ctx, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardSelfReview(ctx, a, g); err != nil {
		return nil, err
	}
	s.markReviewed(g, a, models.RegRejected, req.Comment)
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.Repos.Regularizations.Update(ctx, tx, g); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.RegularizationRejected, regPayload(g))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"attendance.regularization_rejected", "attendance_regularization", g.ID.String(), nil}, row)
	res := mapRegularization(g)
	return &res, nil
}

func (s *regularizationService) markReviewed(g *models.Regularization, a Actor, status string, comment *string) {
	now := s.Now().UTC()
	reviewer := a.UserID
	g.Status, g.ReviewerUserID, g.ReviewedAt, g.ReviewComment = status, &reviewer, &now, comment
}

func regPayload(g *models.Regularization) events.RegularizationPayload {
	return events.RegularizationPayload{RegularizationID: g.ID, EmployeeID: g.EmployeeProfileID,
		AttendanceDate: g.AttendanceDate.Format(validators.DateLayout), Status: g.Status,
		RequestedPunchIn: g.RequestedPunchIn, RequestedPunchOut: g.RequestedPunchOut, ReviewerUserID: g.ReviewerUserID}
}
