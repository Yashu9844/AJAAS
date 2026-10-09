package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/repositories"
)

// CreateCandidate handles POST /recruitment/candidates (R6).
func (ctl *Controller) CreateCandidate(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateCandidateRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.candidates.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListCandidates handles GET /recruitment/candidates (R7).
func (ctl *Controller) ListCandidates(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	job, okJ := queryUUID(c, "job_id")
	if !okJ {
		return
	}
	res, meta, err := ctl.candidates.List(c.Request.Context(), a.TenantID, repositories.CandidateFilter{JobID: job, Stage: c.Query("stage")}, page(c))
	respondList(c, res, meta, err)
}

// GetCandidate handles GET /recruitment/candidates/:id (R8).
func (ctl *Controller) GetCandidate(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.candidates.Get(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

// UpdateCandidate handles PATCH /recruitment/candidates/:id (R9).
func (ctl *Controller) UpdateCandidate(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.UpdateCandidateRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.candidates.Update(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// MoveStage handles POST /recruitment/candidates/:id/stage (R10).
func (ctl *Controller) MoveStage(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.StageRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.candidates.MoveStage(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// Hire handles POST /recruitment/candidates/:id/hire (R11).
func (ctl *Controller) Hire(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.HireRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.hire.Hire(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// ScheduleInterview handles POST /recruitment/interviews (R12).
func (ctl *Controller) ScheduleInterview(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.ScheduleInterviewRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.interviews.Schedule(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListInterviews handles GET /recruitment/interviews?candidate_id (R13).
func (ctl *Controller) ListInterviews(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	cand, okC := requiredQueryUUID(c, "candidate_id")
	if !okC {
		return
	}
	res, meta, err := ctl.interviews.ListByCandidate(c.Request.Context(), a.TenantID, cand, page(c))
	respondList(c, res, meta, err)
}

// MyInterviews handles GET /recruitment/interviews/me (R14).
func (ctl *Controller) MyInterviews(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.interviews.ListMine(c.Request.Context(), a, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

// Feedback handles POST /recruitment/interviews/:id/feedback (R15).
func (ctl *Controller) Feedback(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.FeedbackRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.interviews.Feedback(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// CancelInterview handles POST /recruitment/interviews/:id/cancel (R16).
func (ctl *Controller) CancelInterview(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.interviews.Cancel(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

// CreateOffer handles POST /recruitment/offers (R17).
func (ctl *Controller) CreateOffer(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateOfferRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.offers.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListOffers handles GET /recruitment/offers?candidate_id (R18).
func (ctl *Controller) ListOffers(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	cand, okC := requiredQueryUUID(c, "candidate_id")
	if !okC {
		return
	}
	res, meta, err := ctl.offers.List(c.Request.Context(), a.TenantID, cand, page(c))
	respondList(c, res, meta, err)
}

// DecideOffer handles POST /recruitment/offers/:id/decision (R19).
func (ctl *Controller) DecideOffer(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.OfferDecisionRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.offers.Decide(c.Request.Context(), a, id, req.Decision)
	respond(c, http.StatusOK, res, err)
}

// WithdrawOffer handles POST /recruitment/offers/:id/withdraw (R20).
func (ctl *Controller) WithdrawOffer(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.offers.Withdraw(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}
