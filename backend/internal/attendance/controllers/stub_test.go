package controllers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/services"
)

// stub implements every Module 3 service interface, recording the last call.
type stub struct {
	err      error
	called   string
	actor    services.Actor
	page     dto.Page
	strArgs  []string
	review   dto.ReviewRequest
	punchReq dto.PunchRequest
}

func (s *stub) rec(name string, a services.Actor, args ...string) {
	s.called, s.actor, s.strArgs = name, a, args
}

var (
	_ services.PunchService          = (*stub)(nil)
	_ services.QueryService          = (*stub)(nil)
	_ services.RegularizationService = regStub{}
	_ services.ShiftService          = shiftStub{}
	_ services.AssignmentService     = assignStub{}
)

func (s *stub) Punch(_ context.Context, a services.Actor, req dto.PunchRequest) (*dto.PunchResult, error) {
	s.rec("Punch", a)
	s.punchReq = req
	return &dto.PunchResult{}, s.err
}
func (s *stub) Today(_ context.Context, a services.Actor) (*dto.TodayResponse, error) {
	s.rec("Today", a)
	return &dto.TodayResponse{}, s.err
}
func (s *stub) Mine(_ context.Context, a services.Actor, from, to string) ([]dto.RecordResponse, error) {
	s.rec("Mine", a, from, to)
	return []dto.RecordResponse{}, s.err
}
func (s *stub) List(_ context.Context, t uuid.UUID, q services.RecordQuery, p dto.Page) ([]dto.RecordResponse, dto.PageMeta, error) {
	s.rec("RecordList", services.Actor{TenantID: t}, q.EmployeeID, q.From, q.To, q.Status)
	s.page = p
	return []dto.RecordResponse{}, p.Meta(0), s.err
}
func (s *stub) Detail(_ context.Context, t, id uuid.UUID) (*dto.RecordDetailResponse, error) {
	s.rec("Detail", services.Actor{TenantID: t}, id.String())
	return &dto.RecordDetailResponse{}, s.err
}
func (s *stub) Summary(_ context.Context, t uuid.UUID, date string) (*dto.SummaryResponse, error) {
	s.rec("Summary", services.Actor{TenantID: t}, date)
	return &dto.SummaryResponse{}, s.err
}

// regStub separates regularization List from QueryService.List (same method name).
type regStub struct{ *stub }

func (r regStub) Create(_ context.Context, a services.Actor, _ dto.CreateRegularizationRequest) (*dto.RegularizationResponse, error) {
	r.rec("RegCreate", a)
	return &dto.RegularizationResponse{}, r.err
}
func (r regStub) ListMine(_ context.Context, a services.Actor, status string, p dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error) {
	r.rec("RegListMine", a, status)
	r.page = p
	return nil, p.Meta(0), r.err
}
func (r regStub) List(_ context.Context, t uuid.UUID, status, emp string, p dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error) {
	r.rec("RegList", services.Actor{TenantID: t}, status, emp)
	return nil, p.Meta(0), r.err
}
func (r regStub) Cancel(_ context.Context, a services.Actor, id uuid.UUID) (*dto.RegularizationResponse, error) {
	r.rec("RegCancel", a, id.String())
	return &dto.RegularizationResponse{}, r.err
}
func (r regStub) Approve(_ context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error) {
	r.rec("RegApprove", a, id.String())
	r.review = req
	return &dto.RegularizationResponse{}, r.err
}
func (r regStub) Reject(_ context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error) {
	r.rec("RegReject", a, id.String())
	r.review = req
	return &dto.RegularizationResponse{}, r.err
}

// shiftStub separates shift/assignment List methods.
type shiftStub struct{ *stub }

func (s shiftStub) Create(_ context.Context, a services.Actor, _ dto.CreateShiftRequest) (*dto.ShiftResponse, error) {
	s.rec("ShiftCreate", a)
	return &dto.ShiftResponse{}, s.err
}
func (s shiftStub) Get(_ context.Context, t, id uuid.UUID) (*dto.ShiftResponse, error) {
	s.rec("ShiftGet", services.Actor{TenantID: t}, id.String())
	return &dto.ShiftResponse{}, s.err
}
func (s shiftStub) List(_ context.Context, t uuid.UUID, status string, p dto.Page) ([]dto.ShiftResponse, dto.PageMeta, error) {
	s.rec("ShiftList", services.Actor{TenantID: t}, status)
	return nil, p.Meta(0), s.err
}
func (s shiftStub) Update(_ context.Context, a services.Actor, id uuid.UUID, _ dto.UpdateShiftRequest) (*dto.ShiftResponse, error) {
	s.rec("ShiftUpdate", a, id.String())
	return &dto.ShiftResponse{}, s.err
}
func (s shiftStub) Deactivate(_ context.Context, a services.Actor, id uuid.UUID) (*dto.ShiftResponse, error) {
	s.rec("ShiftDeactivate", a, id.String())
	return &dto.ShiftResponse{}, s.err
}

type assignStub struct{ *stub }

func (s assignStub) Assign(_ context.Context, a services.Actor, id uuid.UUID, _ dto.AssignShiftRequest) (*dto.AssignmentResponse, error) {
	s.rec("Assign", a, id.String())
	return &dto.AssignmentResponse{}, s.err
}
func (s assignStub) List(_ context.Context, t uuid.UUID, emp string, p dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error) {
	s.rec("AssignList", services.Actor{TenantID: t}, emp)
	return nil, p.Meta(0), s.err
}
