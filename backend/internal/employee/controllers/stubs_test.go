package controllers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext(t *testing.T, method, target string, body io.Reader) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("tenant_id", uuid.New())
	c.Set("user_id", uuid.New())
	return c, w
}

type stubEmpSvc struct {
	res   *dto.EmployeeResponse
	list  []dto.EmployeeResponse
	total int64
	err   error
}

func (s *stubEmpSvc) CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error) {
	return s.res, s.err
}
func (s *stubEmpSvc) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error) {
	return s.res, s.err
}
func (s *stubEmpSvc) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error) {
	return s.res, s.err
}
func (s *stubEmpSvc) List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error) {
	return s.list, s.total, s.err
}
func (s *stubEmpSvc) Update(ctx context.Context, tenantID, id uuid.UUID, req dto.UpdateEmployeeRequest) (*dto.EmployeeResponse, error) {
	return s.res, s.err
}
func (s *stubEmpSvc) UpdateSelfContact(ctx context.Context, tenantID, userID uuid.UUID, req dto.UpdateSelfContactRequest) (*dto.EmployeeResponse, error) {
	return s.res, s.err
}
func (s *stubEmpSvc) TransitionStatus(ctx context.Context, tenantID, id uuid.UUID, req dto.TransitionStatusRequest) (*dto.EmployeeResponse, error) {
	return s.res, s.err
}
func (s *stubEmpSvc) Deactivate(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.err
}

type stubStatSvc struct {
	res *dto.StatutoryResponse
	err error
}

func (s *stubStatSvc) GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID, sensitive bool) (*dto.StatutoryResponse, error) {
	return s.res, s.err
}
func (s *stubStatSvc) Upsert(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UpdateStatutoryRequest) (*dto.StatutoryResponse, error) {
	return s.res, s.err
}

type stubDocSvc struct {
	res  *dto.DocumentResponse
	list []dto.DocumentResponse
	err  error
}

func (s *stubDocSvc) Upload(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UploadDocumentRequest) (*dto.DocumentResponse, error) {
	return s.res, s.err
}
func (s *stubDocSvc) List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.DocumentResponse, error) {
	return s.list, s.err
}
func (s *stubDocSvc) Verify(ctx context.Context, tenantID, profileID, docID, verifierID uuid.UUID) (*dto.DocumentResponse, error) {
	return s.res, s.err
}

type stubTlSvc struct {
	list []dto.TimelineResponse
	err  error
}

func (s *stubTlSvc) List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.TimelineResponse, error) {
	return s.list, s.err
}
