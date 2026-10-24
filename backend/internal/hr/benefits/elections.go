package benefits

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	payrollAPI "github.com/jaas/jaas/internal/hr/payroll/api"
	"gorm.io/gorm"
)

// WindowState says whether the caller may elect now and why (BN-C04).
type WindowState struct {
	Open   bool   `json:"open"`
	Reason string `json:"reason,omitempty"`
}

// Option is one plan offered to the caller, with the version in force and their live election.
type Option struct {
	Plan     Plan         `json:"plan"`
	Version  *PlanVersion `json:"version"`
	Election *Election    `json:"election,omitempty"`
}

// Options is GET /options.
type Options struct {
	Window WindowState `json:"window"`
	Plans  []Option    `json:"plans"`
}

// ElectInput creates an election.
type ElectInput struct {
	PlanID        string   `json:"plan_id"`
	LevelCode     string   `json:"level_code"`
	DependantIDs  []string `json:"dependant_ids"`
	EffectiveFrom *string  `json:"effective_from"`
}

// ChangeInput changes the level or dependants of an election.
type ChangeInput struct {
	LevelCode     string   `json:"level_code"`
	DependantIDs  []string `json:"dependant_ids"`
	EffectiveFrom *string  `json:"effective_from"`
}

// DependantInput creates or patches a dependant.
type DependantInput struct {
	Name         *string `json:"name"`
	Relationship *string `json:"relationship"`
	DateOfBirth  *string `json:"date_of_birth"`
}

// LifeEventInput records a life event; employee_id needs benefits:manage.
type LifeEventInput struct {
	Type        string  `json:"type"`
	EventDate   string  `json:"event_date"`
	EvidenceRef *string `json:"evidence_ref"`
	EmployeeID  *string `json:"employee_id"`
}

// ReportRow is one plan and level of the enrolment report (BN-C10); no personal data.
type ReportRow struct {
	PlanID       uuid.UUID    `json:"plan_id"`
	PlanCode     string       `json:"plan_code"`
	PlanName     string       `json:"plan_name"`
	LevelCode    string       `json:"level_code,omitempty"`
	Active       int          `json:"active"`
	Waived       int          `json:"waived"`
	EmployeeCost kernel.Money `json:"employee_cost"`
	EmployerCost kernel.Money `json:"employer_cost"`
}

var relationships = map[string]bool{"spouse": true, "child": true, "parent": true, "other": true}
var lifeTypes = map[string]bool{"marriage": true, "birth": true, "adoption": true, "divorce": true, "death": true, "other": true}

func versionOn(vs []PlanVersion, on time.Time) *PlanVersion {
	for i := range vs {
		if vs[i].In(on) {
			return &vs[i]
		}
	}
	return nil
}

func hasStr(list []string, v string) bool {
	for _, x := range list {
		if x == strings.ToLower(v) {
			return true
		}
	}
	return false
}

func hasID(list []uuid.UUID, v *uuid.UUID) bool {
	if v == nil {
		return false
	}
	for _, x := range list {
		if x == *v {
			return true
		}
	}
	return false
}

// eligible is BN-C03: every non-empty restriction must be met.
func eligible(e Eligibility, emp *employeeDTO.EmployeeResponse, pl *coreAPI.Placement, on time.Time) bool {
	if len(e.WorkerTypes) > 0 && (pl == nil || !hasStr(e.WorkerTypes, pl.WorkerType)) {
		return false
	}
	if len(e.GradeIDs) > 0 && (pl == nil || !hasID(e.GradeIDs, pl.GradeID)) {
		return false
	}
	if len(e.LocationIDs) > 0 && (pl == nil || !hasID(e.LocationIDs, pl.LocationID)) {
		return false
	}
	if len(e.EmploymentTypes) > 0 && (emp.Employment == nil || !hasStr(e.EmploymentTypes, emp.Employment.EmploymentType)) {
		return false
	}
	if e.MinTenureDays > 0 {
		if emp.Employment == nil {
			return false
		}
		if int(on.Sub(midnight(emp.Employment.JoiningDate)).Hours()/24) < e.MinTenureDays {
			return false
		}
	}
	return true
}

// windowFor is BN-C04: which enrolment window, if any, is open for the worker on the date.
func (s *Service) windowFor(ctx context.Context, emp *employeeDTO.EmployeeResponse, on Date) (string, error) {
	open, err := s.store.OpenWindowOn(ctx, s.store.DB(), emp.TenantID, on)
	if err != nil {
		return "", err
	}
	if open {
		return WindowOpenEnrolment, nil
	}
	st, err := s.settings(ctx, emp.TenantID)
	if err != nil {
		return "", err
	}
	if emp.Employment != nil {
		joined := midnight(emp.Employment.JoiningDate)
		if !on.Before(joined) && !on.After(joined.AddDate(0, 0, st.NewHireDays)) {
			return WindowNewHire, nil
		}
	}
	ev, err := s.store.LifeEventSince(ctx, s.store.DB(), emp.TenantID, emp.ID, Date{on.AddDate(0, 0, -st.LifeEventDays)}, on)
	if err != nil {
		return "", err
	}
	if ev != nil {
		return WindowLifeEvent, nil
	}
	return "", nil
}

func (s *Service) requireWindow(ctx context.Context, emp *employeeDTO.EmployeeResponse, on Date) error {
	reason, err := s.windowFor(ctx, emp, on)
	if err != nil {
		return err
	}
	if reason == "" {
		return ErrWindowClosed
	}
	return nil
}

// Options is GET /options: the plans offered to the caller today and whether they may elect.
func (s *Service) Options(ctx context.Context, a Actor) (*Options, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	today := s.today()
	reason, err := s.windowFor(ctx, emp, today)
	if err != nil {
		return nil, err
	}
	plans, err := s.store.ActivePlans(ctx, s.store.DB(), a.TenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(plans))
	for _, p := range plans {
		ids = append(ids, p.ID)
	}
	versions, err := s.store.Versions(ctx, s.store.DB(), a.TenantID, ids)
	if err != nil {
		return nil, err
	}
	byPlan := map[uuid.UUID][]PlanVersion{}
	for _, v := range versions {
		byPlan[v.PlanID] = append(byPlan[v.PlanID], v)
	}
	pl, err := s.p.Core.PlacementOn(ctx, a.TenantID, emp.ID, today.Time)
	if err != nil {
		return nil, err
	}
	out := &Options{Window: WindowState{Open: reason != "", Reason: reason}, Plans: []Option{}}
	for _, p := range plans {
		v := versionOn(byPlan[p.ID], today.Time)
		if v == nil || !eligible(p.Eligibility, emp, pl, today.Time) {
			continue
		}
		live, err := s.store.LiveElection(ctx, s.store.DB(), a.TenantID, emp.ID, p.ID, false)
		if err != nil {
			return nil, err
		}
		out.Plans = append(out.Plans, Option{Plan: p, Version: v, Election: live})
	}
	return out, nil
}

// choice validates a level and dependants for a plan on the start date and returns the covered dependant ids (BN-C05).
func (s *Service) choice(ctx context.Context, emp *employeeDTO.EmployeeResponse, p *Plan, levelCode string, depIDs []string, from Date) (IDs, error) {
	vs, err := s.store.Versions(ctx, s.store.DB(), p.TenantID, []uuid.UUID{p.ID})
	if err != nil {
		return nil, err
	}
	v := versionOn(vs, from.Time)
	if v == nil {
		return nil, invalid("effective_from", "the plan has no cost version in force on that date")
	}
	level, ok := v.Level(strings.TrimSpace(levelCode))
	if !ok {
		return nil, invalid("level_code", "unknown coverage level")
	}
	pl, err := s.p.Core.PlacementOn(ctx, p.TenantID, emp.ID, from.Time)
	if err != nil {
		return nil, err
	}
	if !eligible(p.Eligibility, emp, pl, from.Time) {
		return nil, ErrNotEligible
	}
	ids := IDs{}
	if len(depIDs) > 0 {
		own, err := s.store.Dependants(ctx, s.store.DB(), p.TenantID, emp.ID)
		if err != nil {
			return nil, err
		}
		have := map[uuid.UUID]bool{}
		for _, d := range own {
			have[d.ID] = true
		}
		seen := map[uuid.UUID]bool{}
		for _, raw := range depIDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil {
				return nil, invalid("dependant_ids", "must be UUIDs")
			}
			if !have[id] {
				return nil, invalid("dependant_ids", "only your own active dependants can be covered")
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) > level.MaxDependants {
		return nil, invalid("dependant_ids", fmt.Sprintf("this level covers at most %d dependants", level.MaxDependants))
	}
	return ids, nil
}

func (s *Service) startDate(raw *string) (Date, error) {
	today := s.today()
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return today, nil
	}
	d, err := parseDate("effective_from", *raw)
	if err != nil {
		return Date{}, err
	}
	if d.Before(today.Time) || d.After(today.AddDate(0, 0, maxStartAhead)) {
		return Date{}, invalid("effective_from", "must be today or up to 90 days ahead")
	}
	return d, nil
}

func (s *Service) activePlan(ctx context.Context, tenantID, id uuid.UUID) (*Plan, error) {
	p, err := s.store.Plan(ctx, s.store.DB(), tenantID, id, false)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	if p.Status != PlanActive {
		return nil, ErrPlanState
	}
	return p, nil
}

func end(e *Election, on Date, reason string, now time.Time) {
	e.Status, e.EffectiveTo, e.EndReason, e.UpdatedAt = ElEnded, &on, &reason, now
}

// Elect is BN-C05: a new election for a plan inside an open window.
func (s *Service) Elect(ctx context.Context, a Actor, in ElectInput) (*Election, error) {
	planID, err := uuid.Parse(strings.TrimSpace(in.PlanID))
	if err != nil {
		return nil, invalid("plan_id", "must be a UUID")
	}
	from, err := s.startDate(in.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	if err := s.requireWindow(ctx, emp, s.today()); err != nil {
		return nil, err
	}
	plan, err := s.activePlan(ctx, a.TenantID, planID)
	if err != nil {
		return nil, err
	}
	deps, err := s.choice(ctx, emp, plan, in.LevelCode, in.DependantIDs, from)
	if err != nil {
		return nil, err
	}
	level := strings.TrimSpace(in.LevelCode)
	var out *Election
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Lock(ctx, tx, "elect:"+emp.ID.String()+":"+planID.String()); err != nil {
			return err
		}
		live, err := s.store.LiveElection(ctx, tx, a.TenantID, emp.ID, planID, true)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if live != nil {
			if live.Status == ElActive {
				return ErrElectionExists
			}
			end(live, from, ReasonReplaced, now) // a waiver is replaced by the new election
			if err := s.store.Save(ctx, tx, live); err != nil {
				return err
			}
		}
		out = &Election{ID: uuid.New(), TenantID: a.TenantID, EmployeeProfileID: emp.ID, PlanID: planID, LevelCode: &level, DependantIDs: deps, Status: ElActive,
			EffectiveFrom: from, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
		if err := s.store.Create(ctx, tx, out); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "benefits.election.created", map[string]string{"election_id": out.ID.String(), "plan_id": planID.String()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.election.create", "benefit_election", out.ID.String(), map[string]string{"plan_id": planID.String()})
	return out, nil
}

// Change is BN-C06: end the current election on the new one's start date and write the new one.
func (s *Service) Change(ctx context.Context, a Actor, id uuid.UUID, in ChangeInput) (*Election, error) {
	from, err := s.startDate(in.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	if err := s.requireWindow(ctx, emp, s.today()); err != nil {
		return nil, err
	}
	var out *Election
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		old, err := s.store.Election(ctx, tx, a.TenantID, id, true)
		if err != nil || old == nil || old.EmployeeProfileID != emp.ID {
			return orNotFound(err)
		}
		if old.Status != ElActive || old.EffectiveTo != nil {
			return ErrElectionState
		}
		if from.Before(old.EffectiveFrom.Time) {
			return invalid("effective_from", "must not be before the current election starts")
		}
		plan, err := s.activePlan(ctx, a.TenantID, old.PlanID)
		if err != nil {
			return err
		}
		deps, err := s.choice(ctx, emp, plan, in.LevelCode, in.DependantIDs, from)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		end(old, from, ReasonChanged, now)
		if err := s.store.Save(ctx, tx, old); err != nil {
			return err
		}
		level := strings.TrimSpace(in.LevelCode)
		out = &Election{ID: uuid.New(), TenantID: a.TenantID, EmployeeProfileID: emp.ID, PlanID: old.PlanID, LevelCode: &level, DependantIDs: deps, Status: ElActive,
			EffectiveFrom: from, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
		if err := s.store.Create(ctx, tx, out); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "benefits.election.created", map[string]string{"election_id": out.ID.String(), "plan_id": old.PlanID.String()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.election.change", "benefit_election", id.String(), map[string]string{"new_election_id": out.ID.String()})
	return out, nil
}

// Waive is BN-C06: an explicit waived election from today; any active election of the plan ends today. It is idempotent.
func (s *Service) Waive(ctx context.Context, a Actor, planIDRaw string) (*Election, error) {
	planID, err := uuid.Parse(strings.TrimSpace(planIDRaw))
	if err != nil {
		return nil, invalid("plan_id", "must be a UUID")
	}
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	today := s.today()
	if err := s.requireWindow(ctx, emp, today); err != nil {
		return nil, err
	}
	if _, err := s.activePlan(ctx, a.TenantID, planID); err != nil {
		return nil, err
	}
	var out *Election
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Lock(ctx, tx, "elect:"+emp.ID.String()+":"+planID.String()); err != nil {
			return err
		}
		live, err := s.store.LiveElection(ctx, tx, a.TenantID, emp.ID, planID, true)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if live != nil {
			if live.Status == ElWaived {
				out = live
				return nil
			}
			end(live, today, ReasonWaived, now)
			if err := s.store.Save(ctx, tx, live); err != nil {
				return err
			}
		}
		out = &Election{ID: uuid.New(), TenantID: a.TenantID, EmployeeProfileID: emp.ID, PlanID: planID, DependantIDs: IDs{}, Status: ElWaived, EffectiveFrom: today,
			CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
		if err := s.store.Create(ctx, tx, out); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "benefits.election.waived", map[string]string{"election_id": out.ID.String(), "plan_id": planID.String()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.election.waive", "benefit_election", out.ID.String(), map[string]string{"plan_id": planID.String()})
	return out, nil
}

// End is BN-C06: end an election today (a future one is cancelled). The owner needs an open window; benefits:manage
// may end anyone's election at any time.
func (s *Service) End(ctx context.Context, a Actor, id uuid.UUID) (*Election, error) {
	var out *Election
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		e, err := s.store.Election(ctx, tx, a.TenantID, id, true)
		if err != nil || e == nil {
			return orNotFound(err)
		}
		if !a.CanManage {
			emp, err := s.me(ctx, a)
			if err != nil {
				return err
			}
			if e.EmployeeProfileID != emp.ID {
				return ErrNotFound
			}
			if err := s.requireWindow(ctx, emp, s.today()); err != nil {
				return err
			}
		}
		if e.Status != ElActive || e.EffectiveTo != nil {
			return ErrElectionState
		}
		on := s.today()
		if e.EffectiveFrom.After(on.Time) {
			on = e.EffectiveFrom
		}
		end(e, on, ReasonEnded, s.now().UTC())
		out = e
		if err := s.store.Save(ctx, tx, e); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "benefits.election.ended", map[string]string{"election_id": id.String(), "reason": ReasonEnded})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.election.end", "benefit_election", id.String(), nil)
	return out, nil
}

// MyElections lists the caller's elections, history included.
func (s *Service) MyElections(ctx context.Context, a Actor) ([]Election, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.EmployeeElections(ctx, s.store.DB(), a.TenantID, emp.ID)
	if rows == nil {
		rows = []Election{}
	}
	return rows, err
}

// Elections lists anyone's elections (benefits:read_sensitive).
func (s *Service) Elections(ctx context.Context, a Actor, f ElectionFilter, page, perPage int) ([]Election, int64, error) {
	switch f.Status {
	case "", ElActive, ElWaived, ElEnded:
	default:
		return nil, 0, invalid("status", "must be active, waived or ended")
	}
	return s.store.Elections(ctx, s.store.DB(), a.TenantID, f, page, perPage)
}

// --- payroll hand-off (BN-C07) ---

// Deductions implements payroll's BenefitDeductions: the employee cost of each election in force on the period's
// last day, at the level's cost in the version in force that day. A level missing from that version is skipped.
func (s *Service) Deductions(ctx context.Context, tenantID, employeeID uuid.UUID, _, to time.Time) ([]payrollAPI.BenefitLine, error) {
	on := Date{midnight(to)}
	els, err := s.store.ElectionsOn(ctx, s.store.DB(), tenantID, employeeID, on)
	if err != nil || len(els) == 0 {
		return nil, err
	}
	var out []payrollAPI.BenefitLine
	for _, e := range els {
		p, err := s.store.Plan(ctx, s.store.DB(), tenantID, e.PlanID, false)
		if err != nil || p == nil || e.LevelCode == nil {
			if err != nil {
				return nil, err
			}
			continue
		}
		vs, err := s.store.Versions(ctx, s.store.DB(), tenantID, []uuid.UUID{p.ID})
		if err != nil {
			return nil, err
		}
		v := versionOn(vs, on.Time)
		if v == nil {
			continue
		}
		level, ok := v.Level(*e.LevelCode)
		if !ok || level.EmployeeCost == 0 {
			continue
		}
		out = append(out, payrollAPI.BenefitLine{Code: "BEN_" + strings.ReplaceAll(p.Code, "-", "_"), Name: p.Name, Amount: level.EmployeeCost})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// MyDeductions previews the caller's deductions for a month ("YYYY-MM", default the current month).
func (s *Service) MyDeductions(ctx context.Context, a Actor, month string) ([]payrollAPI.BenefitLine, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	first := time.Date(s.now().UTC().Year(), s.now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	if strings.TrimSpace(month) != "" {
		first, err = time.Parse("2006-01", strings.TrimSpace(month))
		if err != nil {
			return nil, invalid("month", "must be YYYY-MM")
		}
	}
	rows, err := s.Deductions(ctx, a.TenantID, emp.ID, first, first.AddDate(0, 1, -1))
	if rows == nil {
		rows = []payrollAPI.BenefitLine{}
	}
	return rows, err
}

// CoreHook is BN-C08: a termination ends the worker's elections the day after their last working day; rescinding it
// reinstates exactly those. An error rolls the action back, because a missed end would keep deducting.
func (s *Service) CoreHook(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, fact coreAPI.ActionFact) error {
	if fact.Type != "terminate" {
		return nil
	}
	if fact.Rescinded {
		_, err := s.store.Reinstate(ctx, tx, tenantID, fact.ActionID)
		return err
	}
	last := fact.EffectiveDate
	if fact.LastWorkingDay != nil {
		last = *fact.LastWorkingDay
	}
	n, err := s.store.EndForTermination(ctx, tx, tenantID, fact.EmployeeID, Date{midnight(last).AddDate(0, 0, 1)}, fact.ActionID)
	if err != nil || n == 0 {
		return err
	}
	return s.store.Event(ctx, tx, tenantID, "benefits.election.ended", map[string]string{"employee_id": fact.EmployeeID.String(), "reason": ReasonTermination})
}

// --- reports (BN-C10) ---

// EnrolmentReport counts active elections and waivers per plan and level today, with the monthly costs they imply.
func (s *Service) EnrolmentReport(ctx context.Context, a Actor) ([]ReportRow, error) {
	today := s.today()
	counts, err := s.store.Counts(ctx, s.store.DB(), a.TenantID, today)
	if err != nil {
		return nil, err
	}
	rows := []ReportRow{}
	if len(counts) == 0 {
		return rows, nil
	}
	ids := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, c := range counts {
		if !seen[c.PlanID] {
			seen[c.PlanID] = true
			ids = append(ids, c.PlanID)
		}
	}
	versions, err := s.store.Versions(ctx, s.store.DB(), a.TenantID, ids)
	if err != nil {
		return nil, err
	}
	byPlan := map[uuid.UUID][]PlanVersion{}
	for _, v := range versions {
		byPlan[v.PlanID] = append(byPlan[v.PlanID], v)
	}
	plans := map[uuid.UUID]*Plan{}
	for _, id := range ids {
		p, err := s.store.Plan(ctx, s.store.DB(), a.TenantID, id, false)
		if err != nil || p == nil {
			if err != nil {
				return nil, err
			}
			continue
		}
		plans[id] = p
	}
	byKey := map[string]*ReportRow{}
	for _, c := range counts {
		p := plans[c.PlanID]
		if p == nil {
			continue
		}
		level := ""
		if c.LevelCode != nil {
			level = *c.LevelCode
		}
		key := c.PlanID.String() + "/" + level
		r := byKey[key]
		if r == nil {
			r = &ReportRow{PlanID: c.PlanID, PlanCode: p.Code, PlanName: p.Name, LevelCode: level}
			byKey[key] = r
		}
		if c.Status == ElWaived {
			r.Waived += c.N
			continue
		}
		r.Active += c.N
		if v := versionOn(byPlan[c.PlanID], today.Time); v != nil {
			if l, ok := v.Level(level); ok {
				r.EmployeeCost += l.EmployeeCost * kernel.Money(c.N)
				r.EmployerCost += l.EmployerCost * kernel.Money(c.N)
			}
		}
	}
	for _, r := range byKey {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PlanCode != rows[j].PlanCode {
			return rows[i].PlanCode < rows[j].PlanCode
		}
		return rows[i].LevelCode < rows[j].LevelCode
	})
	return rows, nil
}

// --- dependants and life events: sensitive (BN-C09) ---

func (s *Service) dependantFields(d *Dependant, in DependantInput) error {
	if in.Name != nil {
		n, err := required("name", *in.Name, 100)
		if err != nil {
			return err
		}
		d.Name = n
	}
	if in.Relationship != nil {
		if !relationships[*in.Relationship] {
			return invalid("relationship", "must be spouse, child, parent or other")
		}
		d.Relationship = *in.Relationship
	}
	if in.DateOfBirth != nil {
		dob, err := parseDate("date_of_birth", *in.DateOfBirth)
		if err != nil {
			return err
		}
		if dob.After(s.today().Time) || dob.Year() < 1900 {
			return invalid("date_of_birth", "must be between 1900 and today")
		}
		d.DateOfBirth = dob
	}
	return nil
}

// CreateDependant adds a dependant to the caller.
func (s *Service) CreateDependant(ctx context.Context, a Actor, in DependantInput) (*Dependant, error) {
	if in.Name == nil || in.Relationship == nil || in.DateOfBirth == nil {
		return nil, invalid("name", "name, relationship and date_of_birth are required")
	}
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	d := &Dependant{TenantID: a.TenantID, EmployeeProfileID: emp.ID, Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := s.dependantFields(d, in); err != nil {
		return nil, err
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, d) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.dependant.create", "benefit_dependant", d.ID.String(), nil)
	return d, nil
}

// UpdateDependant patches the caller's own dependant.
func (s *Service) UpdateDependant(ctx context.Context, a Actor, id uuid.UUID, in DependantInput) (*Dependant, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	var out *Dependant
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		d, err := s.store.Dependant(ctx, tx, a.TenantID, id, true)
		if err != nil || d == nil || d.EmployeeProfileID != emp.ID || d.Status != "active" {
			return orNotFound(err)
		}
		if err := s.dependantFields(d, in); err != nil {
			return err
		}
		d.UpdatedAt = s.now().UTC()
		out = d
		return s.store.Save(ctx, tx, d)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.dependant.update", "benefit_dependant", id.String(), nil)
	return out, nil
}

// RemoveDependant retires a dependant unless a live election covers them.
func (s *Service) RemoveDependant(ctx context.Context, a Actor, id uuid.UUID) (*Dependant, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	var out *Dependant
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		d, err := s.store.Dependant(ctx, tx, a.TenantID, id, true)
		if err != nil || d == nil || d.EmployeeProfileID != emp.ID {
			return orNotFound(err)
		}
		out = d
		if d.Status == "removed" {
			return nil
		}
		els, err := s.store.ElectionsOn(ctx, tx, a.TenantID, emp.ID, s.today())
		if err != nil {
			return err
		}
		for _, e := range els {
			for _, covered := range e.DependantIDs {
				if covered == id {
					return ErrDependantInUse
				}
			}
		}
		d.Status, d.UpdatedAt = "removed", s.now().UTC()
		return s.store.Save(ctx, tx, d)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.dependant.remove", "benefit_dependant", id.String(), nil)
	return out, nil
}

// MyDependants lists the caller's active dependants.
func (s *Service) MyDependants(ctx context.Context, a Actor) ([]Dependant, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Dependants(ctx, s.store.DB(), a.TenantID, emp.ID)
	if rows == nil {
		rows = []Dependant{}
	}
	return rows, err
}

// EmployeeDependants lists a worker's dependants (benefits:read_sensitive).
func (s *Service) EmployeeDependants(ctx context.Context, a Actor, employeeID uuid.UUID) ([]Dependant, error) {
	if _, err := s.worker(ctx, a.TenantID, employeeID); err != nil {
		return nil, err
	}
	rows, err := s.store.Dependants(ctx, s.store.DB(), a.TenantID, employeeID)
	if rows == nil {
		rows = []Dependant{}
	}
	return rows, err
}

// RecordLifeEvent records a life event for the caller, or for a worker with benefits:manage.
func (s *Service) RecordLifeEvent(ctx context.Context, a Actor, in LifeEventInput) (*LifeEvent, error) {
	if !lifeTypes[in.Type] {
		return nil, invalid("type", "must be marriage, birth, adoption, divorce, death or other")
	}
	date, err := parseDate("event_date", in.EventDate)
	if err != nil {
		return nil, err
	}
	if date.After(s.today().AddDate(0, 0, maxStartAhead)) || date.Year() < 1900 {
		return nil, invalid("event_date", "must not be more than 90 days in the future")
	}
	ref, err := optional("evidence_ref", in.EvidenceRef, 300)
	if err != nil {
		return nil, err
	}
	var emp *employeeDTO.EmployeeResponse
	if in.EmployeeID != nil && strings.TrimSpace(*in.EmployeeID) != "" {
		if !a.CanManage {
			return nil, ErrForbidden
		}
		id, perr := uuid.Parse(strings.TrimSpace(*in.EmployeeID))
		if perr != nil {
			return nil, invalid("employee_id", "must be a UUID")
		}
		emp, err = s.worker(ctx, a.TenantID, id)
	} else {
		emp, err = s.me(ctx, a)
	}
	if err != nil {
		return nil, err
	}
	ev := &LifeEvent{TenantID: a.TenantID, EmployeeProfileID: emp.ID, EventType: in.Type, EventDate: date, EvidenceRef: ref, RecordedByUserID: a.UserID, CreatedAt: s.now().UTC()}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, ev) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "benefits.life_event.record", "benefit_life_event", ev.ID.String(), nil)
	return ev, nil
}

// MyLifeEvents lists the caller's life events.
func (s *Service) MyLifeEvents(ctx context.Context, a Actor) ([]LifeEvent, error) {
	emp, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.LifeEvents(ctx, s.store.DB(), a.TenantID, emp.ID)
	if rows == nil {
		rows = []LifeEvent{}
	}
	return rows, err
}

// EmployeeLifeEvents lists a worker's life events (benefits:read_sensitive).
func (s *Service) EmployeeLifeEvents(ctx context.Context, a Actor, employeeID uuid.UUID) ([]LifeEvent, error) {
	if _, err := s.worker(ctx, a.TenantID, employeeID); err != nil {
		return nil, err
	}
	rows, err := s.store.LifeEvents(ctx, s.store.DB(), a.TenantID, employeeID)
	if rows == nil {
		rows = []LifeEvent{}
	}
	return rows, err
}
