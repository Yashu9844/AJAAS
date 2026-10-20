package services

import (
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/score"
	"github.com/jaas/jaas/internal/hr/performance/validators"
)

func cycleResponse(c *models.Cycle) dto.CycleResponse {
	return dto.CycleResponse{ID: c.ID, Title: c.Title, StartDate: c.StartDate.Format(validators.DateLayout), EndDate: c.EndDate.Format(validators.DateLayout),
		Criteria: append([]string{}, c.Criteria...), Status: c.Status, ActivatedAt: c.ActivatedAt, ReviewStartedAt: c.ReviewStartedAt,
		CompletedAt: c.CompletedAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

func cycleResponses(rows []models.Cycle) []dto.CycleResponse {
	out := make([]dto.CycleResponse, len(rows))
	for i := range rows {
		out[i] = cycleResponse(&rows[i])
	}
	return out
}

func goalResponse(g *models.Goal) dto.GoalResponse {
	return dto.GoalResponse{ID: g.ID, CycleID: g.CycleID, EmployeeID: g.EmployeeID, ParentGoalID: g.ParentGoalID, Title: g.Title,
		Description: g.Description, Weight: g.Weight, Progress: g.Progress, Status: g.Status, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
}

func goalResponses(rows []models.Goal) []dto.GoalResponse {
	out := make([]dto.GoalResponse, len(rows))
	for i := range rows {
		out[i] = goalResponse(&rows[i])
	}
	return out
}

func keyResultResponse(k *models.KeyResult) dto.KeyResultResponse {
	return dto.KeyResultResponse{ID: k.ID, Title: k.Title, Unit: k.Unit, StartValue: k.StartValue, TargetValue: k.TargetValue,
		CurrentValue: k.CurrentValue, Progress: score.KRProgress(k.StartValue, k.TargetValue, k.CurrentValue)}
}

func checkInResponse(c *models.CheckIn) dto.CheckInResponse {
	return dto.CheckInResponse{ID: c.ID, KeyResultID: c.KeyResultID, FromProgress: c.FromProgress, ToProgress: c.ToProgress, Value: c.Value,
		Note: c.Note, AuthorUserID: c.AuthorUserID, CreatedAt: c.CreatedAt}
}

// reviewResponse maps a review; anonymous hides the reviewer of peer reviews (PM-014).
func reviewResponse(r *models.Review, anonymous bool) dto.ReviewResponse {
	out := dto.ReviewResponse{ID: r.ID, CycleID: r.CycleID, EmployeeID: r.EmployeeID, Type: r.Type, Status: r.Status,
		Ratings: append(models.Ratings{}, r.Ratings...), Strengths: r.Strengths, Improvements: r.Improvements, OverallScore: r.OverallScore,
		Potential: r.Potential, SubmittedAt: r.SubmittedAt, CreatedAt: r.CreatedAt}
	if !(anonymous && r.Type == models.ReviewPeer) {
		out.ReviewerEmployeeID = uuidPtr(r.ReviewerEmployeeID)
	}
	return out
}

func reviewResponses(rows []models.Review, anonymous bool) []dto.ReviewResponse {
	out := make([]dto.ReviewResponse, len(rows))
	for i := range rows {
		out[i] = reviewResponse(&rows[i], anonymous)
	}
	return out
}
