package services

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/events"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// Budgeted FTE bounds (CO-010).
const maxBudget = kernel.Fixed(9999)

// PositionService implements positions and occupancy (CO-010..CO-014).
type PositionService struct{ base }

// NewPositionService builds the service.
func NewPositionService(d Deps) *PositionService { return &PositionService{base{d}} }

// Create is CO-010.
func (s *PositionService) Create(ctx context.Context, a Actor, req dto.CreatePositionRequest) (*dto.PositionResponse, error) {
	c, err := code(req.Code)
	if err != nil {
		return nil, err
	}
	title, err := text("title", req.Title, maxName)
	if err != nil {
		return nil, err
	}
	profile, err := parseUUIDField("job_profile_id", req.JobProfileID)
	if err != nil {
		return nil, err
	}
	dept, err := parseUUIDField("department_id", req.DepartmentID)
	if err != nil {
		return nil, err
	}
	loc, err := parseOptionalUUID("location_id", req.LocationID)
	if err != nil {
		return nil, err
	}
	budget := kernel.One
	if req.BudgetedFTE != nil {
		budget = *req.BudgetedFTE
	}
	if budget <= 0 || budget > maxBudget {
		return nil, invalid("budgeted_fte", "must be 0.01–99.99")
	}
	now := s.Now().UTC()
	p := &models.Position{TenantID: a.TenantID, Code: c, Title: title, JobProfileID: profile, DepartmentID: dept, LocationID: loc, BudgetedFTE: budget,
		Shared: req.Shared, Status: models.PositionActive, CreatedAt: now, UpdatedAt: now}
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.refs(ctx, tx, a.TenantID, &profile, &dept, loc); err != nil {
			return err
		}
		taken, err := s.Repo.CodeTaken(ctx, tx, &models.Position{}, a.TenantID, c, uuid.Nil)
		if err != nil {
			return err
		}
		if taken {
			return ErrCodeTaken
		}
		if err := s.Repo.Create(ctx, tx, p); err != nil {
			return err
		}
		row, err = s.changed(ctx, tx, a, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.position_created", "core_position", p.ID.String(), map[string]string{"code": p.Code}}, row)
	return s.response(ctx, s.Tx.DB(), p, s.today())
}

// refs checks a job profile (active), a department and a location (exist and active) — CO-003, CO-010.
func (b base) refs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, profile, dept, loc *uuid.UUID) error {
	if profile != nil {
		p, err := b.Repo.Profile(ctx, db, tenantID, *profile)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		if p.Status != models.StatusActive {
			return ErrInactiveReference
		}
	}
	if dept != nil {
		exists, active, err := b.Org.DepartmentState(ctx, tenantID, *dept)
		if err != nil || !exists {
			return orNotFound(err)
		}
		if !active {
			return ErrInactiveReference
		}
	}
	if loc != nil {
		exists, active, err := b.Org.LocationState(ctx, tenantID, *loc)
		if err != nil || !exists {
			return orNotFound(err)
		}
		if !active {
			return ErrInactiveReference
		}
	}
	return nil
}

func (b base) changed(ctx context.Context, tx *gorm.DB, a Actor, p *models.Position) (*models.OutboxEvent, error) {
	return b.writeOutbox(ctx, tx, a, events.PositionChanged, events.PositionPayload{PositionID: p.ID, DepartmentID: p.DepartmentID, Status: p.Status})
}

// Get is CO-011 for one position.
func (s *PositionService) Get(ctx context.Context, tenantID, id uuid.UUID, asOf string) (*dto.PositionResponse, error) {
	d, err := s.asOf(asOf)
	if err != nil {
		return nil, err
	}
	p, err := s.Repo.Position(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	return s.response(ctx, s.Tx.DB(), p, d)
}

// List is CO-011 with filters; vacant filters on occupancy as of the date.
func (s *PositionService) List(ctx context.Context, tenantID uuid.UUID, f repositories.PositionFilter, vacant *bool, asOf string, page dto.Page) ([]dto.PositionResponse, dto.PageMeta, error) {
	d, err := s.asOf(asOf)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	if f.Status != "" && f.Status != models.PositionActive && f.Status != models.PositionFrozen && f.Status != models.PositionClosed {
		return nil, dto.PageMeta{}, invalid("status", "must be active, frozen or closed")
	}
	if vacant != nil {
		rows, _, err := s.Repo.Positions(ctx, s.Tx.DB(), tenantID, f, everything)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		var all []dto.PositionResponse
		for i := range rows {
			r, err := s.response(ctx, s.Tx.DB(), &rows[i], d)
			if err != nil {
				return nil, dto.PageMeta{}, err
			}
			if r.Vacant == *vacant {
				all = append(all, *r)
			}
		}
		start, end := min(page.Offset(), len(all)), min(page.Offset()+page.PerPage, len(all))
		return append([]dto.PositionResponse{}, all[start:end]...), page.Meta(int64(len(all))), nil
	}
	rows, total, err := s.Repo.Positions(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.PositionResponse, len(rows))
	for i := range rows {
		r, err := s.response(ctx, s.Tx.DB(), &rows[i], d)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		out[i] = *r
	}
	return out, page.Meta(total), nil
}

// Update is CO-012: the budget never drops below the current or any future filled FTE.
func (s *PositionService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdatePositionRequest) (*dto.PositionResponse, error) {
	var p *models.Position
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.Repo.LockPosition(ctx, tx, id); err != nil {
			return err
		}
		var err error
		if p, err = s.Repo.Position(ctx, tx, a.TenantID, id); err != nil || p == nil {
			return orNotFound(err)
		}
		if err := s.patch(ctx, tx, a.TenantID, p, req); err != nil {
			return err
		}
		peak, count, err := s.peak(ctx, tx, a.TenantID, p.ID, s.today(), nil, uuid.Nil)
		if err != nil {
			return err
		}
		if peak > p.BudgetedFTE || (!p.Shared && count > 1) {
			return ErrPositionOverfill
		}
		p.UpdatedAt = s.Now().UTC()
		if err := s.Repo.Save(ctx, tx, p); err != nil {
			return err
		}
		row, err = s.changed(ctx, tx, a, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.position_updated", "core_position", p.ID.String(), map[string]string{"status": p.Status}}, row)
	return s.response(ctx, s.Tx.DB(), p, s.today())
}

func (s *PositionService) patch(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, p *models.Position, req dto.UpdatePositionRequest) error {
	if req.Title != nil {
		t, err := text("title", *req.Title, maxName)
		if err != nil {
			return err
		}
		p.Title = t
	}
	profile, err := parseOptionalUUID("job_profile_id", req.JobProfileID)
	if err != nil {
		return err
	}
	dept, err := parseOptionalUUID("department_id", req.DepartmentID)
	if err != nil {
		return err
	}
	loc, err := parseOptionalUUID("location_id", req.LocationID)
	if err != nil {
		return err
	}
	if err := s.refs(ctx, tx, tenantID, profile, dept, loc); err != nil {
		return err
	}
	if profile != nil {
		p.JobProfileID = *profile
	}
	if dept != nil {
		p.DepartmentID = *dept
	}
	if loc != nil {
		p.LocationID = loc
	}
	if req.BudgetedFTE != nil {
		if *req.BudgetedFTE <= 0 || *req.BudgetedFTE > maxBudget {
			return invalid("budgeted_fte", "must be 0.01–99.99")
		}
		p.BudgetedFTE = *req.BudgetedFTE
	}
	if req.Shared != nil {
		p.Shared = *req.Shared
	}
	return nil
}

// SetStatus is CO-013.
func (s *PositionService) SetStatus(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.PositionResponse, error) {
	var p *models.Position
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.Repo.LockPosition(ctx, tx, id); err != nil {
			return err
		}
		var err error
		if p, err = s.Repo.Position(ctx, tx, a.TenantID, id); err != nil || p == nil {
			return orNotFound(err)
		}
		if !lifecycle.CanMovePosition(p.Status, to) {
			return ErrPositionState
		}
		if to == models.PositionClosed {
			incumbents, err := s.Repo.PositionAssignments(ctx, tx, a.TenantID, p.ID, s.today(), nil)
			if err != nil {
				return err
			}
			if len(incumbents) > 0 {
				return ErrPositionOccupied
			}
		}
		p.Status, p.UpdatedAt = to, s.Now().UTC()
		if err := s.Repo.Save(ctx, tx, p); err != nil {
			return err
		}
		row, err = s.changed(ctx, tx, a, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"core.position_" + to, "core_position", p.ID.String(), map[string]string{"status": p.Status}}, row)
	return s.response(ctx, s.Tx.DB(), p, s.today())
}

// peak returns the highest filled FTE and incumbent count of a position over [from, to), ignoring one assignment.
func (b base) peak(ctx context.Context, db *gorm.DB, tenantID, positionID uuid.UUID, from time.Time, to *time.Time, exclude uuid.UUID) (kernel.Fixed, int, error) {
	rows, err := b.Repo.PositionAssignments(ctx, db, tenantID, positionID, from, to)
	if err != nil {
		return 0, 0, err
	}
	var others []models.Assignment
	for _, r := range rows {
		if r.ID != exclude {
			others = append(others, r)
		}
	}
	dates := []time.Time{from}
	for _, r := range others {
		if r.EffectiveFrom.After(from) {
			dates = append(dates, r.EffectiveFrom)
		}
	}
	var peak kernel.Fixed
	peakCount := 0
	for _, d := range dates {
		var sum kernel.Fixed
		n := 0
		for i := range others {
			if others[i].CoversDate(d) {
				sum += others[i].FTE
				n++
			}
		}
		if sum > peak {
			peak = sum
		}
		if n > peakCount {
			peakCount = n
		}
	}
	return peak, peakCount, nil
}

// checkSeat is CO-014 for an assignment about to be written: the position is active and has room on every date.
func (b base) checkSeat(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, as *models.Assignment, exclude uuid.UUID) error {
	if as.PositionID == nil {
		return nil
	}
	if err := b.Repo.LockPosition(ctx, tx, *as.PositionID); err != nil {
		return err
	}
	p, err := b.Repo.Position(ctx, tx, tenantID, *as.PositionID)
	if err != nil || p == nil {
		return orNotFound(err)
	}
	if p.Status != models.PositionActive {
		return ErrPositionState
	}
	peak, count, err := b.peak(ctx, tx, tenantID, p.ID, as.EffectiveFrom, as.EffectiveTo, exclude)
	if err != nil {
		return err
	}
	if (!p.Shared && count > 0) || peak+as.FTE > p.BudgetedFTE {
		return ErrPositionFilled
	}
	return nil
}

// response maps a position with its occupancy on date d (CO-011).
func (b base) response(ctx context.Context, db *gorm.DB, p *models.Position, d time.Time) (*dto.PositionResponse, error) {
	rows, err := b.Repo.PositionAssignments(ctx, db, p.TenantID, p.ID, d, nil)
	if err != nil {
		return nil, err
	}
	out := &dto.PositionResponse{ID: p.ID, Code: p.Code, Title: p.Title, JobProfileID: p.JobProfileID, DepartmentID: p.DepartmentID,
		LocationID: p.LocationID, BudgetedFTE: p.BudgetedFTE, Shared: p.Shared, Status: p.Status, AsOf: fmtDate(d), Incumbents: []uuid.UUID{},
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	for i := range rows {
		if rows[i].CoversDate(d) {
			out.FilledFTE += rows[i].FTE
			out.Incumbents = append(out.Incumbents, rows[i].EmployeeID)
		}
	}
	sort.Slice(out.Incumbents, func(i, j int) bool { return out.Incumbents[i].String() < out.Incumbents[j].String() })
	out.OpenFTE = p.BudgetedFTE - out.FilledFTE
	if out.OpenFTE < 0 {
		out.OpenFTE = 0
	}
	out.Vacant = len(out.Incumbents) == 0
	return out, nil
}
