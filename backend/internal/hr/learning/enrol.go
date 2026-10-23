package learning

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SelfEnrolInput is the body of POST /enrolments/self: exactly one of the two ids.
type SelfEnrolInput struct {
	CourseID *string `json:"course_id"`
	PlanID   *string `json:"plan_id"`
}

// CompleteInput is the learner's completion submission (LN-C10).
type CompleteInput struct {
	Score          *int    `json:"score"`
	EvidenceRef    *string `json:"evidence_ref"`
	CertificateRef *string `json:"certificate_ref"`
}

// CorrectInput is HR's correction of a completion (LN-C12).
type CorrectInput struct {
	Reason         string  `json:"reason"`
	Score          *int    `json:"score"`
	EvidenceRef    *string `json:"evidence_ref"`
	CertificateRef *string `json:"certificate_ref"`
}

// EnrolmentView is an enrolment with its live completion.
type EnrolmentView struct {
	Enrolment
	Completion *Completion `json:"completion,omitempty"`
}

// PendingItem is a completion waiting for the caller to verify.
type PendingItem struct {
	Completion
	EmployeeID uuid.UUID `json:"employee_id"`
	CourseID   uuid.UUID `json:"course_id"`
}

// PlanProgress is a worker's progress through a plan (LN-C04).
type PlanProgress struct {
	PlanID   uuid.UUID          `json:"plan_id"`
	Complete bool               `json:"complete"`
	Items    []PlanProgressItem `json:"items"`
}

// PlanProgressItem is one course of a plan: status is `not_enrolled` before an enrolment exists.
type PlanProgressItem struct {
	CourseID  uuid.UUID `json:"course_id"`
	Mandatory bool      `json:"mandatory"`
	Status    string    `json:"status"`
	Overdue   bool      `json:"overdue"`
}

func decorate(rows []Enrolment, today Date) {
	for i := range rows {
		rows[i].Overdue = rows[i].Live() && rows[i].DueDate != nil && rows[i].DueDate.Before(today.Time)
	}
}

func (s *Service) isManagerOf(ctx context.Context, a Actor, learnerUser uuid.UUID) (bool, error) {
	ids, err := s.p.Reports.DirectReportUserIDs(ctx, a.TenantID, a.UserID)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == learnerUser {
			return true, nil
		}
	}
	return false, nil
}

// reportIDs resolves the employee ids of the caller's direct reports.
func (s *Service) reportIDs(ctx context.Context, a Actor) ([]uuid.UUID, error) {
	users, err := s.p.Reports.DirectReportUserIDs(ctx, a.TenantID, a.UserID)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{}
	for _, u := range users {
		e, err := s.p.Employees.GetByUserID(ctx, a.TenantID, u)
		if err != nil || e == nil {
			continue // a report without a profile has nothing to show
		}
		out = append(out, e.ID)
	}
	return out, nil
}

func validStatusFilter(status string) error {
	switch status {
	case "", EnrolNotStarted, EnrolInProgress, EnrolCompleted, EnrolWaived, FilterOverdue:
		return nil
	}
	return invalid("status", "must be not_started, in_progress, completed, waived or overdue")
}

// --- self-enrolment (LN-C08) ---

// EnrolSelf enrols the caller in a course, or in every course of a plan, when the courses allow it.
func (s *Service) EnrolSelf(ctx context.Context, a Actor, in SelfEnrolInput) ([]Enrolment, error) {
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
	me, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	cnt, err := s.contentOf(ctx, s.store.DB(), a.TenantID, courseID, planID, false, true)
	if err != nil {
		return nil, err
	}
	courses, err := s.store.CoursesByIDs(ctx, s.store.DB(), a.TenantID, cnt.Courses)
	if err != nil {
		return nil, err
	}
	for _, c := range courses {
		if !c.SelfEnrol {
			return nil, ErrSelfEnrolOff
		}
	}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	p, err := s.p.Core.PlacementOn(ctx, a.TenantID, me.ID, today.Time)
	if err != nil {
		return nil, err
	}
	var out []Enrolment
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		var rows []Enrolment
		for _, course := range cnt.Courses {
			if err := s.store.Lock(ctx, tx, "self:"+me.ID.String()+":"+course.String()); err != nil {
				return err
			}
			live, err := s.store.LiveSelfEnrolment(ctx, tx, a.TenantID, me.ID, course)
			if err != nil {
				return err
			}
			if live != nil {
				continue
			}
			id := uuid.New()
			rows = append(rows, Enrolment{ID: id, TenantID: a.TenantID, EmployeeProfileID: me.ID, CourseID: course, PlanID: cnt.PlanID, Source: SourceSelf,
				Status: EnrolNotStarted, DepartmentID: department(p), DedupeKey: fmt.Sprintf("s:%s:%s", course, id)})
		}
		if len(rows) == 0 {
			return ErrAlreadyEnrolled
		}
		if _, err := s.store.InsertEnrolments(ctx, tx, rows); err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, e := range out {
		s.log(ctx, a, "learning.enrolment.self", "learning_enrolment", e.ID.String(), map[string]string{"course_id": e.CourseID.String()})
	}
	return out, nil
}

// --- the learner's own enrolment (LN-C10) ---

// own loads an enrolment the caller owns, locked; anyone else's is 404.
func (s *Service) own(ctx context.Context, tx *gorm.DB, a Actor, me uuid.UUID, id uuid.UUID) (*Enrolment, error) {
	e, err := s.store.Enrolment(ctx, tx, a.TenantID, id, true)
	if err != nil || e == nil || e.EmployeeProfileID != me {
		return nil, orNotFound(err)
	}
	return e, nil
}

// Start moves the caller's not-started enrolment to in progress.
func (s *Service) Start(ctx context.Context, a Actor, id uuid.UUID) (*Enrolment, error) {
	me, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	var out *Enrolment
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		e, err := s.own(ctx, tx, a, me.ID, id)
		if err != nil {
			return err
		}
		if e.Status != EnrolNotStarted {
			return ErrEnrolmentState
		}
		now := s.now().UTC()
		e.Status, e.StartedAt, e.UpdatedAt = EnrolInProgress, &now, now
		out = e
		return s.store.Save(ctx, tx, e)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.enrolment.start", "learning_enrolment", id.String(), nil)
	return out, nil
}

func completionFields(score *int, evidence, certificate *string) (*int, *string, *string, error) {
	if score != nil && (*score < 0 || *score > 100) {
		return nil, nil, nil, invalid("score", "must be between 0 and 100")
	}
	ev, err := optional("evidence_ref", evidence, 300)
	if err != nil {
		return nil, nil, nil, err
	}
	ce, err := optional("certificate_ref", certificate, 300)
	return score, ev, ce, err
}

// Complete is LN-C10: the learner attests completion; a course needing verification leaves it pending.
func (s *Service) Complete(ctx context.Context, a Actor, id uuid.UUID, in CompleteInput) (*EnrolmentView, error) {
	score, ev, ce, err := completionFields(in.Score, in.EvidenceRef, in.CertificateRef)
	if err != nil {
		return nil, err
	}
	me, err := s.me(ctx, a)
	if err != nil {
		return nil, err
	}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	var out *EnrolmentView
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		e, err := s.own(ctx, tx, a, me.ID, id)
		if err != nil {
			return err
		}
		if !e.Live() {
			return ErrEnrolmentState
		}
		live, err := s.store.LiveCompletion(ctx, tx, a.TenantID, e.ID, true)
		if err != nil {
			return err
		}
		if live != nil {
			return ErrCompletionPending
		}
		c, err := s.store.Course(ctx, tx, a.TenantID, e.CourseID, false)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		now := s.now().UTC()
		comp := &Completion{ID: uuid.New(), TenantID: a.TenantID, EnrolmentID: e.ID, Attestation: c.Verification, Status: CompPending, Score: score, EvidenceRef: ev,
			CertificateRef: ce, CompletedOn: today, SubmittedByUserID: a.UserID, CreatedAt: now}
		if e.StartedAt == nil {
			e.StartedAt = &now
		}
		e.Status, e.UpdatedAt = EnrolInProgress, now
		if c.Verification == VerifySelf {
			comp.Status, comp.DecidedAt = CompVerified, &now
			if err := s.finish(ctx, tx, a, e); err != nil {
				return err
			}
		} else if err := s.store.Save(ctx, tx, e); err != nil {
			return err
		}
		out = &EnrolmentView{Enrolment: *e, Completion: comp}
		return s.store.Create(ctx, tx, comp)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.enrolment.complete", "learning_enrolment", id.String(), map[string]string{"completion_status": out.Completion.Status})
	return out, nil
}

// finish marks an enrolment completed and emits the event.
func (s *Service) finish(ctx context.Context, tx *gorm.DB, a Actor, e *Enrolment) error {
	t := s.now().UTC()
	e.Status, e.CompletedAt, e.UpdatedAt = EnrolCompleted, &t, t
	if err := s.store.Save(ctx, tx, e); err != nil {
		return err
	}
	return s.store.Event(ctx, tx, a.TenantID, "learning.enrolment.completed", map[string]string{"enrolment_id": e.ID.String(), "course_id": e.CourseID.String()})
}

// --- verification (LN-C11) ---

// Verify approves a pending completion.
func (s *Service) Verify(ctx context.Context, a Actor, id uuid.UUID) (*EnrolmentView, error) {
	return s.decide(ctx, a, id, true, nil)
}

// Reject sends a pending completion back to the learner; the comment is required.
func (s *Service) Reject(ctx context.Context, a Actor, id uuid.UUID, comment string) (*EnrolmentView, error) {
	t, err := required("comment", comment, 500)
	if err != nil {
		return nil, err
	}
	return s.decide(ctx, a, id, false, &t)
}

func (s *Service) decide(ctx context.Context, a Actor, id uuid.UUID, approve bool, comment *string) (*EnrolmentView, error) {
	var out *EnrolmentView
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		comp, err := s.store.Completion(ctx, tx, a.TenantID, id, true)
		if err != nil || comp == nil {
			return orNotFound(err)
		}
		e, err := s.store.Enrolment(ctx, tx, a.TenantID, comp.EnrolmentID, true)
		if err != nil || e == nil {
			return orNotFound(err)
		}
		learner, err := s.worker(ctx, a.TenantID, e.EmployeeProfileID)
		if err != nil {
			return err
		}
		if learner.UserID == a.UserID {
			return ErrSelfVerify
		}
		if err := s.mayVerify(ctx, a, comp, learner.UserID); err != nil {
			return err
		}
		if comp.Status != CompPending {
			return ErrCompletionState
		}
		now := s.now().UTC()
		comp.DecidedByUserID, comp.DecidedAt, comp.DecisionComment = &a.UserID, &now, comment
		if approve {
			comp.Status = CompVerified
			if err := s.finish(ctx, tx, a, e); err != nil {
				return err
			}
		} else {
			comp.Status = CompRejected
			e.Status, e.UpdatedAt = EnrolInProgress, now
			if err := s.store.Save(ctx, tx, e); err != nil {
				return err
			}
		}
		out = &EnrolmentView{Enrolment: *e, Completion: comp}
		return s.store.Save(ctx, tx, comp)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.completion."+out.Completion.Status, "learning_completion", id.String(), nil)
	return out, nil
}

// mayVerify: learning:manage verifies anything; a manager verifies their reports' completions on manager-verified courses.
// Anyone else sees nothing (404).
func (s *Service) mayVerify(ctx context.Context, a Actor, comp *Completion, learnerUser uuid.UUID) error {
	if a.CanManage {
		return nil
	}
	if comp.Attestation == VerifyManager {
		ok, err := s.isManagerOf(ctx, a, learnerUser)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return ErrNotFound
}

// Pending lists the completions the caller can verify.
func (s *Service) Pending(ctx context.Context, a Actor) ([]PendingItem, error) {
	comps, err := s.store.Pending(ctx, s.store.DB(), a.TenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(comps))
	for _, c := range comps {
		ids = append(ids, c.EnrolmentID)
	}
	rows, err := s.store.EnrolmentsByIDs(ctx, s.store.DB(), a.TenantID, ids)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]Enrolment{}
	for _, e := range rows {
		byID[e.ID] = e
	}
	var reports map[uuid.UUID]bool
	if !a.CanManage {
		list, err := s.reportIDs(ctx, a)
		if err != nil {
			return nil, err
		}
		reports = map[uuid.UUID]bool{}
		for _, id := range list {
			reports[id] = true
		}
	}
	out := []PendingItem{}
	for _, c := range comps {
		e, ok := byID[c.EnrolmentID]
		if !ok || (!a.CanManage && !(reports[e.EmployeeProfileID] && c.Attestation == VerifyManager)) {
			continue
		}
		out = append(out, PendingItem{Completion: c, EmployeeID: e.EmployeeProfileID, CourseID: e.CourseID})
	}
	return out, nil
}

// --- HR corrections and waivers (LN-C12, LN-C13) ---

// Correct writes a new verified completion that supersedes the live one, or completes on the worker's behalf.
func (s *Service) Correct(ctx context.Context, a Actor, id uuid.UUID, in CorrectInput) (*EnrolmentView, error) {
	reason, err := required("reason", in.Reason, 500)
	if err != nil {
		return nil, err
	}
	score, ev, ce, err := completionFields(in.Score, in.EvidenceRef, in.CertificateRef)
	if err != nil {
		return nil, err
	}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	var out *EnrolmentView
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		e, err := s.store.Enrolment(ctx, tx, a.TenantID, id, true)
		if err != nil || e == nil {
			return orNotFound(err)
		}
		if e.Status == EnrolWaived {
			return ErrEnrolmentState
		}
		now := s.now().UTC()
		comp := &Completion{ID: uuid.New(), TenantID: a.TenantID, EnrolmentID: e.ID, Attestation: VerifyHR, Status: CompVerified, Score: score, EvidenceRef: ev,
			CertificateRef: ce, CompletedOn: today, SubmittedByUserID: a.UserID, DecidedByUserID: &a.UserID, DecidedAt: &now, CorrectionReason: &reason, CreatedAt: now}
		live, err := s.store.LiveCompletion(ctx, tx, a.TenantID, e.ID, true)
		if err != nil {
			return err
		}
		if live != nil {
			live.Status = CompSuperseded
			comp.SupersedesID = &live.ID
			if err := s.store.Save(ctx, tx, live); err != nil {
				return err
			}
		}
		if e.StartedAt == nil {
			e.StartedAt = &now
		}
		if e.Status != EnrolCompleted {
			if err := s.finish(ctx, tx, a, e); err != nil {
				return err
			}
		}
		out = &EnrolmentView{Enrolment: *e, Completion: comp}
		return s.store.Create(ctx, tx, comp)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.completion.correct", "learning_enrolment", id.String(), nil)
	return out, nil
}

// Waive excuses a live enrolment (LN-C13).
func (s *Service) Waive(ctx context.Context, a Actor, id uuid.UUID, reason string) (*Enrolment, error) {
	r, err := required("reason", reason, 500)
	if err != nil {
		return nil, err
	}
	var out *Enrolment
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		e, err := s.store.Enrolment(ctx, tx, a.TenantID, id, true)
		if err != nil || e == nil {
			return orNotFound(err)
		}
		if !e.Live() {
			return ErrEnrolmentState
		}
		live, err := s.store.LiveCompletion(ctx, tx, a.TenantID, e.ID, true)
		if err != nil {
			return err
		}
		if live != nil {
			return ErrCompletionPending
		}
		e.Status, e.WaiveReason, e.UpdatedAt = EnrolWaived, &r, s.now().UTC()
		out = e
		if err := s.store.Save(ctx, tx, e); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "learning.enrolment.waived", map[string]string{"enrolment_id": id.String(), "course_id": e.CourseID.String()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "learning.enrolment.waive", "learning_enrolment", id.String(), nil)
	return out, nil
}

// --- views (LN-C15, LN-C16) ---

func (s *Service) list(ctx context.Context, a Actor, f EnrolFilter, page, perPage int) ([]Enrolment, int64, error) {
	if err := validStatusFilter(f.Status); err != nil {
		return nil, 0, err
	}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, 0, err
	}
	f.Today = today
	rows, total, err := s.store.Enrolments(ctx, s.store.DB(), a.TenantID, f, page, perPage)
	decorate(rows, today)
	return rows, total, err
}

// Mine lists the caller's enrolments.
func (s *Service) Mine(ctx context.Context, a Actor, status string, page, perPage int) ([]Enrolment, int64, error) {
	me, err := s.me(ctx, a)
	if err != nil {
		return nil, 0, err
	}
	return s.list(ctx, a, EnrolFilter{EmployeeID: &me.ID, Status: status}, page, perPage)
}

// Team lists the enrolments of the caller's direct reports.
func (s *Service) Team(ctx context.Context, a Actor, status string, page, perPage int) ([]Enrolment, int64, error) {
	ids, err := s.reportIDs(ctx, a)
	if err != nil {
		return nil, 0, err
	}
	return s.list(ctx, a, EnrolFilter{EmployeeIDs: ids, Status: status}, page, perPage)
}

// Enrolments lists every enrolment (learning:read).
func (s *Service) Enrolments(ctx context.Context, a Actor, f EnrolFilter, page, perPage int) ([]Enrolment, int64, error) {
	return s.list(ctx, a, f, page, perPage)
}

// GetEnrolment returns one enrolment with its live completion: the learner, their manager or learning:read.
func (s *Service) GetEnrolment(ctx context.Context, a Actor, id uuid.UUID) (*EnrolmentView, error) {
	e, err := s.store.Enrolment(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || e == nil {
		return nil, orNotFound(err)
	}
	if !a.CanRead {
		learner, err := s.worker(ctx, a.TenantID, e.EmployeeProfileID)
		if err != nil {
			return nil, err
		}
		allowed := learner.UserID == a.UserID
		if !allowed {
			if allowed, err = s.isManagerOf(ctx, a, learner.UserID); err != nil {
				return nil, err
			}
		}
		if !allowed {
			return nil, ErrNotFound
		}
	}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	rows := []Enrolment{*e}
	decorate(rows, today)
	live, err := s.store.LiveCompletion(ctx, s.store.DB(), a.TenantID, e.ID, false)
	return &EnrolmentView{Enrolment: rows[0], Completion: live}, err
}

// Transcript lists the caller's completed enrolments with their verified completion (LN-C15).
func (s *Service) Transcript(ctx context.Context, a Actor, page, perPage int) ([]EnrolmentView, int64, error) {
	rows, total, err := s.Mine(ctx, a, EnrolCompleted, page, perPage)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, e := range rows {
		ids = append(ids, e.ID)
	}
	comps, err := s.store.LiveCompletions(ctx, s.store.DB(), a.TenantID, ids)
	if err != nil {
		return nil, 0, err
	}
	byEnrolment := map[uuid.UUID]Completion{}
	for _, c := range comps {
		byEnrolment[c.EnrolmentID] = c
	}
	out := make([]EnrolmentView, 0, len(rows))
	for _, e := range rows {
		v := EnrolmentView{Enrolment: e}
		if c, ok := byEnrolment[e.ID]; ok {
			c := c
			v.Completion = &c
		}
		out = append(out, v)
	}
	return out, total, nil
}

// Progress is LN-C04: a worker's progress through a plan; employee_id needs learning:read.
func (s *Service) Progress(ctx context.Context, a Actor, planID uuid.UUID, employeeID *uuid.UUID) (*PlanProgress, error) {
	var emp uuid.UUID
	if employeeID != nil && a.CanRead {
		emp = *employeeID
	} else if employeeID != nil {
		return nil, ErrForbidden
	} else {
		me, err := s.me(ctx, a)
		if err != nil {
			return nil, err
		}
		emp = me.ID
	}
	p, err := s.store.Plan(ctx, s.store.DB(), a.TenantID, planID, false)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	items, err := s.store.PlanItems(ctx, s.store.DB(), a.TenantID, planID)
	if err != nil {
		return nil, err
	}
	courseIDs := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		courseIDs = append(courseIDs, it.CourseID)
	}
	rows, err := s.store.CourseEnrolments(ctx, s.store.DB(), a.TenantID, emp, courseIDs)
	if err != nil {
		return nil, err
	}
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	decorate(rows, today)
	latest := map[uuid.UUID]Enrolment{}
	for _, e := range rows { // newest cycle first
		if _, ok := latest[e.CourseID]; !ok {
			latest[e.CourseID] = e
		}
	}
	out := &PlanProgress{PlanID: planID, Complete: true, Items: []PlanProgressItem{}}
	for _, it := range items {
		pi := PlanProgressItem{CourseID: it.CourseID, Mandatory: it.Mandatory, Status: "not_enrolled"}
		if e, ok := latest[it.CourseID]; ok {
			pi.Status, pi.Overdue = e.Status, e.Overdue
		}
		if it.Mandatory && pi.Status != EnrolCompleted && pi.Status != EnrolWaived {
			out.Complete = false
		}
		out.Items = append(out.Items, pi)
	}
	return out, nil
}

// Compliance is LN-C16.
func (s *Service) Compliance(ctx context.Context, a Actor, f ComplianceFilter) ([]ComplianceRow, error) {
	today, err := s.today(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	f.Today = today
	rows, err := s.store.Compliance(ctx, s.store.DB(), a.TenantID, f)
	if rows == nil {
		rows = []ComplianceRow{}
	}
	return rows, err
}
