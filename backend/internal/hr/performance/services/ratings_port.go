package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/hr/performance/score"
)

// RatingsPort implements performance/api.Ratings from the final ratings (PM-020).
type RatingsPort struct{ base }

// NewRatingsPort builds the port.
func NewRatingsPort(d Deps) *RatingsPort { return &RatingsPort{base{d}} }

// FinalBands returns each rated worker's performance band in the cycle.
func (p *RatingsPort) FinalBands(ctx context.Context, tenantID, cycleID uuid.UUID) (map[uuid.UUID]string, error) {
	c, err := p.Repos.Cycles.FindByID(ctx, p.Tx.DB(), tenantID, cycleID)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	reviews, _, err := p.Repos.Reviews.List(ctx, p.Tx.DB(), tenantID,
		repositories.ReviewFilter{CycleID: &cycleID, Status: models.ReviewSubmitted, Type: models.ReviewManager}, everything)
	if err != nil {
		return nil, err
	}
	cals, err := p.Repos.Calibrations.LatestPerEmployee(ctx, p.Tx.DB(), tenantID, cycleID)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID]string{}
	for id, f := range finalRatings(reviews, cals) {
		out[id] = score.Band(f.OverallScore)
	}
	return out, nil
}
