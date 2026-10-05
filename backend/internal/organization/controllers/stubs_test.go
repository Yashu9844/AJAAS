package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// --- stub services (return canned results/errors; controllers only map them) ---

type stubDeptSvc struct {
	res *dto.DepartmentResponse
	err error
}

func (s *stubDeptSvc) CreateDepartment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDepartmentRequest, correlationID uuid.UUID) (*dto.DepartmentResponse, error) {
	return s.res, s.err
}
func (s *stubDeptSvc) GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DepartmentResponse, error) {
	return s.res, s.err
}
func (s *stubDeptSvc) ListDepartments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DepartmentListResponse, error) {
	return &dto.DepartmentListResponse{Data: []dto.DepartmentResponse{}, Meta: dto.NewPaginationMeta(page, perPage, 0)}, s.err
}
func (s *stubDeptSvc) UpdateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDepartmentRequest) (*dto.DepartmentResponse, error) {
	return s.res, s.err
}
func (s *stubDeptSvc) DeactivateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, force bool, reason *string) (*dto.DepartmentResponse, error) {
	return s.res, s.err
}

type stubTeamSvc struct {
	res *dto.TeamResponse
	err error
}

func (s *stubTeamSvc) CreateTeam(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateTeamRequest, correlationID uuid.UUID) (*dto.TeamResponse, error) {
	return s.res, s.err
}
func (s *stubTeamSvc) GetTeam(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.TeamResponse, error) {
	return s.res, s.err
}
func (s *stubTeamSvc) ListTeams(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) (*dto.TeamListResponse, error) {
	return &dto.TeamListResponse{Data: []dto.TeamResponse{}, Meta: dto.NewPaginationMeta(page, perPage, 0)}, s.err
}
func (s *stubTeamSvc) UpdateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateTeamRequest) (*dto.TeamResponse, error) {
	return s.res, s.err
}
func (s *stubTeamSvc) DeactivateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.TeamResponse, error) {
	return s.res, s.err
}

type stubDesigSvc struct {
	res *dto.DesignationResponse
	err error
}

func (s *stubDesigSvc) CreateDesignation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDesignationRequest, correlationID uuid.UUID) (*dto.DesignationResponse, error) {
	return s.res, s.err
}
func (s *stubDesigSvc) GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DesignationResponse, error) {
	return s.res, s.err
}
func (s *stubDesigSvc) ListDesignations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DesignationListResponse, error) {
	return &dto.DesignationListResponse{Data: []dto.DesignationResponse{}, Meta: dto.NewPaginationMeta(page, perPage, 0)}, s.err
}
func (s *stubDesigSvc) UpdateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDesignationRequest) (*dto.DesignationResponse, error) {
	return s.res, s.err
}
func (s *stubDesigSvc) DeactivateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.DesignationResponse, error) {
	return s.res, s.err
}

type stubMappingSvc struct {
	res *dto.MappingResponse
	err error
}

func (s *stubMappingSvc) CreateMapping(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateMappingRequest, correlationID uuid.UUID) (*dto.MappingResponse, error) {
	return s.res, s.err
}
func (s *stubMappingSvc) GetMapping(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.MappingResponse, error) {
	return s.res, s.err
}
func (s *stubMappingSvc) ListUserMappings(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) (*dto.MappingListResponse, error) {
	return &dto.MappingListResponse{Data: []dto.MappingResponse{}, Meta: dto.NewPaginationMeta(page, perPage, 0)}, s.err
}
func (s *stubMappingSvc) UpdateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateMappingRequest) (*dto.MappingResponse, error) {
	return s.res, s.err
}
func (s *stubMappingSvc) DeactivateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.DeactivateMappingRequest) (*dto.MappingResponse, error) {
	return s.res, s.err
}
func (s *stubMappingSvc) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error {
	return s.err
}

type stubChartSvc struct {
	chart *dto.OrgChartResponse
	chain *dto.UserChainResponse
	err   error
}

func (s *stubChartSvc) GetChart(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, maxDepth int, includeInactive bool) (*dto.OrgChartResponse, error) {
	if s.chart != nil {
		return s.chart, s.err
	}
	return &dto.OrgChartResponse{Data: []dto.OrgChartNode{}}, s.err
}
func (s *stubChartSvc) GetUserChain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.UserChainResponse, error) {
	if s.chain != nil {
		return s.chain, s.err
	}
	return &dto.UserChainResponse{UserID: userID.String(), Ancestors: []dto.DepartmentResponse{}, DirectReports: []string{}}, s.err
}

// --- helpers ---

// newTestContext builds a gin test context with tenant context pre-set.
func newTestContext(t *testing.T, method, target string, body *strings.Reader) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	var rdr *strings.Reader
	if body != nil {
		rdr = body
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("tenant_id", uuid.New())
	return c, w
}

var errNotFound = sharedErrors.ErrNotFound
var errConflict = sharedErrors.ErrConflict
var errValidation = &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "bad input", StatusCode: 400}
var errOpaque = &opaqueError{msg: "boom"}

type opaqueError struct{ msg string }

func (e *opaqueError) Error() string { return e.msg }

func httptestRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func httptestRequest(method, target string, body *strings.Reader) *http.Request {
	if body == nil {
		body = strings.NewReader("")
	}
	return httptest.NewRequest(method, target, body)
}

func validationDetails() []sharedErrors.ValidationErrorDetail {
	return []sharedErrors.ValidationErrorDetail{{Field: "name", Message: "required"}}
}
