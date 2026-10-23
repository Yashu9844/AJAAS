package learning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"gorm.io/gorm"
)

// AssignmentInput is the body of POST /assignments.
type AssignmentInput struct {
	TargetKind     string  `json:"target_kind"`
	TargetID       *string `json:"target_id"`
	CourseID       *string `json:"course_id"`
	PlanID         *string `json:"plan_id"`
	Mandatory      *bool   `json:"mandatory"`
	DueDate        *string `json:"due_date"`
	DueInDays      *int    `json:"due_in_days"`
	RecurrenceDays *int    `json:"recurrence_days"`
}

// AssignResult is what creating an assignment did (LN-C05).
type AssignResult struct {
	Assignment *Assignment `json:"assignment"`
	Matched    int         `json:"matched"`
	Created    int         `json:"created"`
}

// RunResult is what a recurrence run did (LN-C14).
type RunResult struct {
	Assignments int `json:"assignments"`
	Created     int `json:"created"`
}

// target is a worker an assignment applies to, with the department they sit in (the report snapshot).
type target struct {
	EmployeeID uuid.UUID
	Department *uuid.UUID
}

// content is what an assignment enrols a worker in: the courses (a plan's in order) and the plan, if any.
type content struct {
	PlanID    *uuid.UUID
	Courses   []uuid.UUID
	Mandatory map[uuid.UUID]bool
}

func (s *Service) parseTarget(in AssignmentInput) (kind string, id *uuid.UUID, err error) {
	kind = in.TargetKind
	switch kind {
	case TargetAll:
		if in.TargetID != nil && strings.TrimSpace(*in.TargetID) != "" {
			return "", nil, invalid("target_id", "must be absent for all")
		}
		return kind, nil, nil
	case TargetEmployee, TargetDepartment, TargetProfile, TargetLocation:
		if in.TargetID == nil {
			return "", nil, invalid("target_id", "is required")
		}
		t, err := parseID("target_id", *in.TargetID)
		if err != nil {
			return "", nil, err
		}
		return kind, &t, nil
	}
	return "", nil, invalid("target_kind", "must be employee, department, job_profile, location or all")
}

// contentOf resolves what an assignment enrols in, keeping only courses that can still be taken (published).
// strict makes a draft or retired course (or an archived plan) an error, as when the assignment is created.
func (s *Service) contentOf(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, courseID, planID *uuid.UUID, mandatory bool, strict bool) (*content, error) {
	out := &content{Mandatory: map[uuid.UUID]bool{}}
	var ids []uuid.UUID
	if courseID != nil {
		ids = []uuid.UUID{*courseID}
		out.Mandatory[*courseID] = mandatory
	} else {
		p, err := s.store.Plan(ctx, db, tenantID, *planID, false)
		if err != nil || p == nil {
			return nil, orNotFound(err)
		}
		if strict && p.Status != PlanActive {
			return nil, ErrPlanState
		}
		items, err := s.store.PlanItems(ctx, db, tenantID, p.ID)
		if err != nil {
			return nil, err
		}
		out.PlanID = planID
		for _, it := range items {
			ids = append(ids, it.CourseID)
			out.Mandatory[it.CourseID] = mandatory && it.Mandatory
		}
		if strict && len(ids) == 0 {
			return nil, invalid("plan_id", "the plan has no courses")
		}
	}
	courses, err := s.store.CoursesByIDs(ctx, db, tenantID, ids)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]Course{}
	for _, c := range courses {
		byID[c.ID] = c
	}
	for _, id := range ids {
		c, ok := byID[id]
		switch {
		case !ok:
			return nil, ErrNotFound
		case c.Status == CoursePublished:
			out.Courses = append(out.Courses, id)
		case strict:
			return nil, ErrCourseState
		}
	}
	return out, nil
}

func matches(kind string, id *uuid.UUID, p *coreAPI.Placement) bool {
	switch kind {
	case TargetAll:
		return true
	case TargetEmployee:
		return true // resolved by the employee itself, never by placement
	}
	if p == nil || id == nil {
		return false
	}
	var got *uuid.UUID
	switch kind {
	case TargetDepartment:
		got = p.DepartmentID
	case TargetProfile:
		got = p.JobProfileID
	case TargetLocation:
		got = p.LocationID
	}
	return got != nil && *got == *id
}

// matchWorkers returns the active workers an assignment target covers on a date (LN-C06).
func (s *Service) matchWorkers(ctx context.Context, tenantID uuid.UUID, kind string, id *uuid.UUID, on time.Time) ([]target, error) {
	if kind == TargetEmployee {
		e, err := s.worker(ctx, tenantID, *id)
		if err != nil {
			return nil, err
		}
		if !working(e.Status) {
			return nil, nil
		}
		p, err := s.p.Core.PlacementOn(ctx, tenantID, e.ID, on)
		if err != nil {
			return nil, err
		}
		return []target{{EmployeeID: e.ID, Department: department(p)}}, nil
	}
	var out []target
	const perPage = 100
	for page := 1; ; page++ {
		rows, total, err := s.p.Employees.List(ctx, tenantID, employeeDTO.EmployeeFilter{Page: page, PerPage: perPage})
		if err != nil {
			return nil, err
		}
		for _, e := range rows {
			if !working(e.Status) {
				continue
			}
			p, err := s.p.Core.PlacementOn(ctx, tenantID, e.ID, on)
			if err != nil {
				return nil, err
			}
			if matches(kind, id, p) {
				out = append(out, target{EmployeeID: e.ID, Department: department(p)})
			}
		}
		if len(rows) == 0 || int64(page*perPage) >= total {
			return out, nil
		}
	}
}

// working reports whether an employee status is current workforce: still employed, even on probation, in notice or on leave.
func working(status string) bool {
	switch status {
	case "active", "probation", "notice", "on_leave":
		return true
	}
	return false
}

func department(p *coreAPI.Placement) *uuid.UUID {
	if p == nil {
		return nil
	}
	return p.DepartmentID
}

// dueFor is the due date of an enrolment created on `today` (LN-C05).
func dueFor(a *Assignment, today Date) *Date {
	switch {
	case a.DueDate != nil:
		d := *a.DueDate
		return &d
	case a.DueInDays != nil:
		return &Date{today.AddDate(0, 0, *a.DueInDays)}
	}
	return nil
}

// enrolmentRows builds the idempotent enrolments of one assignment cycle for the workers (LN-C06).
func enrolmentRows(tenantID uuid.UUID, a *Assignment, c *content, workers []target, cycle int, today Date) []Enrolment {
	rows := make([]Enrolment, 0, len(workers)*len(c.Courses))
	due := dueFor(a, today)
	for _, w := range workers {
		for _, course := range c.Courses {
			asn := a.ID
			rows = append(rows, Enrolment{ID: uuid.New(), TenantID: tenantID, EmployeeProfileID: w.EmployeeID, CourseID: course, PlanID: c.PlanID, AssignmentID: &asn,
				Source: SourceAssigned, Mandatory: c.Mandatory[course], Cycle: cycle, Status: EnrolNotStarted, DueDate: due, DepartmentID: w.Department,
				DedupeKey: fmt.Sprintf("a:%s:%s:%d", a.ID, course, cycle)})
		}
	}
	return rows
}

// CreateAssignment is LN-C05: it enrols every matching worker now.
func (s *Service) CreateAssignment(ctx context.Context, a Actor, in AssignmentInput) (*AssignResult, error) {
	kind, targetID, err := s.parseTarget(in)
	if err != nil {
		return nil, err
	}
	if (in.CourseID == nil) == (in.PlanID == nil) {
		return nil, invalid("course_id", "give exactly one of course_id or plan_id")
	}
	var courseID, planID *uuid.UUID
	if in.CourseID != nil {
		id, err := parseID("course_id", *in.CourseID)
		if err != nil {
			return nil, err
		}
		courseID = &id
	} else {
		id, err := parseID("plan_id", *in.PlanID)
		if err != nil {
			return nil, err
		}
		planID = &id
	}
	asn := &Assignment{TenantID: a.TenantID, TargetKind: kind, TargetID: targetID, CourseID: courseID, PlanID: planID, Mandatory: in.Mandatory == nil || *in.Mandatory,
		Status: AssignActive, CreatedByUserID: a.UserID}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	if in.DueDate != nil && in.DueInDays != nil {
		return nil, invalid("due_date", "give due_date or due_in_days, not both")
	}
	if in.DueDate != nil {
		t, perr := time.Parse(DateLayout, strings.TrimSpace(*in.DueDate))
		if perr != nil {
			return nil, invalid("due_date", "must be YYYY-MM-DD")
		}
		if t.Before(today.Time) {
			return nil, invalid("due_date", "must not be in the past")
		}
		asn.DueDate = &Date{t}
	}
	if in.DueInDays != nil {
		if *in.DueInDays < 1 || *in.DueInDays > 1000 {
			return nil, invalid("due_in_days", "must be between 1 and 1000")
		}
		asn.DueInDays = in.DueInDays
	}
	if in.RecurrenceDays != nil {
		if *in.RecurrenceDays < 30 || *in.RecurrenceDays > 3650 {
			return nil, invalid("recurrence_days", "must be between 30 and 3650")
		}
		if asn.DueDate != nil {
			return nil, invalid("recurrence_days", "a recurring assignment needs due_in_days, not due_date")
		}
		asn.RecurrenceDays = in.RecurrenceDays
		next := Date{today.AddDate(0, 0, *in.RecurrenceDays)}
		asn.NextCycleOn = &next
	}
	cnt, err := s.contentOf(ctx, s.store.DB(), a.TenantID, courseID, planID, asn.Mandatory, true)
	if err != nil {
		return nil, err
	}
	workers, err := s.matchWorkers(ctx, a.TenantID, kind, targetID, today.Time)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	asn.CreatedAt, asn.UpdatedAt = now, now
	res := &AssignResult{Assignment: asn, Matched: len(workers)}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.store.Create(ctx, tx, asn); err != nil {
			return err
		}
		n, err := s.store.InsertEnrolments(ctx, tx, enrolmentRows(a.TenantID, asn, cnt, workers, 0, today))
		if err != nil {
			return err
		}
		res.Created = n
		return s.store.Event(ctx, tx, a.TenantID, "learning.assignment.created", map[string]string{"assignment_id": asn.ID.String(), "target_kind": kind})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.assignment.create", "learning_assignment", asn.ID.String(), map[string]string{"target_kind": kind, "created": itoa(res.Created)})
	return res, nil
}

// CancelAssignment stops future enrolments and recurrence; existing enrolments stay (LN-C05). It is idempotent.
func (s *Service) CancelAssignment(ctx context.Context, a Actor, id uuid.UUID) (*Assignment, error) {
	var out *Assignment
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		asn, err := s.store.Assignment(ctx, tx, a.TenantID, id, true)
		if err != nil || asn == nil {
			return orNotFound(err)
		}
		out = asn
		if asn.Status == AssignCancelled {
			return nil
		}
		asn.Status, asn.UpdatedAt = AssignCancelled, s.now().UTC()
		if err := s.store.Save(ctx, tx, asn); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "learning.assignment.cancelled", map[string]string{"assignment_id": id.String()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.assignment.cancel", "learning_assignment", id.String(), nil)
	return out, nil
}

// GetAssignment returns an assignment.
func (s *Service) GetAssignment(ctx context.Context, a Actor, id uuid.UUID) (*Assignment, error) {
	asn, err := s.store.Assignment(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || asn == nil {
		return nil, orNotFound(err)
	}
	return asn, nil
}

// Assignments lists assignments.
func (s *Service) Assignments(ctx context.Context, a Actor, status string, page, perPage int) ([]Assignment, int64, error) {
	if status != "" && status != AssignActive && status != AssignCancelled {
		return nil, 0, invalid("status", "must be active or cancelled")
	}
	return s.store.Assignments(ctx, s.store.DB(), a.TenantID, status, page, perPage)
}

// CoreHook is LN-C07: enrol a joiner or mover in the rule assignments their new placement matches. It runs inside
// core's transaction but in a savepoint, and never returns an error, so learning can neither block nor undo an HR action.
func (s *Service) CoreHook(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, fact coreAPI.ActionFact) error {
	if fact.Rescinded || fact.Placement == nil || fact.Type == "terminate" {
		return nil
	}
	_ = s.store.Savepoint(ctx, tx, func(sp *gorm.DB) error {
		rules, err := s.store.RuleAssignments(ctx, sp, tenantID)
		if err != nil {
			return err
		}
		today, err := s.today(ctx, tenantID)
		if err != nil {
			return err
		}
		w := []target{{EmployeeID: fact.EmployeeID, Department: fact.Placement.DepartmentID}}
		for i := range rules {
			r := &rules[i]
			if !matches(r.TargetKind, r.TargetID, fact.Placement) {
				continue
			}
			cnt, err := s.contentOf(ctx, sp, tenantID, r.CourseID, r.PlanID, r.Mandatory, false)
			if err != nil {
				return err
			}
			if _, err := s.store.InsertEnrolments(ctx, sp, enrolmentRows(tenantID, r, cnt, w, r.Cycle, today)); err != nil {
				return err
			}
		}
		return nil
	})
	return nil
}

// RunRecurrence is LN-C14: it starts the next cycle of every recurring assignment that is due. A nil tenant runs for
// all tenants (the background loop); each assignment advances by one cycle per run.
func (s *Service) RunRecurrence(ctx context.Context, tenantID *uuid.UUID) (*RunResult, error) {
	horizon := Date{midnight(s.now().UTC()).AddDate(0, 0, 1)} // every time zone is within a day of UTC
	due, err := s.store.DueRecurring(ctx, s.store.DB(), tenantID, horizon)
	if err != nil {
		return nil, err
	}
	res := &RunResult{}
	for i := range due {
		n, ran, err := s.advance(ctx, &due[i])
		if err != nil {
			return res, err
		}
		if ran {
			res.Assignments++
			res.Created += n
		}
	}
	return res, nil
}

// advance runs one cycle of one assignment if it is due in its tenant's time zone.
func (s *Service) advance(ctx context.Context, asn *Assignment) (created int, ran bool, err error) {
	today, err := s.today(ctx, asn.TenantID)
	if err != nil {
		return 0, false, err
	}
	if asn.NextCycleOn == nil || today.Before(asn.NextCycleOn.Time) {
		return 0, false, nil
	}
	cnt, err := s.contentOf(ctx, s.store.DB(), asn.TenantID, asn.CourseID, asn.PlanID, asn.Mandatory, false)
	if err != nil {
		return 0, false, err
	}
	workers, err := s.matchWorkers(ctx, asn.TenantID, asn.TargetKind, asn.TargetID, today.Time)
	if err != nil {
		return 0, false, err
	}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		cur, err := s.store.Assignment(ctx, tx, asn.TenantID, asn.ID, true)
		if err != nil || cur == nil {
			return orNotFound(err)
		}
		if cur.Status != AssignActive || cur.NextCycleOn == nil || today.Before(cur.NextCycleOn.Time) {
			return nil // another run got there first
		}
		cur.Cycle++
		created, err = s.store.InsertEnrolments(ctx, tx, enrolmentRows(cur.TenantID, cur, cnt, workers, cur.Cycle, today))
		if err != nil {
			return err
		}
		next := Date{cur.NextCycleOn.AddDate(0, 0, *cur.RecurrenceDays)}
		cur.NextCycleOn, cur.UpdatedAt = &next, s.now().UTC()
		ran = true
		return s.store.Save(ctx, tx, cur)
	})
	return created, ran, err
}
