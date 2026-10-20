package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/services"
)

// CalibrationService is the calibration surface (FR-CA001, FR-CA002).
type CalibrationService interface {
	Calibrate(ctx context.Context, a services.Actor, cycleID uuid.UUID, req dto.CalibrateRequest) (*dto.CalibrationResponse, error)
	List(ctx context.Context, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]dto.CalibrationResponse, dto.PageMeta, error)
}

// FeedbackService is the continuous-feedback surface (F1–F9).
type FeedbackService interface {
	Give(ctx context.Context, a services.Actor, req dto.GiveFeedbackRequest) (*dto.FeedbackResponse, error)
	ListReceived(ctx context.Context, a services.Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)
	ListGiven(ctx context.Context, a services.Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)
	ListFor(ctx context.Context, a services.Actor, employeeID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)
	ListPublic(ctx context.Context, tenantID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)
	Request(ctx context.Context, a services.Actor, req dto.RequestFeedbackRequest) ([]dto.FeedbackRequestResponse, error)
	ListAssigned(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)
	ListMine(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)
	Decline(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.FeedbackRequestResponse, error)
}

// --- calibration ---

func (ctl *Controller) Calibrate(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.CalibrateRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.calibrations.Calibrate(c.Request.Context(), a, id, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) ListCalibrations(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	emp, okE := queryUUID(c, "employee_id")
	if !okE {
		return
	}
	res, meta, err := ctl.calibrations.List(c.Request.Context(), a.TenantID, id, emp, page(c))
	respondList(c, res, meta, err)
}

// --- continuous feedback ---

func (ctl *Controller) GiveFeedback(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.GiveFeedbackRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.feedback.Give(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) FeedbackReceived(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.feedback.ListReceived(c.Request.Context(), a, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) FeedbackGiven(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.feedback.ListGiven(c.Request.Context(), a, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) FeedbackFor(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	emp, okE := queryUUID(c, "employee_id")
	if !okE {
		return
	}
	if emp == nil {
		fail(c, invalidQuery("employee_id is required"))
		return
	}
	res, meta, err := ctl.feedback.ListFor(c.Request.Context(), a, *emp, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) PublicFeedback(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.feedback.ListPublic(c.Request.Context(), a.TenantID, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) RequestFeedback(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.RequestFeedbackRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.feedback.Request(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) AssignedFeedbackRequests(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.feedback.ListAssigned(c.Request.Context(), a, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) MyFeedbackRequests(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.feedback.ListMine(c.Request.Context(), a, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) DeclineFeedbackRequest(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.feedback.Decline(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}
