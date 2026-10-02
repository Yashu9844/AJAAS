package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

type stubUserChecker struct {
	users map[uuid.UUID]*identityDTO.UserResponse
}

func (s *stubUserChecker) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*identityDTO.UserResponse, error) {
	return s.users[id], nil
}

func newMappingSvc() (*stubMappingRepo, *stubDeptRepo, *stubTeamRepo, MappingService, *stubUserChecker) {
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}}
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}}
	users := &stubUserChecker{users: map[uuid.UUID]*identityDTO.UserResponse{}}
	svc := NewMappingService(mappings, depts, teams, &stubDesigRepo{}, users, &stubPublisher{}, &stubAudit{})
	return mappings, depts, teams, svc, users
}

type stubDesigRepo struct{}

func (s *stubDesigRepo) Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	return nil
}
func (s *stubDesigRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error) {
	return nil, nil
}
func (s *stubDesigRepo) FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error) {
	return nil, nil
}
func (s *stubDesigRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error) {
	return nil, 0, nil
}
func (s *stubDesigRepo) Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	return nil
}
func (s *stubDesigRepo) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return nil
}

func activeUser(id uuid.UUID) *identityDTO.UserResponse {
	return &identityDTO.UserResponse{ID: id.String(), Status: "active"}
}

func TestMappingService_CreateAndPrimaryGuard(t *testing.T) {
	_, _, _, svc, users := newMappingSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	uid := uuid.New()
	users.users[uid] = activeUser(uid)

	res, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: uid.String(), IsPrimary: true}, uuid.New())
	if err == nil {
		t.Fatal("expected validation error with no org refs")
	}
	_ = res

	deptID := uuid.New().String()
	// Unknown department → 404.
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: uid.String(), DepartmentID: &deptID, IsPrimary: true}, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown department")
	}
}

func TestMappingService_ManagerSelfAndMissing(t *testing.T) {
	_, _, _, svc, users := newMappingSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	uid := uuid.New()
	users.users[uid] = activeUser(uid)

	// Manager = self → 400 (caught before user resolution).
	dept := &models.Department{Name: "D"}
	_ = dept
	self := uid.String()
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: uid.String(), ManagerUserID: &self}, uuid.New()); err == nil {
		t.Fatal("expected error on self-manager without org refs")
	}

	// Unknown manager user → 404.
	missing := uuid.New().String()
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: uid.String(), ManagerUserID: &missing}, uuid.New()); err == nil {
		t.Fatal("expected error on unknown manager")
	}
}

func TestMappingService_DeactivateUserConverges(t *testing.T) {
	mappings, _, _, svc, users := newMappingSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	uid := uuid.New()
	users.users[uid] = activeUser(uid)

	m := &models.Mapping{UserID: uid, IsPrimary: true, Status: "active"}
	m.TenantID = tenantID
	m.ID = uuid.New()
	mappings.byID[m.ID] = m
	mappings.primary[uid] = m

	report := &models.Mapping{UserID: uuid.New(), ManagerUserID: &uid, Status: "active"}
	report.TenantID = tenantID
	report.ID = uuid.New()
	mappings.byID[report.ID] = report

	if err := svc.DeactivateUserMappings(ctx, nil, tenantID, uid, uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mappings.byID[m.ID].Status != "inactive" {
		t.Error("expected user mapping deactivated")
	}
	if mappings.byID[report.ID].ManagerUserID != nil {
		t.Error("expected report manager ref cleared")
	}
}
