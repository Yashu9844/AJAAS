package controllers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/organization/dto"
	"gorm.io/gorm"
)

type stubLocSvc struct {
	res    *dto.LocationResponse
	err    error
	filter dto.LocationFilter
	status string
}

func (s *stubLocSvc) CreateLocation(context.Context, *gorm.DB, uuid.UUID, dto.CreateLocationRequest, uuid.UUID) (*dto.LocationResponse, error) {
	return s.res, s.err
}
func (s *stubLocSvc) GetLocation(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*dto.LocationResponse, error) {
	return s.res, s.err
}
func (s *stubLocSvc) ListLocations(_ context.Context, _ *gorm.DB, _ uuid.UUID, f dto.LocationFilter, page, perPage int) (*dto.LocationListResponse, error) {
	s.filter = f
	if s.err != nil {
		return nil, s.err
	}
	return &dto.LocationListResponse{Data: []dto.LocationResponse{*s.res}, Meta: dto.NewPaginationMeta(page, perPage, 1)}, nil
}
func (s *stubLocSvc) UpdateLocation(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, dto.UpdateLocationRequest, uuid.UUID) (*dto.LocationResponse, error) {
	return s.res, s.err
}
func (s *stubLocSvc) SetLocationStatus(_ context.Context, _ *gorm.DB, _ uuid.UUID, _ uuid.UUID, status string, _ uuid.UUID) (*dto.LocationResponse, error) {
	s.status = status
	return s.res, s.err
}

// FR-L001..FR-L004 at the HTTP edge: statuses, filters, bad ids, bad bodies, missing tenant, service errors.
func TestLocationController(t *testing.T) {
	stub := &stubLocSvc{res: &dto.LocationResponse{ID: uuid.NewString(), Code: "BLR", Status: "active"}}
	ctrl := NewLocationController(nil, stub)
	id := gin.Params{{Key: "id", Value: uuid.NewString()}}
	run := func(method, target, body string, params gin.Params, h func(*gin.Context), tenant bool) int {
		var rdr *strings.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		c, w := newTestContext(t, method, target, rdr)
		c.Params = params
		if !tenant {
			c.Keys = map[any]any{}
		}
		h(c)
		return w.Code
	}
	valid := `{"code":"BLR","name":"Bengaluru","city":"Bengaluru","state_code":"KA"}`
	for _, tc := range []struct {
		name   string
		method string
		body   string
		params gin.Params
		h      func(*gin.Context)
		want   int
	}{
		{"create", "POST", valid, nil, ctrl.Create, 201},
		{"create bad body", "POST", `{"code":"B"}`, nil, ctrl.Create, 400},
		{"get", "GET", "", id, ctrl.GetByID, 200},
		{"get bad id", "GET", "", gin.Params{{Key: "id", Value: "x"}}, ctrl.GetByID, 400},
		{"list", "GET", "", nil, ctrl.List, 200},
		{"update", "PATCH", `{"name":"Campus"}`, id, ctrl.Update, 200},
		{"update bad body", "PATCH", `{"country_code":"IND"}`, id, ctrl.Update, 400},
		{"update bad id", "PATCH", `{}`, gin.Params{{Key: "id", Value: "x"}}, ctrl.Update, 400},
		{"deactivate", "POST", "", id, ctrl.Deactivate, 200},
		{"activate", "POST", "", id, ctrl.Activate, 200},
	} {
		if got := run(tc.method, "/locations?status=active&state_code=KA&search=blr", tc.body, tc.params, tc.h, true); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
	if stub.filter != (dto.LocationFilter{Status: "active", StateCode: "KA", Search: "blr"}) || stub.status != "active" {
		t.Fatalf("filters and status reach the service: %+v %s", stub.filter, stub.status)
	}
	for name, h := range map[string]func(*gin.Context){"create": ctrl.Create, "list": ctrl.List, "get": ctrl.GetByID, "update": ctrl.Update, "deactivate": ctrl.Deactivate} {
		if got := run("POST", "/locations", valid, id, h, false); got == 200 || got == 201 {
			t.Errorf("%s without tenant: %d", name, got)
		}
	}
	stub.err = errConflict
	for name, h := range map[string]func(*gin.Context){"create": ctrl.Create, "list": ctrl.List, "get": ctrl.GetByID, "update": ctrl.Update, "deactivate": ctrl.Deactivate} {
		if got := run("POST", "/locations", valid, id, h, true); got != http.StatusConflict {
			t.Errorf("%s service error: %d", name, got)
		}
	}
}
