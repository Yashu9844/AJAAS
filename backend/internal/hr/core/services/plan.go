package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// outcome is what completing an action writes (CO-035..CO-038).
type outcome struct {
	worker      *employeeDTO.EmployeeResponse
	current     *models.Assignment // closed at closeAt (nil for hire/rehire)
	closeAt     time.Time
	next        *models.Assignment // opened at the effective date (nil for terminate)
	effectsDate time.Time          // when projections apply (DC-06)
}

// plan validates an action against the worker's state on its effective date and computes the outcome. It runs inside
// a transaction (submit, approve); seats are checked under the position lock.
func (b base) plan(ctx context.Context, tx *gorm.DB, act *models.Action) (*outcome, error) {
	worker, err := b.employee(ctx, act.TenantID, act.EmployeeID)
	if err != nil {
		return nil, err
	}
	if act.ReasonCode == "" {
		return nil, invalid("reason_code", "is required")
	}
	done, err := b.Repo.CompletedActions(ctx, tx, act.TenantID, act.EmployeeID)
	if err != nil {
		return nil, err
	}
	for _, d := range done {
		if d.ID != act.ID && !d.EffectiveDate.Before(act.EffectiveDate) {
			return nil, ErrLaterChange
		}
	}
	all, err := b.Repo.WorkerAssignments(ctx, tx, act.TenantID, act.EmployeeID)
	if err != nil {
		return nil, err
	}
	var active []models.Assignment
	for _, as := range all {
		if as.Status == models.AssignmentActive {
			active = append(active, as)
		}
	}
	out := &outcome{worker: worker, effectsDate: act.EffectiveDate}
	switch {
	case lifecycle.Starts(act.Type):
		err = b.planStart(ctx, tx, act, worker, active, out)
	case act.Type == lifecycle.Terminate:
		err = b.planTerminate(act, active, out)
	default:
		err = b.planChange(ctx, tx, act, active, out)
	}
	if err != nil {
		return nil, err
	}
	if out.next != nil {
		exclude := uuid.Nil
		if out.current != nil {
			exclude = out.current.ID // closed at the effective date, so it never shares the new period
		}
		if err := b.checkNext(ctx, tx, act, out.next, exclude); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (b base) planStart(ctx context.Context, tx *gorm.DB, act *models.Action, worker *employeeDTO.EmployeeResponse, active []models.Assignment, out *outcome) error {
	if act.Type == lifecycle.Hire && len(active) > 0 {
		return ErrAlreadyAssigned
	}
	if act.Type == lifecycle.Rehire && worker.Status != "terminated" && worker.Status != "resigned" {
		return ErrNotTerminated
	}
	for _, as := range active {
		if as.EffectiveTo == nil || as.EffectiveTo.After(act.EffectiveDate) {
			return ErrAlreadyAssigned
		}
	}
	ch := act.Changes
	next := &models.Assignment{TenantID: act.TenantID, EmployeeID: act.EmployeeID, EffectiveFrom: act.EffectiveDate, FTE: kernel.One,
		Status: models.AssignmentActive}
	if err := b.mergeChanges(ctx, tx, act.TenantID, next, ch); err != nil {
		return err
	}
	if next.JobProfileID == nil || next.DepartmentID == nil || next.WorkerType == "" || next.EmploymentType == "" {
		return invalid("changes", act.Type+" needs job_profile_id, department_id (or a position), worker_type and employment_type")
	}
	profile, err := b.Repo.Profile(ctx, tx, act.TenantID, *next.JobProfileID)
	if err != nil || profile == nil {
		return orNotFound(err)
	}
	if next.GradeID == nil {
		next.GradeID = profile.DefaultGradeID
	}
	if profile.ProbationDays > 0 {
		end := act.EffectiveDate.AddDate(0, 0, profile.ProbationDays)
		next.ProbationEndDate = &end
	}
	out.next = next
	return nil
}

// openOn returns the worker's open-ended assignment in effect on d, or ErrNoAssignment.
func openOn(active []models.Assignment, d time.Time) (*models.Assignment, error) {
	for i := range active {
		if active[i].EffectiveTo == nil && active[i].CoversDate(d) {
			return &active[i], nil
		}
	}
	return nil, ErrNoAssignment
}

func (b base) planTerminate(act *models.Action, active []models.Assignment, out *outcome) error {
	cur, err := openOn(active, act.EffectiveDate)
	if err != nil {
		return err
	}
	ch := act.Changes
	if ch.TerminationType == nil || !lifecycle.Termination[*ch.TerminationType] {
		return invalid("termination_type", "must be voluntary or involuntary")
	}
	if ch.LastWorkingDay == nil {
		return invalid("last_working_day", "is required")
	}
	lwd, err := parseDate("last_working_day", *ch.LastWorkingDay)
	if err != nil {
		return err
	}
	if lwd.Before(act.EffectiveDate) || lwd.Before(cur.EffectiveFrom) {
		return invalid("last_working_day", "must not be before the effective date or the assignment start")
	}
	if _, err := parseOptionalDate("notice_date", ch.NoticeDate); err != nil {
		return err
	}
	out.current, out.closeAt, out.effectsDate = cur, lwd.AddDate(0, 0, 1), lwd.AddDate(0, 0, 1)
	return nil
}

func (b base) planChange(ctx context.Context, tx *gorm.DB, act *models.Action, active []models.Assignment, out *outcome) error {
	cur, err := openOn(active, act.EffectiveDate)
	if err != nil {
		return err
	}
	if !cur.EffectiveFrom.Before(act.EffectiveDate) {
		return ErrLaterChange
	}
	next := *cur
	next.ID, next.ActionID, next.EffectiveFrom, next.EffectiveTo = uuid.Nil, uuid.Nil, act.EffectiveDate, nil
	if err := b.mergeChanges(ctx, tx, act.TenantID, &next, act.Changes); err != nil {
		return err
	}
	if err := b.typeRule(ctx, tx, act, cur, &next); err != nil {
		return err
	}
	out.current, out.closeAt, out.next = cur, act.EffectiveDate, &next
	return nil
}

// mergeChanges applies the action's field changes to an assignment; a position supplies the department and profile.
func (b base) mergeChanges(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, as *models.Assignment, ch models.Changes) error {
	if ch.ClearPosition {
		as.PositionID = nil
	}
	if ch.PositionID != nil {
		p, err := b.Repo.Position(ctx, tx, tenantID, *ch.PositionID)
		if err != nil || p == nil {
			return orNotFound(err)
		}
		if ch.DepartmentID != nil && *ch.DepartmentID != p.DepartmentID {
			return invalid("department_id", "must be the position's department")
		}
		as.PositionID, as.DepartmentID, as.JobProfileID = &p.ID, &p.DepartmentID, &p.JobProfileID
		if p.LocationID != nil && ch.LocationID == nil {
			as.LocationID = p.LocationID
		}
	}
	for dst, src := range map[**uuid.UUID]*uuid.UUID{&as.DepartmentID: ch.DepartmentID, &as.LocationID: ch.LocationID,
		&as.ManagerEmployeeID: ch.ManagerEmployeeID, &as.GradeID: ch.GradeID} {
		if src != nil {
			v := *src
			*dst = &v
		}
	}
	if ch.JobProfileID != nil && ch.PositionID == nil {
		v := *ch.JobProfileID
		as.JobProfileID = &v
	}
	if ch.WorkerType != nil {
		if !lifecycle.WorkerTypes[*ch.WorkerType] {
			return invalid("worker_type", "must be employee, contractor or intern")
		}
		as.WorkerType = *ch.WorkerType
	}
	if ch.EmploymentType != nil {
		if !lifecycle.EmploymentTypes[*ch.EmploymentType] {
			return invalid("employment_type", "must be full_time, part_time, contract or intern")
		}
		as.EmploymentType = *ch.EmploymentType
	}
	if ch.FTE != nil {
		if *ch.FTE <= 0 || *ch.FTE > kernel.One {
			return invalid("fte", "must be 0.01–1.00")
		}
		as.FTE = *ch.FTE
	}
	return nil
}

// typeRule is CO-034 and CO-036 for change actions.
func (b base) typeRule(ctx context.Context, tx *gorm.DB, act *models.Action, cur, next *models.Assignment) error {
	ch := act.Changes
	switch act.Type {
	case lifecycle.Promote, lifecycle.Demote:
		if ch.GradeID == nil || sameID(cur.GradeID, next.GradeID) {
			return ErrGradeRule
		}
		if ch.GradeOverrideReason != nil && *ch.GradeOverrideReason != "" {
			return nil
		}
		from, err := b.rank(ctx, tx, act.TenantID, cur.GradeID)
		if err != nil {
			return err
		}
		to, err := b.rank(ctx, tx, act.TenantID, next.GradeID)
		if err != nil {
			return err
		}
		if (act.Type == lifecycle.Promote) != (to > from) {
			return ErrGradeRule
		}
	case lifecycle.Transfer:
		if sameID(cur.DepartmentID, next.DepartmentID) && sameID(cur.LocationID, next.LocationID) {
			return ErrNoChange
		}
	case lifecycle.ChangeJob:
		if sameID(cur.JobProfileID, next.JobProfileID) && sameID(cur.PositionID, next.PositionID) {
			return ErrNoChange
		}
	case lifecycle.ChangeManager:
		if sameID(cur.ManagerEmployeeID, next.ManagerEmployeeID) {
			return ErrNoChange
		}
	case lifecycle.ChangeLocation:
		if sameID(cur.LocationID, next.LocationID) {
			return ErrNoChange
		}
	case lifecycle.ProbationConfirm:
		return probationRule(ch, cur, next)
	}
	return nil
}

func probationRule(ch models.Changes, cur, next *models.Assignment) error {
	if cur.ProbationEndDate == nil {
		return ErrProbationState
	}
	switch {
	case ch.ProbationOutcome != nil && *ch.ProbationOutcome == "confirm":
		next.ProbationEndDate = nil
	case ch.ProbationOutcome != nil && *ch.ProbationOutcome == "extend":
		if ch.ProbationEndDate == nil {
			return invalid("probation_end_date", "is required to extend")
		}
		end, err := parseDate("probation_end_date", *ch.ProbationEndDate)
		if err != nil {
			return err
		}
		if !end.After(*cur.ProbationEndDate) {
			return invalid("probation_end_date", "must be after the current probation end")
		}
		next.ProbationEndDate = &end
	default:
		return invalid("probation_outcome", "must be confirm or extend")
	}
	return nil
}

func (b base) rank(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, id *uuid.UUID) (int, error) {
	if id == nil {
		return 0, nil
	}
	g, err := b.Repo.Grade(ctx, tx, tenantID, *id)
	if err != nil || g == nil {
		return 0, orNotFound(err)
	}
	return g.Rank, nil
}

// checkNext validates the references of the assignment to open (CO-003, CO-014, CO-033).
func (b base) checkNext(ctx context.Context, tx *gorm.DB, act *models.Action, next *models.Assignment, exclude uuid.UUID) error {
	if next.ManagerEmployeeID != nil {
		if *next.ManagerEmployeeID == act.EmployeeID {
			return invalid("manager_employee_id", "a worker cannot manage themselves")
		}
		m, err := b.employee(ctx, act.TenantID, *next.ManagerEmployeeID)
		if err != nil {
			return err
		}
		if !workingStatuses[m.Status] {
			return ErrInactiveReference
		}
	}
	ch := act.Changes
	profile := ch.JobProfileID
	if lifecycle.Starts(act.Type) {
		profile = next.JobProfileID
	}
	if err := b.refs(ctx, tx, act.TenantID, profile, ch.DepartmentID, ch.LocationID); err != nil {
		return err
	}
	if ch.GradeID != nil {
		g, err := b.Repo.Grade(ctx, tx, act.TenantID, *ch.GradeID)
		if err != nil || g == nil {
			return orNotFound(err)
		}
		if g.Status != models.StatusActive {
			return ErrInactiveReference
		}
	}
	if next.DepartmentID != nil && ch.DepartmentID == nil && ch.PositionID != nil {
		if err := b.refs(ctx, tx, act.TenantID, nil, next.DepartmentID, nil); err != nil {
			return err
		}
	}
	return b.checkSeat(ctx, tx, act.TenantID, next, exclude)
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
