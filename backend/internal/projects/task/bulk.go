package task

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// BulkInput applies the same changes to up to 100 tasks of one project (TK-09).
type BulkInput struct {
	TaskIDs         []uuid.UUID    `json:"task_ids"`
	ColumnID        *uuid.UUID     `json:"column_id"`
	Priority        *string        `json:"priority"`
	AssigneesAdd    []uuid.UUID    `json:"assignees_add"`
	AssigneesRemove []uuid.UUID    `json:"assignees_remove"`
	LabelsAdd       []uuid.UUID    `json:"labels_add"`
	LabelsRemove    []uuid.UUID    `json:"labels_remove"`
	MilestoneID     Opt[uuid.UUID] `json:"milestone_id"`
	SprintID        Opt[uuid.UUID] `json:"sprint_id"`
	Archive         *bool          `json:"archive"`
	Force           bool           `json:"force"`
}

// BulkError is the reason one task of a bulk request failed.
type BulkError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BulkResult is the outcome for one task.
type BulkResult struct {
	TaskID  uuid.UUID  `json:"task_id"`
	OK      bool       `json:"ok"`
	Version int        `json:"version,omitempty"`
	Error   *BulkError `json:"error,omitempty"`
}

// BulkOutput lists every task outcome in request order.
type BulkOutput struct {
	Results   []BulkResult `json:"results"`
	Succeeded int          `json:"succeeded"`
	Failed    int          `json:"failed"`
}

func (in *BulkInput) validate() error {
	ids, err := uniqueIDs(in.TaskIDs, maxBulk, "task_ids")
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return kernel.Invalid("task_ids", "name at least one task")
	}
	in.TaskIDs = ids
	if in.Priority != nil {
		if err := checkPriority(*in.Priority); err != nil {
			return err
		}
	}
	if in.ColumnID == nil && in.Priority == nil && in.Archive == nil && !in.MilestoneID.Set && !in.SprintID.Set &&
		len(in.AssigneesAdd)+len(in.AssigneesRemove)+len(in.LabelsAdd)+len(in.LabelsRemove) == 0 {
		return kernel.Invalid("body", "name at least one change")
	}
	return nil
}

// errInfo turns any error into the code and message a client may see.
func errInfo(err error) BulkError {
	if _, ok := kernel.AsVersionConflict(err); ok {
		return BulkError{Code: "VERSION_CONFLICT", Message: "the task changed since it was read"}
	}
	var app *sharedErrors.AppError
	if errors.As(sharedErrors.Normalize(err), &app) {
		return BulkError{Code: app.Code, Message: app.Message}
	}
	return BulkError{Code: sharedErrors.ErrInternal.Code, Message: sharedErrors.ErrInternal.Message}
}

// Bulk updates many tasks of a project; each task is authorised and applied on its own, and the response says which
// succeeded (TK-09).
func (s *Service) Bulk(ctx context.Context, a api.Actor, projectID uuid.UUID, in BulkInput) (*BulkOutput, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskUpdate); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	out := &BulkOutput{Results: make([]BulkResult, 0, len(in.TaskIDs))}
	for _, id := range in.TaskIDs {
		res := BulkResult{TaskID: id}
		v, err := s.bulkOne(ctx, a, projectID, id, in)
		if err != nil {
			e := errInfo(err)
			res.Error = &e
			out.Failed++
		} else {
			res.OK, res.Version = true, v.Version
			out.Succeeded++
		}
		out.Results = append(out.Results, res)
	}
	return out, nil
}

func (s *Service) bulkOne(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in BulkInput) (*View, error) {
	t, _, err := s.locate(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != projectID {
		return nil, kernel.NotFound("task")
	}
	spec := mutSpec{Perm: api.TaskUpdate, AllowArchived: in.Archive != nil, LockColumn: targetOf(MoveInput{ColumnID: in.ColumnID})}
	return s.mutate(ctx, a, id, spec, func(m *mut) error { return m.bulkApply(in) })
}

// bulkApply runs the requested changes on one locked task: restore first, archive last.
func (m *mut) bulkApply(in BulkInput) error {
	if in.Archive != nil && !*in.Archive {
		if err := m.setArchived(false); err != nil {
			return err
		}
	}
	if in.ColumnID != nil && *in.ColumnID != m.t.ColumnID {
		if err := m.place(MoveInput{ColumnID: in.ColumnID, Force: in.Force}); err != nil {
			return err
		}
	}
	if in.Priority != nil || in.MilestoneID.Set || in.SprintID.Set {
		if err := m.applyUpdate(UpdateInput{Priority: in.Priority, MilestoneID: in.MilestoneID, SprintID: in.SprintID}); err != nil {
			return err
		}
	}
	if err := m.bulkLinks(in); err != nil {
		return err
	}
	if in.Archive != nil && *in.Archive {
		return m.setArchived(true)
	}
	return nil
}

func (m *mut) bulkLinks(in BulkInput) error {
	steps := []func() error{
		func() error { return m.assign(in.AssigneesAdd) },
		func() error { return m.unassign(in.AssigneesRemove) },
		func() error { return m.attachLabels(in.LabelsAdd) },
		func() error { return m.detachLabels(in.LabelsRemove) },
	}
	for i, step := range steps {
		skip := []int{len(in.AssigneesAdd), len(in.AssigneesRemove), len(in.LabelsAdd), len(in.LabelsRemove)}[i] == 0
		if skip {
			continue
		}
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}
