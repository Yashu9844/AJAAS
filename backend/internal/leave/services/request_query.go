package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/repositories"
	"github.com/jaas/jaas/internal/leave/validators"
)

// ListMine lists the caller's own requests (R3); the employee comes from the JWT only (NFR-SEC002).
func (s *requestService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error) {
	emp, err := s.employeeFor(ctx, a, false)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return s.list(ctx, a.TenantID, repositories.RequestFilter{EmployeeID: &emp.ID, Status: status}, page)
}

// List lists tenant requests with filters (R5).
func (s *requestService) List(ctx context.Context, tenantID uuid.UUID, q RequestQuery, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error) {
	f, err := requestFilter(q)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return s.list(ctx, tenantID, f, page)
}

// Get returns one request (R6); foreign ids → 404.
func (s *requestService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveRequestResponse, error) {
	r, err := s.Repos.Requests.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	codes, err := s.typeCodes(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	res := mapRequest(r, codes[r.LeaveTypeID])
	return &res, nil
}

func (s *requestService) list(ctx context.Context, tenantID uuid.UUID, f repositories.RequestFilter, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Requests.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	codes, err := s.typeCodes(ctx, tenantID)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.LeaveRequestResponse, len(rows))
	for i := range rows {
		out[i] = mapRequest(&rows[i], codes[rows[i].LeaveTypeID])
	}
	return out, page.Meta(total), nil
}

// typeCodes maps type id → code in one query (no N+1, NFR-P002).
func (s *requestService) typeCodes(ctx context.Context, tenantID uuid.UUID) (map[uuid.UUID]string, error) {
	types, _, err := s.Repos.Types.List(ctx, s.Tx.DB(), tenantID, "", allTypes)
	if err != nil {
		return nil, err
	}
	codes := make(map[uuid.UUID]string, len(types))
	for _, t := range types {
		codes[t.ID] = t.Code
	}
	return codes, nil
}

func requestFilter(q RequestQuery) (repositories.RequestFilter, error) {
	f := repositories.RequestFilter{Status: q.Status}
	if q.EmployeeID != "" {
		id, err := parseUUIDField("employee_id", q.EmployeeID)
		if err != nil {
			return f, err
		}
		f.EmployeeID = &id
	}
	if q.From != "" {
		d, err := validators.ParseDate(q.From)
		if err != nil {
			return f, invalid("from", err.Error())
		}
		f.From = &d
	}
	if q.To != "" {
		d, err := validators.ParseDate(q.To)
		if err != nil {
			return f, invalid("to", err.Error())
		}
		f.To = &d
	}
	return f, nil
}
