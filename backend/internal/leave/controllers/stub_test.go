package controllers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/services"
)

// stub implements every Module 4 service interface, recording the last call.
type stub struct {
	err     error
	called  string
	actor   services.Actor
	page    dto.Page
	args    []string
	review  dto.ReviewRequest
	applied dto.ApplyLeaveRequest
}

func (s *stub) rec(name string, a services.Actor, args ...string) {
	s.called, s.actor, s.args = name, a, args
}

type typeStub struct{ *stub }
type holidayStub struct{ *stub }
type balanceStub struct{ *stub }
type requestStub struct{ *stub }

var (
	_ services.TypeService    = typeStub{}
	_ services.HolidayService = holidayStub{}
	_ services.BalanceService = balanceStub{}
	_ services.RequestService = requestStub{}
)

func tenantActor(t uuid.UUID) services.Actor { return services.Actor{TenantID: t} }

func (s typeStub) Create(_ context.Context, a services.Actor, _ dto.CreateLeaveTypeRequest) (*dto.LeaveTypeResponse, error) {
	s.rec("TypeCreate", a)
	return &dto.LeaveTypeResponse{}, s.err
}
func (s typeStub) Get(_ context.Context, t, id uuid.UUID) (*dto.LeaveTypeResponse, error) {
	s.rec("TypeGet", tenantActor(t), id.String())
	return &dto.LeaveTypeResponse{}, s.err
}
func (s typeStub) List(_ context.Context, t uuid.UUID, status string, p dto.Page) ([]dto.LeaveTypeResponse, dto.PageMeta, error) {
	s.rec("TypeList", tenantActor(t), status)
	s.page = p
	return []dto.LeaveTypeResponse{}, p.Meta(0), s.err
}
func (s typeStub) Update(_ context.Context, a services.Actor, id uuid.UUID, _ dto.UpdateLeaveTypeRequest) (*dto.LeaveTypeResponse, error) {
	s.rec("TypeUpdate", a, id.String())
	return &dto.LeaveTypeResponse{}, s.err
}
func (s typeStub) Deactivate(_ context.Context, a services.Actor, id uuid.UUID) (*dto.LeaveTypeResponse, error) {
	s.rec("TypeDeactivate", a, id.String())
	return &dto.LeaveTypeResponse{}, s.err
}

func (s holidayStub) Create(_ context.Context, a services.Actor, _ dto.CreateHolidayRequest) (*dto.HolidayResponse, error) {
	s.rec("HolidayCreate", a)
	return &dto.HolidayResponse{}, s.err
}
func (s holidayStub) List(_ context.Context, t uuid.UUID, year string) ([]dto.HolidayResponse, error) {
	s.rec("HolidayList", tenantActor(t), year)
	return []dto.HolidayResponse{}, s.err
}
func (s holidayStub) Delete(_ context.Context, a services.Actor, id uuid.UUID) (*dto.HolidayResponse, error) {
	s.rec("HolidayDelete", a, id.String())
	return &dto.HolidayResponse{}, s.err
}

func (s balanceStub) Mine(_ context.Context, a services.Actor) ([]dto.BalanceResponse, error) {
	s.rec("BalanceMine", a)
	return []dto.BalanceResponse{}, s.err
}
func (s balanceStub) ForEmployee(_ context.Context, a services.Actor, emp string) ([]dto.BalanceResponse, error) {
	s.rec("BalanceFor", a, emp)
	return []dto.BalanceResponse{}, s.err
}
func (s balanceStub) Adjust(_ context.Context, a services.Actor, _ dto.AdjustBalanceRequest) (*dto.BalanceResponse, error) {
	s.rec("BalanceAdjust", a)
	return &dto.BalanceResponse{}, s.err
}
func (s balanceStub) Ledger(_ context.Context, t uuid.UUID, q services.LedgerQuery, p dto.Page) ([]dto.LedgerEntryResponse, dto.PageMeta, error) {
	s.rec("Ledger", tenantActor(t), q.EmployeeID, q.LeaveTypeID)
	s.page = p
	return []dto.LedgerEntryResponse{}, p.Meta(0), s.err
}

func (s requestStub) Preview(_ context.Context, a services.Actor, req dto.ApplyLeaveRequest) (*dto.PreviewResponse, error) {
	s.rec("Preview", a)
	s.applied = req
	return &dto.PreviewResponse{}, s.err
}
func (s requestStub) Apply(_ context.Context, a services.Actor, req dto.ApplyLeaveRequest) (*dto.LeaveRequestResponse, error) {
	s.rec("Apply", a)
	s.applied = req
	return &dto.LeaveRequestResponse{}, s.err
}
func (s requestStub) ListMine(_ context.Context, a services.Actor, status string, p dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error) {
	s.rec("ListMine", a, status)
	s.page = p
	return []dto.LeaveRequestResponse{}, p.Meta(0), s.err
}
func (s requestStub) List(_ context.Context, t uuid.UUID, q services.RequestQuery, p dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error) {
	s.rec("List", tenantActor(t), q.EmployeeID, q.Status, q.From, q.To)
	s.page = p
	return []dto.LeaveRequestResponse{}, p.Meta(0), s.err
}
func (s requestStub) Get(_ context.Context, t, id uuid.UUID) (*dto.LeaveRequestResponse, error) {
	s.rec("Get", tenantActor(t), id.String())
	return &dto.LeaveRequestResponse{}, s.err
}
func (s requestStub) Cancel(_ context.Context, a services.Actor, id uuid.UUID) (*dto.LeaveRequestResponse, error) {
	s.rec("Cancel", a, id.String())
	return &dto.LeaveRequestResponse{}, s.err
}
func (s requestStub) Approve(_ context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error) {
	s.rec("Approve", a, id.String())
	s.review = req
	return &dto.LeaveRequestResponse{}, s.err
}
func (s requestStub) Reject(_ context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error) {
	s.rec("Reject", a, id.String())
	s.review = req
	return &dto.LeaveRequestResponse{}, s.err
}
