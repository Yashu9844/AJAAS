package compensation

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var (
	ratingBands = map[string]bool{"low": true, "medium": true, "high": true, "unrated": true}
	compaBands  = map[string]bool{"below": true, "within": true, "above": true, "unknown": true}
	cycleMoves  = map[string]string{CycleDraft: CycleOpen, CycleOpen: CycleInReview, CycleInReview: CycleApproved, CycleApproved: CycleApplied}
)

// CycleInput creates or edits a draft merit cycle (CO-C20).
type CycleInput struct {
	Name               *string     `json:"name"`
	EffectiveDate      *string     `json:"effective_date"`
	PerformanceCycleID *string     `json:"performance_cycle_id"`
	Guidelines         *Guidelines `json:"guidelines"`
}

func (in CycleInput) apply(c *Cycle) error {
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len([]rune(n)) > 150 {
			return invalid("name", "must be 1–150 characters")
		}
		c.Name = n
	}
	if in.EffectiveDate != nil {
		d, err := parseDate("effective_date", *in.EffectiveDate)
		if err != nil {
			return err
		}
		c.EffectiveDate = d
	}
	if in.PerformanceCycleID != nil {
		if *in.PerformanceCycleID == "" {
			c.PerformanceCycleID = nil
		} else {
			id, err := uuid.Parse(*in.PerformanceCycleID)
			if err != nil {
				return invalid("performance_cycle_id", "must be a UUID")
			}
			c.PerformanceCycleID = &id
		}
	}
	if in.Guidelines != nil {
		seen := map[string]bool{}
		for _, g := range *in.Guidelines {
			key := g.Rating + "/" + g.Compa
			if !ratingBands[g.Rating] || !compaBands[g.Compa] || seen[key] || g.MinPct < 0 || g.MinPct > g.MaxPct || g.MaxPct > maxMeritPct {
				return invalid("guidelines", "each cell needs a rating (low|medium|high|unrated), a compa band (below|within|above|unknown), 0 ≤ min ≤ max ≤ 50 and no duplicates")
			}
			seen[key] = true
		}
		c.Guidelines = *in.Guidelines
	}
	return nil
}

// CreateCycle is CO-C20.
func (s *Service) CreateCycle(ctx context.Context, a Actor, in CycleInput) (*Cycle, error) {
	if in.Name == nil || in.EffectiveDate == nil {
		return nil, invalid("body", "name and effective_date are required")
	}
	now := s.now().UTC()
	c := &Cycle{TenantID: a.TenantID, Status: CycleDraft, Guidelines: Guidelines{}, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := in.apply(c); err != nil {
		return nil, err
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, c) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "compensation.cycle_created", "comp_merit_cycle", c.ID.String(), nil)
	return c, nil
}

// UpdateCycle edits a draft (CO-C20).
func (s *Service) UpdateCycle(ctx context.Context, a Actor, id uuid.UUID, in CycleInput) (*Cycle, error) {
	return s.withCycle(ctx, a, id, "updated", func(tx *gorm.DB, c *Cycle) error {
		if c.Status != CycleDraft {
			return ErrCycleState
		}
		return in.apply(c)
	})
}

// GetCycle returns one cycle.
func (s *Service) GetCycle(ctx context.Context, a Actor, id uuid.UUID) (*Cycle, error) {
	c, err := s.store.Cycle(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrNotFound
	}
	return c, nil
}

// Cycles lists merit cycles.
func (s *Service) Cycles(ctx context.Context, a Actor, status string, page, perPage int) ([]Cycle, int64, error) {
	if _, ok := cycleMoves[status]; status != "" && !ok && status != CycleApplied {
		return nil, 0, invalid("status", "must be draft, open, in_review, approved or applied")
	}
	return s.store.Cycles(ctx, s.store.DB(), a.TenantID, status, page, perPage)
}

// BudgetInput is one department budget.
type BudgetInput struct {
	DepartmentID string       `json:"department_id"`
	Amount       kernel.Money `json:"amount"`
}

// SetBudgets replaces the cycle's budgets (CO-C21, draft only).
func (s *Service) SetBudgets(ctx context.Context, a Actor, id uuid.UUID, in []BudgetInput) ([]Budget, error) {
	rows := make([]Budget, 0, len(in))
	seen := map[uuid.UUID]bool{}
	for _, b := range in {
		dept, err := uuid.Parse(b.DepartmentID)
		if err != nil || seen[dept] || b.Amount < 0 {
			return nil, invalid("budgets", "each needs a distinct department_id and an amount ≥ 0")
		}
		seen[dept] = true
		rows = append(rows, Budget{TenantID: a.TenantID, CycleID: id, DepartmentID: dept, Amount: b.Amount})
	}
	if _, err := s.withCycle(ctx, a, id, "budgets_set", func(tx *gorm.DB, c *Cycle) error {
		if c.Status != CycleDraft {
			return ErrCycleState
		}
		return s.store.ReplaceBudgets(ctx, tx, a.TenantID, id, rows)
	}); err != nil {
		return nil, err
	}
	return s.Budgets(ctx, a, id)
}

// Budgets lists the cycle's budgets.
func (s *Service) Budgets(ctx context.Context, a Actor, id uuid.UUID) ([]Budget, error) {
	if _, err := s.GetCycle(ctx, a, id); err != nil {
		return nil, err
	}
	return s.store.Budgets(ctx, s.store.DB(), a.TenantID, id)
}

// Move advances the cycle (CO-C22, CO-C25): open snapshots proposals; approve needs a user other than the opener;
// apply writes the records and syncs payroll.
func (s *Service) Move(ctx context.Context, a Actor, id uuid.UUID, to string) (*Cycle, error) {
	return s.withCycle(ctx, a, id, to, func(tx *gorm.DB, c *Cycle) error {
		if cycleMoves[c.Status] != to {
			return ErrCycleState
		}
		c.Status = to
		now := s.now().UTC()
		switch to {
		case CycleOpen:
			c.OpenedByUserID, c.OpenedAt = &a.UserID, &now
			return s.snapshot(ctx, tx, a, c)
		case CycleApproved:
			if c.OpenedByUserID != nil && *c.OpenedByUserID == a.UserID {
				return ErrSelfApproval
			}
			c.ApprovedByUserID, c.ApprovedAt = &a.UserID, &now
		case CycleApplied:
			c.AppliedAt = &now
			return s.applyCycle(ctx, tx, a, c)
		}
		return nil
	})
}

func (s *Service) withCycle(ctx context.Context, a Actor, id uuid.UUID, verb string, fn func(*gorm.DB, *Cycle) error) (*Cycle, error) {
	var c *Cycle
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if c, err = s.store.Cycle(ctx, tx, a.TenantID, id, true); err != nil {
			return err
		}
		if c == nil {
			return ErrNotFound
		}
		if err := fn(tx, c); err != nil {
			return err
		}
		c.UpdatedAt = s.now().UTC()
		return s.store.Save(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "compensation.cycle_"+verb, "comp_merit_cycle", c.ID.String(), map[string]string{"status": c.Status})
	return c, nil
}

// snapshot is CO-C22: one proposal per worker with a current record and a department on the cycle date.
func (s *Service) snapshot(ctx context.Context, tx *gorm.DB, a Actor, c *Cycle) error {
	ratings := map[uuid.UUID]string{}
	if c.PerformanceCycleID != nil {
		var err error
		if ratings, err = s.p.Ratings.FinalBands(ctx, a.TenantID, *c.PerformanceCycleID); err != nil {
			return err
		}
	}
	on := c.EffectiveDate.Time
	for page := 1; ; page++ {
		emps, total, err := s.p.Employees.List(ctx, a.TenantID, employeeDTO.EmployeeFilter{Page: page, PerPage: 100})
		if err != nil {
			return err
		}
		for _, e := range emps {
			if err := s.propose(ctx, tx, a, c, e.ID, on, ratings); err != nil {
				return err
			}
		}
		if int64(page*100) >= total || len(emps) == 0 {
			return nil
		}
	}
}

func (s *Service) propose(ctx context.Context, tx *gorm.DB, a Actor, c *Cycle, emp uuid.UUID, on time.Time, ratings map[uuid.UUID]string) error {
	rows, err := s.store.Records(ctx, tx, a.TenantID, emp)
	if err != nil {
		return err
	}
	rec := current(rows, on)
	pl, err := s.p.Core.PlacementOn(ctx, a.TenantID, emp, on)
	if err != nil || rec == nil || pl == nil || pl.DepartmentID == nil {
		return err
	}
	pos, err := s.position(ctx, tx, a.TenantID, emp, rec.AnnualBase, on)
	if err != nil {
		return err
	}
	band := ratings[emp]
	if band == "" {
		band = "unrated"
	}
	now := s.now().UTC()
	return s.store.Create(ctx, tx, &Proposal{TenantID: a.TenantID, CycleID: c.ID, EmployeeProfileID: emp, DepartmentID: *pl.DepartmentID,
		ManagerEmployeeID: pl.ManagerEmployeeID, CurrentBase: rec.AnnualBase, CompaRatio: pos.CompaRatio, CompaBand: pos.CompaBand, RatingBand: band,
		CreatedAt: now, UpdatedAt: now})
}

// applyCycle is CO-C25: one record per proposal with an increase, effective on the cycle date.
func (s *Service) applyCycle(ctx context.Context, tx *gorm.DB, a Actor, c *Cycle) error {
	props, err := s.store.Proposals(ctx, tx, a.TenantID, c.ID, nil)
	if err != nil {
		return err
	}
	cid := c.ID
	for _, p := range props {
		if p.IncreasePct <= 0 {
			continue
		}
		if err := s.applyRecord(ctx, tx, a, &Record{TenantID: a.TenantID, EmployeeProfileID: p.EmployeeProfileID, AnnualBase: p.CurrentBase + p.Amount,
			EffectiveFrom: c.EffectiveDate, Reason: ReasonMerit, MeritCycleID: &cid}); err != nil {
			return err
		}
	}
	return s.store.Event(ctx, tx, a.TenantID, "hr.compensation.merit_applied", map[string]string{"cycle_id": c.ID.String(),
		"effective_date": c.EffectiveDate.Format(DateLayout)})
}

// self resolves the caller's employee profile, or nil.
func (s *Service) self(ctx context.Context, a Actor) (*uuid.UUID, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &me.ID, nil
}

// Proposals is CO-C26: read/manage see all; others only proposals they manage, only while open or in review.
func (s *Service) Proposals(ctx context.Context, a Actor, cycleID uuid.UUID, mine bool) ([]Proposal, error) {
	c, err := s.GetCycle(ctx, a, cycleID)
	if err != nil {
		return nil, err
	}
	var manager *uuid.UUID
	if mine || (!a.CanRead && !a.CanManage) {
		if c.Status != CycleOpen && c.Status != CycleInReview {
			return []Proposal{}, nil
		}
		if manager, err = s.self(ctx, a); err != nil {
			return nil, err
		}
		if manager == nil {
			return []Proposal{}, nil
		}
	}
	rows, err := s.store.Proposals(ctx, s.store.DB(), a.TenantID, cycleID, manager)
	if rows == nil {
		rows = []Proposal{}
	}
	return rows, err
}

// ProposalInput updates a proposal (CO-C23, CO-C24).
type ProposalInput struct {
	IncreasePct    kernel.Fixed `json:"increase_pct"`
	Justification  *string      `json:"justification"`
	OverrideBudget bool         `json:"override_budget"`
}

// UpdateProposal is CO-C23/CO-C24: the worker's manager or compensation:manage, while the cycle is open.
func (s *Service) UpdateProposal(ctx context.Context, a Actor, id uuid.UUID, in ProposalInput) (*Proposal, error) {
	if in.IncreasePct < 0 || in.IncreasePct > maxMeritPct {
		return nil, invalid("increase_pct", "must be 0–50")
	}
	just, err := optional("justification", in.Justification, 1000)
	if err != nil {
		return nil, err
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	var p *Proposal
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if p, err = s.store.Proposal(ctx, tx, a.TenantID, id, false); err != nil {
			return err
		}
		if p == nil || (!a.CanManage && (me == nil || p.ManagerEmployeeID == nil || *p.ManagerEmployeeID != *me)) {
			return ErrNotFound
		}
		c, err := s.store.Cycle(ctx, tx, a.TenantID, p.CycleID, true)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		if c.Status != CycleOpen {
			return ErrCycleState
		}
		if p, err = s.store.Proposal(ctx, tx, a.TenantID, id, true); err != nil {
			return err
		}
		p.IncreasePct, p.Justification, p.OverrideBudget = in.IncreasePct, just, in.OverrideBudget
		p.Amount = p.CurrentBase.Percent(kernel.Money(in.IncreasePct)).RoundRupee()
		p.OutsideGuideline = outside(c.Guidelines, p)
		if (p.OutsideGuideline || p.OverrideBudget) && p.Justification == nil {
			return invalid("justification", "is required outside the guideline or over budget")
		}
		if err := s.checkBudget(ctx, tx, a, p); err != nil {
			return err
		}
		now := s.now().UTC()
		p.UpdatedByUserID, p.UpdatedAt = &a.UserID, now
		return s.store.Save(ctx, tx, p)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "compensation.proposal_updated", "comp_merit_proposal", p.ID.String(), map[string]string{"cycle_id": p.CycleID.String()})
	return p, nil
}

// outside reports whether a proposal's percentage falls outside its guideline cell (no cell = no limit).
func outside(gs Guidelines, p *Proposal) bool {
	for _, g := range gs {
		if g.Rating == p.RatingBand && g.Compa == p.CompaBand {
			return p.IncreasePct < g.MinPct || p.IncreasePct > g.MaxPct
		}
	}
	return false
}

// checkBudget is CO-C24 under the cycle lock: Σ amounts of the department ≤ budget unless overridden.
func (s *Service) checkBudget(ctx context.Context, tx *gorm.DB, a Actor, p *Proposal) error {
	if p.OverrideBudget || p.Amount == 0 {
		return nil
	}
	usage, err := s.usage(ctx, tx, a.TenantID, p.CycleID, p)
	if err != nil {
		return err
	}
	if u := usage[p.DepartmentID]; u.Used > u.Budget {
		return ErrBudget
	}
	return nil
}

// Usage is a department's live budget usage (CO-C24).
type Usage struct {
	DepartmentID uuid.UUID    `json:"department_id"`
	Budget       kernel.Money `json:"budget"`
	Used         kernel.Money `json:"used"`
	Remaining    kernel.Money `json:"remaining"`
	Proposals    int          `json:"proposals"`
}

// usage sums amounts per department; replace substitutes one proposal's new values.
func (s *Service) usage(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID, replace *Proposal) (map[uuid.UUID]*Usage, error) {
	budgets, err := s.store.Budgets(ctx, db, tenantID, cycleID)
	if err != nil {
		return nil, err
	}
	props, err := s.store.Proposals(ctx, db, tenantID, cycleID, nil)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID]*Usage{}
	get := func(d uuid.UUID) *Usage {
		if out[d] == nil {
			out[d] = &Usage{DepartmentID: d}
		}
		return out[d]
	}
	for _, b := range budgets {
		get(b.DepartmentID).Budget = b.Amount
	}
	for _, p := range props {
		if replace != nil && p.ID == replace.ID {
			p = *replace
		}
		u := get(p.DepartmentID)
		u.Used += p.Amount
		u.Proposals++
	}
	for _, u := range out {
		u.Remaining = u.Budget - u.Used
	}
	return out, nil
}

// Usage is CO-C24 for HR.
func (s *Service) Usage(ctx context.Context, a Actor, cycleID uuid.UUID) ([]Usage, error) {
	if _, err := s.GetCycle(ctx, a, cycleID); err != nil {
		return nil, err
	}
	m, err := s.usage(ctx, s.store.DB(), a.TenantID, cycleID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Usage, 0, len(m))
	for _, u := range m {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DepartmentID.String() < out[j].DepartmentID.String() })
	return out, nil
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}
