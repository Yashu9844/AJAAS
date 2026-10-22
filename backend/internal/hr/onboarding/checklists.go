package onboarding

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Progress counts a checklist's tasks (OB-09).
type Progress struct {
	Total   int `json:"total"`
	Done    int `json:"done"`
	Skipped int `json:"skipped"`
	Open    int `json:"open"`
	Overdue int `json:"overdue"`
}

// ChecklistView is a checklist with its tasks, clearance and progress (OB-09).
type ChecklistView struct {
	Checklist
	Tasks      []Task      `json:"tasks"`
	Clearances []Clearance `json:"clearances"`
	Progress   Progress    `json:"progress"`
}

func (s *Service) progress(tasks []Task) Progress {
	p := Progress{Total: len(tasks)}
	for _, t := range tasks {
		switch t.Status {
		case TaskDone:
			p.Done++
		case TaskSkipped:
			p.Skipped++
		default:
			p.Open++
			if t.DueDate.Before(s.today()) {
				p.Overdue++
			}
		}
	}
	return p
}

func (s *Service) view(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, c *Checklist) (*ChecklistView, error) {
	tasks, err := s.store.Tasks(ctx, db, tenantID, c.ID)
	if err != nil {
		return nil, err
	}
	cl, err := s.store.Clearances(ctx, db, tenantID, c.ID)
	if err != nil {
		return nil, err
	}
	return &ChecklistView{Checklist: *c, Tasks: append([]Task{}, tasks...), Clearances: append([]Clearance{}, cl...), Progress: s.progress(tasks)}, nil
}

// visible loads a checklist the caller may see: onboarding:read/manage, the worker, or the manager (OB-09).
func (s *Service) visible(ctx context.Context, a Actor, id uuid.UUID) (*Checklist, *uuid.UUID, error) {
	c, err := s.store.Checklist(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || c == nil {
		return nil, nil, orNotFound(err)
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, nil, err
	}
	if a.CanRead || a.CanManage || involves(c, me) {
		return c, me, nil
	}
	return nil, nil, ErrNotFound
}

func involves(c *Checklist, me *uuid.UUID) bool {
	return me != nil && (c.EmployeeProfileID == *me || (c.ManagerEmployeeID != nil && *c.ManagerEmployeeID == *me))
}

// Get is OB-09.
func (s *Service) Get(ctx context.Context, a Actor, id uuid.UUID) (*ChecklistView, error) {
	c, _, err := s.visible(ctx, a, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, s.store.DB(), a.TenantID, c)
}

// Checklists is OB-09: everything for onboarding:read/manage, else checklists where the caller is worker or manager.
func (s *Service) Checklists(ctx context.Context, a Actor, f ChecklistFilter, page, perPage int) ([]Checklist, int64, error) {
	if f.Kind != "" && !kinds[f.Kind] {
		return nil, 0, invalid("kind", "must be onboarding or offboarding")
	}
	if f.Status != "" && f.Status != ChecklistOpen && f.Status != ChecklistCompleted && f.Status != ChecklistCancelled {
		return nil, 0, invalid("status", "must be open, completed or cancelled")
	}
	if !a.CanRead && !a.CanManage {
		me, err := s.self(ctx, a)
		if err != nil || me == nil {
			return []Checklist{}, 0, err
		}
		f.Involving = me
	}
	return s.store.Checklists(ctx, s.store.DB(), a.TenantID, f, page, perPage)
}

// Me is OB-09: the caller's own checklists with tasks.
func (s *Service) Me(ctx context.Context, a Actor) ([]ChecklistView, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	if me == nil {
		return nil, ErrEmployeeNotFound
	}
	rows, _, err := s.store.Checklists(ctx, s.store.DB(), a.TenantID, ChecklistFilter{EmployeeID: me}, 1, 100)
	if err != nil {
		return nil, err
	}
	out := make([]ChecklistView, 0, len(rows))
	for i := range rows {
		v, err := s.view(ctx, s.store.DB(), a.TenantID, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// MyTasks is OB-09: tasks assigned to me on open checklists, plus pool tasks for onboarding:manage.
func (s *Service) MyTasks(ctx context.Context, a Actor) ([]Task, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.AssignedTasks(ctx, s.store.DB(), a.TenantID, me, a.CanManage)
	if rows == nil {
		rows = []Task{}
	}
	return rows, err
}

// withTask locks a task and its open checklist and runs fn; the checklist may complete afterwards (OB-06).
func (s *Service) withTask(ctx context.Context, a Actor, id uuid.UUID, verb string, fn func(tx *gorm.DB, c *Checklist, t *Task, me *uuid.UUID) error) (*Task, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	var t *Task
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if t, err = s.store.Task(ctx, tx, a.TenantID, id, false); err != nil || t == nil {
			return orNotFound(err)
		}
		c, err := s.store.Checklist(ctx, tx, a.TenantID, t.ChecklistID, true)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		if !a.CanManage && (me == nil || t.AssigneeEmployeeID == nil || *t.AssigneeEmployeeID != *me) {
			return ErrNotFound
		}
		if c.Status != ChecklistOpen {
			return ErrChecklistState
		}
		if t, err = s.store.Task(ctx, tx, a.TenantID, id, true); err != nil {
			return err
		}
		if err := fn(tx, c, t, me); err != nil {
			return err
		}
		t.UpdatedAt = s.now().UTC()
		if err := s.store.Save(ctx, tx, t); err != nil {
			return err
		}
		return s.maybeComplete(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.task_"+verb, "onb_task", id.String(), map[string]string{"checklist_id": t.ChecklistID.String()})
	return t, nil
}

// Done is OB-05: the assignee or onboarding:manage; a document task needs a verified document.
func (s *Service) Done(ctx context.Context, a Actor, id uuid.UUID, note *string) (*Task, error) {
	n, err := optional("note", note, 1000)
	if err != nil {
		return nil, err
	}
	return s.withTask(ctx, a, id, "done", func(tx *gorm.DB, c *Checklist, t *Task, _ *uuid.UUID) error {
		if t.Status != TaskOpen {
			return ErrTaskState
		}
		if t.DocumentType != nil {
			doc, err := s.p.Documents.VerifiedDocument(ctx, tx, a.TenantID, c.EmployeeProfileID, *t.DocumentType)
			if err != nil {
				return err
			}
			if doc == nil {
				return ErrDocumentRequired
			}
			t.EvidenceDocumentID = doc
		}
		now := s.now().UTC()
		t.Status, t.Note, t.DoneByUserID, t.DoneAt = TaskDone, n, &a.UserID, &now
		return nil
	})
}

// Skip is OB-05: onboarding:manage, with a reason, never a blocking task.
func (s *Service) Skip(ctx context.Context, a Actor, id uuid.UUID, reason *string) (*Task, error) {
	if reason == nil {
		return nil, invalid("reason", "is required")
	}
	r, err := text("reason", *reason, 1000)
	if err != nil {
		return nil, err
	}
	return s.withTask(ctx, a, id, "skipped", func(_ *gorm.DB, _ *Checklist, t *Task, _ *uuid.UUID) error {
		if t.Status != TaskOpen {
			return ErrTaskState
		}
		if t.Blocking {
			return ErrTaskBlocking
		}
		now := s.now().UTC()
		t.Status, t.Note, t.DoneByUserID, t.DoneAt = TaskSkipped, &r, &a.UserID, &now
		return nil
	})
}

// Reopen is OB-05 (onboarding:manage).
func (s *Service) Reopen(ctx context.Context, a Actor, id uuid.UUID) (*Task, error) {
	return s.withTask(ctx, a, id, "reopened", func(_ *gorm.DB, _ *Checklist, t *Task, _ *uuid.UUID) error {
		if t.Status == TaskOpen {
			return ErrTaskState
		}
		t.Status, t.DoneByUserID, t.DoneAt, t.EvidenceDocumentID = TaskOpen, nil, nil, nil
		return nil
	})
}

// maybeComplete closes the checklist when no task is open and (offboarding) clearance is complete (OB-06).
func (s *Service) maybeComplete(ctx context.Context, tx *gorm.DB, c *Checklist) error {
	if c.Kind == KindOffboarding && c.ClearanceCompletedAt == nil {
		return nil
	}
	tasks, err := s.store.Tasks(ctx, tx, c.TenantID, c.ID)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Status == TaskOpen {
			return nil
		}
	}
	return s.complete(ctx, tx, c, nil)
}

func (s *Service) complete(ctx context.Context, tx *gorm.DB, c *Checklist, by *uuid.UUID) error {
	now := s.now().UTC()
	c.Status, c.CompletedAt, c.ClosedByUserID, c.UpdatedAt = ChecklistCompleted, &now, by, now
	if err := s.store.Save(ctx, tx, c); err != nil {
		return err
	}
	return s.store.Event(ctx, tx, c.TenantID, "hr.onboarding.checklist_completed", map[string]string{"checklist_id": c.ID.String(),
		"employee_id": c.EmployeeProfileID.String(), "kind": c.Kind})
}

// withChecklist locks an open checklist (onboarding:manage) and runs fn.
func (s *Service) withChecklist(ctx context.Context, a Actor, id uuid.UUID, verb string, fn func(tx *gorm.DB, c *Checklist) error) (*ChecklistView, error) {
	var c *Checklist
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if c, err = s.store.Checklist(ctx, tx, a.TenantID, id, true); err != nil || c == nil {
			return orNotFound(err)
		}
		if c.Status != ChecklistOpen {
			return ErrChecklistState
		}
		return fn(tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.checklist_"+verb, "onb_checklist", id.String(), map[string]string{"kind": c.Kind})
	return s.view(ctx, s.store.DB(), a.TenantID, c)
}

// Complete is OB-06: no blocking task open and clearance complete; the remaining open tasks are skipped.
func (s *Service) Complete(ctx context.Context, a Actor, id uuid.UUID) (*ChecklistView, error) {
	return s.withChecklist(ctx, a, id, "completed", func(tx *gorm.DB, c *Checklist) error {
		if c.Kind == KindOffboarding && c.ClearanceCompletedAt == nil {
			return ErrClearancePending
		}
		tasks, err := s.store.Tasks(ctx, tx, a.TenantID, c.ID)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if t.Status == TaskOpen && t.Blocking {
				return ErrBlockingOpen
			}
		}
		now, note := s.now().UTC(), "closed with the checklist"
		for i := range tasks {
			if t := &tasks[i]; t.Status == TaskOpen {
				t.Status, t.Note, t.DoneByUserID, t.DoneAt, t.UpdatedAt = TaskSkipped, &note, &a.UserID, &now, now
				if err := s.store.Save(ctx, tx, t); err != nil {
					return err
				}
			}
		}
		return s.complete(ctx, tx, c, &a.UserID)
	})
}

// Cancel is OB-06 (onboarding:manage, with a reason).
func (s *Service) Cancel(ctx context.Context, a Actor, id uuid.UUID, reason *string) (*ChecklistView, error) {
	if reason == nil {
		return nil, invalid("reason", "is required")
	}
	r, err := text("reason", *reason, 500)
	if err != nil {
		return nil, err
	}
	return s.withChecklist(ctx, a, id, "cancelled", func(tx *gorm.DB, c *Checklist) error {
		now := s.now().UTC()
		c.Status, c.CloseReason, c.ClosedByUserID, c.UpdatedAt = ChecklistCancelled, &r, &a.UserID, now
		return s.store.Save(ctx, tx, c)
	})
}

// Clear is OB-07: the manager area by the worker's manager or onboarding:manage, the others by onboarding:manage.
func (s *Service) Clear(ctx context.Context, a Actor, id uuid.UUID, area string, note *string) (*ChecklistView, error) {
	n, err := optional("note", note, 1000)
	if err != nil {
		return nil, err
	}
	known := false
	for _, x := range clearanceSet {
		known = known || x == area
	}
	if !known {
		return nil, invalid("area", "must be manager, hr, it or finance")
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	var c *Checklist
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if c, err = s.store.Checklist(ctx, tx, a.TenantID, id, true); err != nil || c == nil {
			return orNotFound(err)
		}
		isManager := area == AreaManager && me != nil && c.ManagerEmployeeID != nil && *c.ManagerEmployeeID == *me
		if !a.CanManage && !isManager {
			if involves(c, me) {
				return appErr(403, "FORBIDDEN", "you cannot sign this clearance")
			}
			return ErrNotFound
		}
		if c.Kind != KindOffboarding || c.Status != ChecklistOpen {
			return ErrNotOffboarding
		}
		rows, err := s.store.Clearances(ctx, tx, a.TenantID, c.ID)
		if err != nil {
			return err
		}
		pending := 0
		for i := range rows {
			r := &rows[i]
			if r.Area == area {
				if r.ClearedAt != nil {
					return ErrAlreadyCleared
				}
				now := s.now().UTC()
				r.ClearedByUserID, r.ClearedAt, r.Note = &a.UserID, &now, n
				if err := s.store.Save(ctx, tx, r); err != nil {
					return err
				}
			}
			if r.ClearedAt == nil {
				pending++
			}
		}
		if pending > 0 {
			return nil
		}
		now := s.now().UTC()
		c.ClearanceCompletedAt, c.UpdatedAt = &now, now
		if err := s.store.Save(ctx, tx, c); err != nil {
			return err
		}
		if err := s.store.Event(ctx, tx, a.TenantID, "hr.onboarding.clearance_completed", map[string]string{"checklist_id": c.ID.String(),
			"employee_id": c.EmployeeProfileID.String()}); err != nil {
			return err
		}
		return s.maybeComplete(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.clearance_signed", "onb_checklist", id.String(), map[string]string{"area": area})
	return s.view(ctx, s.store.DB(), a.TenantID, c)
}

// SubmitExit is OB-08: the departing worker, once, on an open offboarding checklist.
func (s *Service) SubmitExit(ctx context.Context, a Actor, id uuid.UUID, answers Answers) (*ExitInterview, error) {
	if len(answers) == 0 || len(answers) > 50 {
		return nil, invalid("answers", "must hold 1–50 answers")
	}
	clean := Answers{}
	for k, v := range answers {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || utf8.RuneCountInString(k) > 100 || utf8.RuneCountInString(v) > 2000 {
			return nil, invalid("answers", "keys are 1–100 characters and answers up to 2000")
		}
		clean[k] = v
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	ei := &ExitInterview{ChecklistID: id, TenantID: a.TenantID, Answers: clean, SubmittedByUserID: a.UserID, SubmittedAt: s.now().UTC()}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.store.Checklist(ctx, tx, a.TenantID, id, true)
		if err != nil || c == nil || me == nil || c.EmployeeProfileID != *me {
			return orNotFound(err)
		}
		if c.Kind != KindOffboarding || c.Status != ChecklistOpen {
			return ErrNotOffboarding
		}
		if prev, err := s.store.ExitInterview(ctx, tx, a.TenantID, id); err != nil || prev != nil {
			if err != nil {
				return err
			}
			return ErrAlreadySubmitted
		}
		return s.store.Create(ctx, tx, ei)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "onboarding.exit_interview_submitted", "onb_checklist", id.String(), nil)
	return ei, nil
}

// GetExit is OB-08: the worker or onboarding:exit_interview; everyone else gets 404.
func (s *Service) GetExit(ctx context.Context, a Actor, id uuid.UUID) (*ExitInterview, error) {
	c, err := s.store.Checklist(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	if !a.CanReadEI {
		me, err := s.self(ctx, a)
		if err != nil {
			return nil, err
		}
		if me == nil || c.EmployeeProfileID != *me {
			return nil, ErrNotFound
		}
	}
	ei, err := s.store.ExitInterview(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || ei == nil {
		return nil, orNotFound(err)
	}
	if a.CanReadEI {
		s.log(ctx, a, "onboarding.exit_interview_read", "onb_checklist", id.String(), nil)
	}
	return ei, nil
}

// Progress is OB-10.
func (s *Service) Progress(ctx context.Context, a Actor, kind string) ([]ProgressRow, error) {
	if kind == "" {
		kind = KindOnboarding
	}
	if !kinds[kind] {
		return nil, invalid("kind", "must be onboarding or offboarding")
	}
	rows, err := s.store.Progress(ctx, s.store.DB(), a.TenantID, kind, s.today())
	if rows == nil {
		rows = []ProgressRow{}
	}
	return rows, err
}

// ClearanceCompleted implements onboarding/api.Clearance for payroll final settlement (OB-07).
func (s *Service) ClearanceCompleted(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (bool, error) {
	c, err := s.store.LatestOffboarding(ctx, db, tenantID, employeeID)
	return err == nil && c != nil && c.ClearanceCompletedAt != nil, err
}
