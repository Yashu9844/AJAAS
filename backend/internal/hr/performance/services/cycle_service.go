package services

import (
	"context"
	"errors"
	"sort"

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

// unrated is the 9-box box of employees without a submitted manager review (PM-015).
const unrated = "unrated"

// everything is the internal page used to read a whole cycle (bounded by the cycle's size).
var everything = dto.Page{Page: 1, PerPage: 100000}

// CycleService implements FR-CY (C1–C8).
type CycleService struct{ base }

// NewCycleService builds the service.
func NewCycleService(d Deps) *CycleService { return &CycleService{base{d}} }

// Create is C1 (FR-CY001, PM-001).
func (s *CycleService) Create(ctx context.Context, a Actor, req dto.CreateCycleRequest) (*dto.CycleResponse, error) {
	title, err := text("title", req.Title, maxCycleTitle)
	if err != nil {
		return nil, err
	}
	start, end, err := validators.DateRange(req.StartDate, req.EndDate)
	if err != nil {
		return nil, invalid("dates", err.Error())
	}
	criteria, err := validators.Criteria(req.Criteria)
	if err != nil {
		return nil, invalid("criteria", err.Error())
	}
	now := s.Now().UTC()
	c := &models.Cycle{TenantID: a.TenantID, Title: title, StartDate: start, EndDate: end, Criteria: criteria, Status: lifecycle.CycleDraft,
		CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.Repos.Cycles.Create(ctx, tx, c) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.cycle_created", "performance_cycle", c.ID.String(), map[string]string{"status": c.Status}}, nil)
	out := cycleResponse(c)
	return &out, nil
}

// Get is C3.
func (s *CycleService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CycleResponse, error) {
	c, err := s.Repos.Cycles.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	out := cycleResponse(c)
	return &out, nil
}

// List is C2.
func (s *CycleService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.CycleResponse, dto.PageMeta, error) {
	if status != "" && !lifecycle.ValidCycleStatus(status) {
		return nil, dto.PageMeta{}, invalid("status", "must be draft, active, review or completed")
	}
	rows, total, err := s.Repos.Cycles.List(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return cycleResponses(rows), page.Meta(total), nil
}

// Update is C4: title, dates and criteria, draft only (FR-CY002, PM-001).
func (s *CycleService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateCycleRequest) (*dto.CycleResponse, error) {
	var c *models.Cycle
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if c, err = s.lock(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if !lifecycle.CycleEditable(c.Status) {
			return ErrCycleState
		}
		if err := applyCycleUpdate(c, req); err != nil {
			return err
		}
		c.UpdatedAt = s.Now().UTC()
		return s.Repos.Cycles.Update(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.cycle_updated", "performance_cycle", c.ID.String(), map[string]string{"status": c.Status}}, nil)
	out := cycleResponse(c)
	return &out, nil
}

func applyCycleUpdate(c *models.Cycle, req dto.UpdateCycleRequest) error {
	if req.Title != nil {
		t, err := text("title", *req.Title, maxCycleTitle)
		if err != nil {
			return err
		}
		c.Title = t
	}
	start, end := c.StartDate.Format(validators.DateLayout), c.EndDate.Format(validators.DateLayout)
	if req.StartDate != nil {
		start = *req.StartDate
	}
	if req.EndDate != nil {
		end = *req.EndDate
	}
	s, e, err := validators.DateRange(start, end)
	if err != nil {
		return invalid("dates", err.Error())
	}
	c.StartDate, c.EndDate = s, e
	if req.Criteria != nil {
		if len(*req.Criteria) == 0 {
			return invalid("criteria", validators.ErrCriteria.Error())
		}
		cr, err := validators.Criteria(*req.Criteria)
		if err != nil {
			return invalid("criteria", err.Error())
		}
		c.Criteria = cr
	}
	return nil
}

// Activate is C5: draft → active (FR-CY003).
func (s *CycleService) Activate(ctx context.Context, a Actor, id uuid.UUID) (*dto.CycleResponse, error) {
	return s.transition(ctx, a, id, lifecycle.CycleActive, events.CycleStarted, nil)
}

// Complete is C7: review → completed; pending reviews expire (FR-CY005, PM-013).
func (s *CycleService) Complete(ctx context.Context, a Actor, id uuid.UUID) (*dto.CycleResponse, error) {
	return s.transition(ctx, a, id, lifecycle.CycleCompleted, events.CycleCompleted, func(tx *gorm.DB, c *models.Cycle) error {
		_, err := s.Repos.Reviews.ExpirePending(ctx, tx, c.TenantID, c.ID, s.Now().UTC())
		return err
	})
}

func (s *CycleService) transition(ctx context.Context, a Actor, id uuid.UUID, to, event string, extra func(*gorm.DB, *models.Cycle) error) (*dto.CycleResponse, error) {
	var c *models.Cycle
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if c, err = s.lock(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if !lifecycle.CanTransitionCycle(c.Status, to) {
			return ErrCycleState
		}
		if extra != nil {
			if err := extra(tx, c); err != nil {
				return err
			}
		}
		s.stamp(c, to)
		if err := s.Repos.Cycles.Update(ctx, tx, c); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, event, events.Payload{CycleID: c.ID, Status: c.Status})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.cycle_" + to, "performance_cycle", c.ID.String(), map[string]string{"status": c.Status}}, row)
	out := cycleResponse(c)
	return &out, nil
}

// stamp moves the cycle to status and records when.
func (s *CycleService) stamp(c *models.Cycle, status string) {
	now := s.Now().UTC()
	c.Status, c.UpdatedAt = status, now
	switch status {
	case lifecycle.CycleActive:
		c.ActivatedAt = &now
	case lifecycle.CycleReview:
		c.ReviewStartedAt = &now
	case lifecycle.CycleCompleted:
		c.CompletedAt = &now
	}
}

// LaunchReviews is C6 (FR-CY004, PM-010): active → review with a self review per goal owner and a manager review where
// the owner's manager has a profile. Idempotent per assignment: on a cycle already in review it only creates what is
// missing (EC-06: a second launch creates 0).
func (s *CycleService) LaunchReviews(ctx context.Context, a Actor, id uuid.UUID) (*dto.LaunchResponse, error) {
	out := &dto.LaunchResponse{}
	var c *models.Cycle
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		*out = dto.LaunchResponse{}
		var err error
		if c, err = s.lock(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		relaunch := c.Status == lifecycle.CycleReview
		if !relaunch && !lifecycle.CanTransitionCycle(c.Status, lifecycle.CycleReview) {
			return ErrCycleState
		}
		goals, err := s.Repos.Goals.Open(ctx, tx, a.TenantID, c.ID)
		if err != nil {
			return err
		}
		for _, owner := range owners(goals) {
			if err := s.launchFor(ctx, tx, c, owner, out); err != nil {
				return err
			}
		}
		if relaunch {
			return nil
		}
		s.stamp(c, lifecycle.CycleReview)
		return s.Repos.Cycles.Update(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.cycle_review", "performance_cycle", c.ID.String(),
		map[string]interface{}{"status": c.Status, "self_reviews_created": out.SelfReviews, "manager_reviews_created": out.ManagerReviews}}, nil)
	out.Cycle = cycleResponse(c)
	return out, nil
}

func owners(goals []models.Goal) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, g := range goals {
		if !seen[g.EmployeeID] {
			seen[g.EmployeeID] = true
			out = append(out, g.EmployeeID)
		}
	}
	return out
}

func (s *CycleService) launchFor(ctx context.Context, tx *gorm.DB, c *models.Cycle, owner uuid.UUID, out *dto.LaunchResponse) error {
	out.Employees++
	created, err := s.ensureReview(ctx, tx, c, owner, owner, models.ReviewSelf)
	if err != nil {
		return err
	}
	if created {
		out.SelfReviews++
	}
	emp, err := s.employee(ctx, c.TenantID, owner)
	if errors.Is(err, ErrEmployeeNotFound) {
		out.WithoutManager++
		return nil
	}
	if err != nil {
		return err
	}
	mgr, err := s.managerEmployee(ctx, c.TenantID, emp)
	if err != nil {
		return err
	}
	if mgr == nil || mgr.ID == owner {
		out.WithoutManager++
		return nil
	}
	if created, err = s.ensureReview(ctx, tx, c, owner, mgr.ID, models.ReviewManager); err == nil && created {
		out.ManagerReviews++
	}
	return err
}

func (s *CycleService) ensureReview(ctx context.Context, tx *gorm.DB, c *models.Cycle, employee, reviewer uuid.UUID, typ string) (bool, error) {
	exists, err := s.Repos.Reviews.Exists(ctx, tx, c.TenantID, c.ID, employee, reviewer, typ)
	if err != nil || exists {
		return false, err
	}
	now := s.Now().UTC()
	return true, s.Repos.Reviews.Create(ctx, tx, &models.Review{TenantID: c.TenantID, CycleID: c.ID, EmployeeID: employee, ReviewerEmployeeID: reviewer,
		Type: typ, Status: models.ReviewPending, Ratings: models.Ratings{}, CreatedAt: now, UpdatedAt: now})
}

// NineBox is C8 (PM-015, D7-04): performance band from the latest submitted manager review × its potential, with the
// goal score (PM-009) alongside; cycles in review or completed only.
func (s *CycleService) NineBox(ctx context.Context, tenantID, id uuid.UUID) (*dto.NineBoxResponse, error) {
	c, err := s.Repos.Cycles.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	if c.Status != lifecycle.CycleReview && c.Status != lifecycle.CycleCompleted {
		return nil, ErrCycleState
	}
	goals, err := s.Repos.Goals.Open(ctx, s.Tx.DB(), tenantID, c.ID)
	if err != nil {
		return nil, err
	}
	reviews, _, err := s.Repos.Reviews.List(ctx, s.Tx.DB(), tenantID,
		repositories.ReviewFilter{CycleID: &c.ID, Status: models.ReviewSubmitted, Type: models.ReviewManager}, everything)
	if err != nil {
		return nil, err
	}
	return nineBox(c.ID, goals, reviews), nil
}

func nineBox(cycleID uuid.UUID, goals []models.Goal, reviews []models.Review) *dto.NineBoxResponse {
	weights, progress := map[uuid.UUID][]score.Fixed{}, map[uuid.UUID][]score.Fixed{}
	for _, g := range goals {
		weights[g.EmployeeID] = append(weights[g.EmployeeID], g.Weight)
		progress[g.EmployeeID] = append(progress[g.EmployeeID], g.Progress)
	}
	latest := map[uuid.UUID]*models.Review{}
	for i := range reviews {
		r := &reviews[i]
		if cur, ok := latest[r.EmployeeID]; !ok || r.SubmittedAt.After(*cur.SubmittedAt) {
			latest[r.EmployeeID] = r
		}
	}
	ids := map[uuid.UUID]bool{}
	for id := range weights {
		ids[id] = true
	}
	for id := range latest {
		ids[id] = true
	}
	out := &dto.NineBoxResponse{CycleID: cycleID, Entries: []dto.NineBoxEntry{}, Counts: map[string]int{}}
	for id := range ids {
		e := dto.NineBoxEntry{EmployeeID: id, GoalScore: score.WeightedScore(weights[id], progress[id]), Box: unrated}
		if r := latest[id]; r != nil && r.OverallScore != nil && r.Potential != nil {
			e.OverallScore, e.Performance, e.Potential = r.OverallScore, score.Band(*r.OverallScore), *r.Potential
			e.Box = e.Performance + "/" + e.Potential
		}
		out.Counts[e.Box]++
		out.Entries = append(out.Entries, e)
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].EmployeeID.String() < out.Entries[j].EmployeeID.String() })
	return out
}

func (s *CycleService) lock(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error) {
	c, err := s.Repos.Cycles.LockByID(ctx, tx, tenantID, id)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	return c, nil
}
