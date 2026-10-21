package services

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/events"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// WorkerService implements worker history, the as-of org view, headcount and back-fill (CO-021, CO-022, CO-042,
// CO-050, CO-051).
type WorkerService struct{ base }

// NewWorkerService builds the service.
func NewWorkerService(d Deps) *WorkerService { return &WorkerService{base{d}} }

// Worker is CO-021.
func (s *WorkerService) Worker(ctx context.Context, tenantID, employeeID uuid.UUID, asOf string) (*dto.WorkerResponse, error) {
	d, err := s.asOf(asOf)
	if err != nil {
		return nil, err
	}
	if _, err := s.employee(ctx, tenantID, employeeID); err != nil {
		return nil, err
	}
	rows, err := s.Repo.WorkerAssignments(ctx, s.Tx.DB(), tenantID, employeeID)
	if err != nil {
		return nil, err
	}
	acts, _, err := s.Repo.Actions(ctx, s.Tx.DB(), tenantID, repositories.ActionFilter{EmployeeID: &employeeID}, everything)
	if err != nil {
		return nil, err
	}
	out := &dto.WorkerResponse{EmployeeID: employeeID, AsOf: fmtDate(d), History: make([]dto.AssignmentResponse, len(rows)), Actions: make([]dto.ActionResponse, len(acts))}
	for i := range rows {
		out.History[i] = assignmentResponse(&rows[i])
		if rows[i].CoversDate(d) {
			cur := out.History[i]
			out.Current = &cur
		}
	}
	for i := range acts {
		out.Actions[i] = *actionResponse(&acts[i])
	}
	return out, nil
}

// Me is CO-021 for the caller.
func (s *WorkerService) Me(ctx context.Context, a Actor, asOf string) (*dto.WorkerResponse, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	return s.Worker(ctx, a.TenantID, me.ID, asOf)
}

// Assignments is CO-022: who held what on a date.
func (s *WorkerService) Assignments(ctx context.Context, tenantID uuid.UUID, asOf string, f repositories.AssignmentFilter, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error) {
	d, err := s.asOf(asOf)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repo.AsOf(ctx, s.Tx.DB(), tenantID, d, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.AssignmentResponse, len(rows))
	for i := range rows {
		out[i] = assignmentResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// CreatePlan is CO-050.
func (s *WorkerService) CreatePlan(ctx context.Context, a Actor, req dto.CreatePlanRequest) (*dto.PlanResponse, error) {
	dept, err := parseUUIDField("department_id", req.DepartmentID)
	if err != nil {
		return nil, err
	}
	start, err := parseDate("period_start", req.PeriodStart)
	if err != nil {
		return nil, err
	}
	end, err := parseDate("period_end", req.PeriodEnd)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, invalid("period_end", "must not be before period_start")
	}
	if req.PlannedFTE < 0 {
		return nil, invalid("planned_fte", "must not be negative")
	}
	p := &models.HeadcountPlan{TenantID: a.TenantID, DepartmentID: dept, PeriodStart: start, PeriodEnd: end, PlannedFTE: req.PlannedFTE,
		CreatedByUserID: a.UserID, CreatedAt: s.Now().UTC()}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.refs(ctx, tx, a.TenantID, nil, &dept, nil); err != nil {
			return err
		}
		overlap, err := s.Repo.PlanOverlaps(ctx, tx, a.TenantID, dept, start, end)
		if err != nil {
			return err
		}
		if overlap {
			return ErrPlanOverlap
		}
		return s.Repo.Create(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.headcount_plan_created", "core_headcount_plan", p.ID.String(), map[string]string{"department_id": dept.String()}})
	out := planResponse(p)
	return &out, nil
}

// Plans lists headcount plans.
func (s *WorkerService) Plans(ctx context.Context, tenantID uuid.UUID, dept *uuid.UUID, page dto.Page) ([]dto.PlanResponse, dto.PageMeta, error) {
	rows, total, err := s.Repo.Plans(ctx, s.Tx.DB(), tenantID, dept, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.PlanResponse, len(rows))
	for i := range rows {
		out[i] = planResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Headcount is CO-051: per department, planned vs budgeted vs filled FTE, open positions and workers on a date.
func (s *WorkerService) Headcount(ctx context.Context, tenantID uuid.UUID, dept *uuid.UUID, asOf string) (*dto.HeadcountResponse, error) {
	d, err := s.asOf(asOf)
	if err != nil {
		return nil, err
	}
	db := s.Tx.DB()
	positions, _, err := s.Repo.Positions(ctx, db, tenantID, repositories.PositionFilter{DepartmentID: dept, Status: models.PositionActive}, everything)
	if err != nil {
		return nil, err
	}
	current, _, err := s.Repo.AsOf(ctx, db, tenantID, d, repositories.AssignmentFilter{DepartmentID: dept}, everything)
	if err != nil {
		return nil, err
	}
	plans, _, err := s.Repo.Plans(ctx, db, tenantID, dept, everything)
	if err != nil {
		return nil, err
	}
	rows := map[uuid.UUID]*dto.HeadcountRow{}
	row := func(id uuid.UUID) *dto.HeadcountRow {
		if rows[id] == nil {
			rows[id] = &dto.HeadcountRow{DepartmentID: id}
		}
		return rows[id]
	}
	filled := map[uuid.UUID]kernel.Fixed{}
	for _, as := range current {
		if as.PositionID != nil {
			filled[*as.PositionID] += as.FTE
		}
		if as.DepartmentID != nil {
			row(*as.DepartmentID).Workers++
		}
	}
	for _, p := range positions {
		r := row(p.DepartmentID)
		r.BudgetedFTE += p.BudgetedFTE
		r.FilledFTE += filled[p.ID]
		if filled[p.ID] < p.BudgetedFTE {
			r.OpenPositions++
		}
	}
	for _, p := range plans {
		if !p.PeriodStart.After(d) && !p.PeriodEnd.Before(d) {
			v := p.PlannedFTE
			row(p.DepartmentID).PlannedFTE = &v
		}
	}
	out := &dto.HeadcountResponse{AsOf: fmtDate(d), Departments: []dto.HeadcountRow{}}
	for _, r := range rows {
		out.Departments = append(out.Departments, *r)
	}
	sort.Slice(out.Departments, func(i, j int) bool {
		return out.Departments[i].DepartmentID.String() < out.Departments[j].DepartmentID.String()
	})
	return out, nil
}

// Backfill is CO-042: one initial assignment per worker without one, from their joining date and primary mapping,
// recorded as a completed hire with reason "backfill" (DC-08). Idempotent.
func (s *WorkerService) Backfill(ctx context.Context, a Actor) (*dto.RunResponse, error) {
	assigned, err := s.Repo.AssignedWorkers(ctx, s.Tx.DB(), a.TenantID)
	if err != nil {
		return nil, err
	}
	out := &dto.RunResponse{}
	for page := 1; ; page++ {
		emps, total, err := s.Employees.List(ctx, a.TenantID, employeeDTO.EmployeeFilter{Page: page, PerPage: 100})
		if err != nil {
			return out, err
		}
		for i := range emps {
			emp := &emps[i]
			if assigned[emp.ID] || emp.Status == "terminated" || emp.Status == "resigned" {
				out.Skipped++
				continue
			}
			if err := s.backfillOne(ctx, a, emp); err != nil {
				return out, err
			}
			out.Processed++
		}
		if int64(page*100) >= total || len(emps) == 0 {
			break
		}
	}
	s.afterCommit(ctx, a, auditEntry{"core.backfill", "core_hr_action", "", map[string]int{"processed": out.Processed, "skipped": out.Skipped}})
	return out, nil
}

func (s *WorkerService) backfillOne(ctx context.Context, a Actor, emp *employeeDTO.EmployeeResponse) error {
	dept, loc, mgrUser, err := s.Org.PrimaryPlacement(ctx, a.TenantID, emp.UserID)
	if err != nil {
		return err
	}
	var manager *uuid.UUID
	if mgrUser != nil {
		if m, err := s.Employees.GetByUserID(ctx, a.TenantID, *mgrUser); err == nil && m != nil && m.ID != emp.ID {
			manager = &m.ID
		}
	}
	from := dateOf(emp.CreatedAt)
	employment := "full_time"
	if e := emp.Employment; e != nil {
		from = dateOf(e.JoiningDate)
		if t := strings.ToLower(e.EmploymentType); lifecycle.EmploymentTypes[t] {
			employment = t
		}
	}
	now := s.Now().UTC()
	act := &models.Action{TenantID: a.TenantID, Type: lifecycle.Hire, EmployeeID: emp.ID, EffectiveDate: from, Status: lifecycle.Completed,
		ReasonCode: "backfill", InitiatedByUserID: a.UserID, DecidedByUserID: &a.UserID, SubmittedAt: &now, CompletedAt: &now,
		EffectsAppliedAt: &now, CreatedAt: now, UpdatedAt: now}
	return s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.Repo.LockWorker(ctx, tx, emp.ID); err != nil {
			return err
		}
		rows, err := s.Repo.WorkerAssignments(ctx, tx, a.TenantID, emp.ID)
		if err != nil || len(rows) > 0 {
			return err
		}
		if err := s.Repo.Create(ctx, tx, act); err != nil {
			return err
		}
		as := &models.Assignment{TenantID: a.TenantID, EmployeeID: emp.ID, ActionID: act.ID, DepartmentID: dept, LocationID: loc,
			ManagerEmployeeID: manager, WorkerType: "employee", EmploymentType: employment, FTE: kernel.One, EffectiveFrom: from,
			Status: models.AssignmentActive, CreatedAt: now, UpdatedAt: now}
		if err := s.Repo.Create(ctx, tx, as); err != nil {
			return err
		}
		act.NewAssignmentID = &as.ID
		return s.Repo.Save(ctx, tx, act)
	})
}

func assignmentResponse(a *models.Assignment) dto.AssignmentResponse {
	return dto.AssignmentResponse{ID: a.ID, EmployeeID: a.EmployeeID, ActionID: a.ActionID, JobProfileID: a.JobProfileID, PositionID: a.PositionID,
		DepartmentID: a.DepartmentID, LocationID: a.LocationID, ManagerEmployeeID: a.ManagerEmployeeID, GradeID: a.GradeID, WorkerType: a.WorkerType,
		EmploymentType: a.EmploymentType, FTE: a.FTE, ProbationEndDate: fmtDatePtr(a.ProbationEndDate), EffectiveFrom: fmtDate(a.EffectiveFrom),
		EffectiveTo: fmtDatePtr(a.EffectiveTo), Status: a.Status}
}

func planResponse(p *models.HeadcountPlan) dto.PlanResponse {
	return dto.PlanResponse{ID: p.ID, DepartmentID: p.DepartmentID, PeriodStart: fmtDate(p.PeriodStart), PeriodEnd: fmtDate(p.PeriodEnd),
		PlannedFTE: p.PlannedFTE, CreatedAt: p.CreatedAt}
}

// Relay tuning.
const (
	relayInterval    = 30 * time.Second
	relayBatchSize   = 100
	relayMaxAttempts = 10
)

// OutboxRelay republishes outbox rows the request path could not publish.
type OutboxRelay struct {
	base
	interval time.Duration
}

// NewOutboxRelay builds a relay over the context dependencies.
func NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }

// RelayOnce publishes one batch and returns how many rows were published.
func (r *OutboxRelay) RelayOnce(ctx context.Context) (int, error) {
	rows, err := r.Repo.FetchUnpublished(ctx, r.Tx.DB(), relayBatchSize, relayMaxAttempts)
	if err != nil {
		return 0, err
	}
	published := 0
	for i := range rows {
		row := &rows[i]
		if err := r.Publisher.Publish(ctx, events.Exchange, row.RoutingKey, json.RawMessage(row.Payload)); err != nil {
			_ = r.Repo.RecordFailure(ctx, r.Tx.DB(), row.ID, err.Error())
			continue
		}
		if err := r.Repo.MarkPublished(ctx, r.Tx.DB(), row.ID, r.Now().UTC()); err == nil {
			published++
		}
	}
	return published, nil
}

// Run ticks RelayOnce until ctx is cancelled.
func (r *OutboxRelay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = r.RelayOnce(ctx)
		}
	}
}
