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

// maxGoalDepth is PM-005: goal → parent → grandparent.
const maxGoalDepth = 3

// GoalService implements FR-GL (G1–G8).
type GoalService struct{ base }

// NewGoalService builds the service.
func NewGoalService(d Deps) *GoalService { return &GoalService{base{d}} }

// Create is G1 (FR-GL001, PM-003..PM-005, PM-008).
func (s *GoalService) Create(ctx context.Context, a Actor, req dto.CreateGoalRequest) (*dto.GoalResponse, error) {
	in, err := s.parseCreate(ctx, a, req)
	if err != nil {
		return nil, err
	}
	g := &models.Goal{TenantID: a.TenantID, CycleID: in.cycleID, EmployeeID: in.owner, ParentGoalID: in.parent, Title: in.title,
		Description: in.description, Weight: req.Weight, Status: lifecycle.GoalActive, CreatedByUserID: a.UserID}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.openCycle(ctx, tx, a.TenantID, in.cycleID, lifecycle.GoalsOpen); err != nil {
			return err
		}
		if err := s.checkParent(ctx, tx, a.TenantID, in.cycleID, in.parent); err != nil {
			return err
		}
		if err := s.checkWeight(ctx, tx, g, g.Weight); err != nil {
			return err
		}
		now := s.Now().UTC()
		g.CreatedAt, g.UpdatedAt = now, now
		return s.Repos.Goals.Create(ctx, tx, g)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.goal_created", "performance_goal", g.ID.String(), map[string]string{"cycle_id": g.CycleID.String(),
		"employee_id": g.EmployeeID.String()}}, nil)
	out := goalResponse(g)
	return &out, nil
}

type goalInput struct {
	cycleID, owner uuid.UUID
	parent         *uuid.UUID
	title          string
	description    *string
}

func (s *GoalService) parseCreate(ctx context.Context, a Actor, req dto.CreateGoalRequest) (*goalInput, error) {
	in := &goalInput{}
	var err error
	if in.cycleID, err = parseUUIDField("cycle_id", req.CycleID); err != nil {
		return nil, err
	}
	if in.parent, err = parseOptionalUUID("parent_goal_id", req.ParentGoalID); err != nil {
		return nil, err
	}
	if in.title, err = text("title", req.Title, maxGoalTitle); err != nil {
		return nil, err
	}
	if in.description, err = optionalText("description", req.Description, maxLongText); err != nil {
		return nil, err
	}
	if err := validators.Weight(req.Weight); err != nil {
		return nil, invalid("weight", err.Error())
	}
	owner, err := parseOptionalUUID("employee_id", req.EmployeeID)
	if err != nil {
		return nil, err
	}
	if owner == nil {
		self, err := s.employeeFor(ctx, a)
		if err != nil {
			return nil, err
		}
		owner = &self.ID
	}
	in.owner = *owner
	if ok, err := s.canAccessEmployee(ctx, a, in.owner, true); err != nil || !ok {
		return nil, orNotFound(err)
	}
	emp, err := s.employee(ctx, a.TenantID, in.owner)
	if err != nil {
		return nil, err
	}
	if !workingStatuses[emp.Status] {
		return nil, ErrEmployeeState
	}
	return in, nil
}

// openCycle locks the cycle and checks that its status allows the action.
func (s *GoalService) openCycle(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, open func(string) bool) error {
	c, err := s.Repos.Cycles.LockByID(ctx, tx, tenantID, id)
	if err != nil || c == nil {
		return orNotFound(err)
	}
	if !open(c.Status) {
		return ErrCycleState
	}
	return nil
}

// checkParent is PM-005: same tenant and cycle, not cancelled, chain depth ≤ 3.
func (s *GoalService) checkParent(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, parent *uuid.UUID) error {
	if parent == nil {
		return nil
	}
	p, err := s.Repos.Goals.FindByID(ctx, tx, tenantID, *parent)
	if err != nil || p == nil {
		return orNotFound(err)
	}
	if p.CycleID != cycleID || p.Status == lifecycle.GoalCancelled {
		return invalid("parent_goal_id", "must be a goal of the same cycle that is not cancelled")
	}
	depth := 2 // the new goal and its parent
	for cur := p; cur.ParentGoalID != nil; depth++ {
		if depth >= maxGoalDepth {
			return ErrGoalDepth
		}
		if cur, err = s.Repos.Goals.FindByID(ctx, tx, tenantID, *cur.ParentGoalID); err != nil || cur == nil {
			return orNotFound(err)
		}
	}
	return nil
}

// checkWeight is PM-004 under the (cycle, owner) advisory lock.
func (s *GoalService) checkWeight(ctx context.Context, tx *gorm.DB, g *models.Goal, weight score.Fixed) error {
	if err := s.Repos.Goals.LockOwner(ctx, tx, g.CycleID, g.EmployeeID); err != nil {
		return err
	}
	sum, err := s.Repos.Goals.WeightSum(ctx, tx, g.TenantID, g.CycleID, g.EmployeeID, g.ID)
	if err != nil {
		return err
	}
	if sum+weight > score.Hundred {
		return ErrWeightExceeded
	}
	return nil
}

// Get is G4 with key results and the newest check-ins (PM-008 read).
func (s *GoalService) Get(ctx context.Context, a Actor, id uuid.UUID) (*dto.GoalResponse, error) {
	g, err := s.Repos.Goals.FindByID(ctx, s.Tx.DB(), a.TenantID, id)
	if err != nil || g == nil {
		return nil, orNotFound(err)
	}
	if ok, err := s.canAccessEmployee(ctx, a, g.EmployeeID, false); err != nil || !ok {
		return nil, orNotFound(err)
	}
	return s.detail(ctx, g)
}

func (s *GoalService) detail(ctx context.Context, g *models.Goal) (*dto.GoalResponse, error) {
	krs, err := s.Repos.Goals.KeyResults(ctx, s.Tx.DB(), g.TenantID, g.ID)
	if err != nil {
		return nil, err
	}
	cis, err := s.Repos.Goals.CheckIns(ctx, s.Tx.DB(), g.TenantID, g.ID, checkInWindow)
	if err != nil {
		return nil, err
	}
	out := goalResponse(g)
	out.KeyResults = make([]dto.KeyResultResponse, len(krs))
	for i := range krs {
		out.KeyResults[i] = keyResultResponse(&krs[i])
	}
	out.CheckIns = make([]dto.CheckInResponse, len(cis))
	for i := range cis {
		out.CheckIns[i] = checkInResponse(&cis[i])
	}
	return &out, nil
}

// List is G2: any filter with performance:read; otherwise own goals or a direct report's (D7-05).
func (s *GoalService) List(ctx context.Context, a Actor, f repositories.GoalFilter, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error) {
	if f.Status != "" && f.Status != lifecycle.GoalActive && f.Status != lifecycle.GoalCompleted && f.Status != lifecycle.GoalCancelled {
		return nil, dto.PageMeta{}, invalid("status", "must be active, completed or cancelled")
	}
	if !a.CanRead && !a.CanManage {
		if f.EmployeeID == nil {
			self, err := s.employeeFor(ctx, a)
			if err != nil {
				return nil, dto.PageMeta{}, err
			}
			f.EmployeeID = &self.ID
		} else if ok, err := s.canAccessEmployee(ctx, a, *f.EmployeeID, false); err != nil || !ok {
			return nil, dto.PageMeta{}, orNotFound(err)
		}
	}
	return s.list(ctx, a, f, page)
}

// ListMine is G3.
func (s *GoalService) ListMine(ctx context.Context, a Actor, cycleID *uuid.UUID, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error) {
	self, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return s.list(ctx, a, repositories.GoalFilter{CycleID: cycleID, EmployeeID: &self.ID}, page)
}

func (s *GoalService) list(ctx context.Context, a Actor, f repositories.GoalFilter, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Goals.List(ctx, s.Tx.DB(), a.TenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return goalResponses(rows), page.Meta(total), nil
}

// Update is G5: title, description, weight while the cycle allows goal changes (FR-GL003, PM-003, PM-004).
func (s *GoalService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateGoalRequest) (*dto.GoalResponse, error) {
	g, err := s.mutate(ctx, a, id, func(tx *gorm.DB, g *models.Goal) (*models.OutboxEvent, error) {
		if err := s.openCycle(ctx, tx, a.TenantID, g.CycleID, lifecycle.GoalsOpen); err != nil {
			return nil, err
		}
		if g.Status == lifecycle.GoalCancelled {
			return nil, ErrGoalState
		}
		if err := applyGoalUpdate(g, req); err != nil {
			return nil, err
		}
		if req.Weight != nil {
			if err := s.checkWeight(ctx, tx, g, g.Weight); err != nil {
				return nil, err
			}
		}
		return nil, s.Repos.Goals.Update(ctx, tx, g)
	})
	if err != nil {
		return nil, err
	}
	out := goalResponse(g)
	return &out, nil
}

func applyGoalUpdate(g *models.Goal, req dto.UpdateGoalRequest) error {
	if req.Title != nil {
		t, err := text("title", *req.Title, maxGoalTitle)
		if err != nil {
			return err
		}
		g.Title = t
	}
	if req.Description != nil {
		d, err := optionalText("description", req.Description, maxLongText)
		if err != nil {
			return err
		}
		g.Description = d
	}
	if req.Weight != nil {
		if err := validators.Weight(*req.Weight); err != nil {
			return invalid("weight", err.Error())
		}
		g.Weight = *req.Weight
	}
	return nil
}

// SetStatus is G6 (FR-GL004).
func (s *GoalService) SetStatus(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.GoalResponse, error) {
	g, err := s.mutate(ctx, a, id, func(tx *gorm.DB, g *models.Goal) (*models.OutboxEvent, error) {
		if err := s.openCycle(ctx, tx, a.TenantID, g.CycleID, lifecycle.GoalsOpen); err != nil {
			return nil, err
		}
		if !lifecycle.CanTransitionGoal(g.Status, to) {
			return nil, ErrGoalState
		}
		g.Status = to
		if err := s.Repos.Goals.Update(ctx, tx, g); err != nil {
			return nil, err
		}
		return s.writeOutbox(ctx, tx, a, events.GoalUpdated, events.Payload{CycleID: g.CycleID, GoalID: &g.ID, EmployeeID: &g.EmployeeID,
			Status: g.Status, Progress: &g.Progress})
	})
	if err != nil {
		return nil, err
	}
	out := goalResponse(g)
	return &out, nil
}

// AddKeyResult is G7 (FR-GL005, PM-006, PM-007): the goal's progress becomes the mean of its key results.
func (s *GoalService) AddKeyResult(ctx context.Context, a Actor, id uuid.UUID, req dto.KeyResultRequest) (*dto.GoalResponse, error) {
	title, err := text("title", req.Title, maxGoalTitle)
	if err != nil {
		return nil, err
	}
	unit, err := optionalText("unit", req.Unit, maxUnit)
	if err != nil {
		return nil, err
	}
	if err := validators.KeyResult(req.StartValue, req.TargetValue); err != nil {
		return nil, invalid("target_value", err.Error())
	}
	g, err := s.mutate(ctx, a, id, func(tx *gorm.DB, g *models.Goal) (*models.OutboxEvent, error) {
		if err := s.openCycle(ctx, tx, a.TenantID, g.CycleID, lifecycle.GoalsOpen); err != nil {
			return nil, err
		}
		if g.Status != lifecycle.GoalActive {
			return nil, ErrGoalState
		}
		now := s.Now().UTC()
		kr := &models.KeyResult{TenantID: g.TenantID, GoalID: g.ID, Title: title, Unit: unit, StartValue: req.StartValue,
			TargetValue: req.TargetValue, CurrentValue: req.StartValue, CreatedAt: now, UpdatedAt: now}
		if err := s.Repos.Goals.AddKeyResult(ctx, tx, kr); err != nil {
			return nil, err
		}
		return s.recompute(ctx, tx, a, g, progressChange{keyResultID: &kr.ID})
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, g)
}

// CheckIn is G8 (FR-GL006, PM-006, PM-007).
func (s *GoalService) CheckIn(ctx context.Context, a Actor, id uuid.UUID, req dto.CheckInRequest) (*dto.GoalResponse, error) {
	note, err := optionalText("note", req.Note, maxNote)
	if err != nil {
		return nil, err
	}
	krID, err := parseOptionalUUID("key_result_id", req.KeyResultID)
	if err != nil {
		return nil, err
	}
	if (krID == nil) == (req.Progress == nil) || (krID != nil) != (req.Value != nil) {
		return nil, invalid("body", "send either progress, or key_result_id with value")
	}
	if req.Progress != nil {
		if err := validators.Progress(*req.Progress); err != nil {
			return nil, invalid("progress", err.Error())
		}
	}
	g, err := s.mutate(ctx, a, id, func(tx *gorm.DB, g *models.Goal) (*models.OutboxEvent, error) {
		if err := s.openCycle(ctx, tx, a.TenantID, g.CycleID, lifecycle.CheckInsOpen); err != nil {
			return nil, err
		}
		if g.Status != lifecycle.GoalActive {
			return nil, ErrGoalState
		}
		return s.applyCheckIn(ctx, tx, a, g, krID, req, note)
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, g)
}

func (s *GoalService) applyCheckIn(ctx context.Context, tx *gorm.DB, a Actor, g *models.Goal, krID *uuid.UUID, req dto.CheckInRequest, note *string) (*models.OutboxEvent, error) {
	krs, err := s.Repos.Goals.KeyResults(ctx, tx, g.TenantID, g.ID)
	if err != nil {
		return nil, err
	}
	if krID == nil {
		if len(krs) > 0 {
			return nil, ErrHasKeyResults
		}
		return s.recordProgress(ctx, tx, a, g, *req.Progress, progressChange{note: note})
	}
	for i := range krs {
		if krs[i].ID == *krID {
			krs[i].CurrentValue, krs[i].UpdatedAt = *req.Value, s.Now().UTC()
			if err := s.Repos.Goals.UpdateKeyResult(ctx, tx, &krs[i]); err != nil {
				return nil, err
			}
			return s.recompute(ctx, tx, a, g, progressChange{keyResultID: krID, value: req.Value, note: note})
		}
	}
	return nil, ErrNotFound
}

// progressChange describes what caused a progress update (recorded on the check-in).
type progressChange struct {
	keyResultID *uuid.UUID
	value       *score.Fixed
	note        *string
}

// recompute derives the goal's progress from all its key results (PM-006) and records the change.
func (s *GoalService) recompute(ctx context.Context, tx *gorm.DB, a Actor, g *models.Goal, ch progressChange) (*models.OutboxEvent, error) {
	krs, err := s.Repos.Goals.KeyResults(ctx, tx, g.TenantID, g.ID)
	if err != nil {
		return nil, err
	}
	ps := make([]score.Fixed, len(krs))
	for i, k := range krs {
		ps[i] = score.KRProgress(k.StartValue, k.TargetValue, k.CurrentValue)
	}
	return s.recordProgress(ctx, tx, a, g, score.Mean(ps), ch)
}

// recordProgress updates the goal and appends the check-in in the same transaction (PM-007, NFR-D001).
func (s *GoalService) recordProgress(ctx context.Context, tx *gorm.DB, a Actor, g *models.Goal, to score.Fixed, ch progressChange) (*models.OutboxEvent, error) {
	from := g.Progress
	g.Progress = to
	if err := s.Repos.Goals.Update(ctx, tx, g); err != nil {
		return nil, err
	}
	ci := &models.CheckIn{TenantID: g.TenantID, GoalID: g.ID, KeyResultID: ch.keyResultID, FromProgress: from, ToProgress: to, Value: ch.value,
		Note: ch.note, AuthorUserID: a.UserID, CreatedAt: s.Now().UTC()}
	if err := s.Repos.Goals.AddCheckIn(ctx, tx, ci); err != nil {
		return nil, err
	}
	return s.writeOutbox(ctx, tx, a, events.GoalUpdated, events.Payload{CycleID: g.CycleID, GoalID: &g.ID, EmployeeID: &g.EmployeeID,
		Status: g.Status, Progress: &g.Progress})
}

// mutate locks the goal, applies PM-008 write access (denial → 404) and runs fn in one transaction; the audit row and
// event follow the commit.
func (s *GoalService) mutate(ctx context.Context, a Actor, id uuid.UUID, fn func(*gorm.DB, *models.Goal) (*models.OutboxEvent, error)) (*models.Goal, error) {
	var g *models.Goal
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		g, err = s.Repos.Goals.LockByID(ctx, tx, a.TenantID, id)
		if err != nil || g == nil {
			return orNotFound(err)
		}
		if ok, err := s.canAccessEmployee(ctx, a, g.EmployeeID, true); err != nil || !ok {
			return orNotFound(err)
		}
		g.UpdatedAt = s.Now().UTC()
		row, err = fn(tx, g)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"performance.goal_updated", "performance_goal", g.ID.String(), map[string]string{"status": g.Status,
		"progress": g.Progress.String()}}, row)
	return g, nil
}
