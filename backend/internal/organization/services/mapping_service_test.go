package services

import (
	"context"
	"strings"
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
	svc := NewMappingService(mappings, depts, teams, &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}, users, &stubPublisher{}, &stubAudit{})
	return mappings, depts, teams, svc, users
}

type stubDesigRepo struct {
	byID    map[uuid.UUID]*models.Designation
	byTitle map[string]*models.Designation
}

func (s *stubDesigRepo) Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	s.byID[d.ID] = d
	s.byTitle[strings.ToLower(d.Title)] = d
	return nil
}
func (s *stubDesigRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error) {
	return s.byID[id], nil
}
func (s *stubDesigRepo) FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error) {
	return s.byTitle[strings.ToLower(title)], nil
}
func (s *stubDesigRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error) {
	out := make([]models.Designation, 0, len(s.byID))
	for _, d := range s.byID {
		out = append(out, *d)
	}
	return out, int64(len(out)), nil
}
func (s *stubDesigRepo) Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	s.byID[d.ID] = d
	s.byTitle[strings.ToLower(d.Title)] = d
	return nil
}
func (s *stubDesigRepo) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	delete(s.byID, id)
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

func TestMappingService_CRUDAndPrimarySwap(t *testing.T) {
	mappings, depts, teams, svc, users := newMappingSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	uid := uuid.New()
	users.users[uid] = activeUser(uid)

	dept := &models.Department{Name: "Eng", Status: "active"}
	dept.ID = uuid.New()
	dept.TenantID = tenantID
	depts.byID[dept.ID] = dept

	team := &models.Team{DepartmentID: dept.ID, Name: "Core", Status: "active"}
	team.ID = uuid.New()
	team.TenantID = tenantID
	teams.byID[team.ID] = team

	// Happy path: user + dept + team (team belongs to dept) + primary.
	res, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{
		UserID: uid.String(), DepartmentID: strPtr(dept.ID.String()), TeamID: strPtr(team.ID.String()), IsPrimary: true,
	}, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !res.IsPrimary || res.Status != "active" {
		t.Errorf("unexpected mapping: %+v", res)
	}

	// Second primary without swap → 409 (FR-M002).
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: uid.String(), DepartmentID: strPtr(dept.ID.String()), IsPrimary: true}, uuid.New()); err == nil {
		t.Fatal("expected conflict on second primary")
	}

	// Get + list.
	got, err := svc.GetMapping(ctx, nil, tenantID, mustParse(res.ID))
	if err != nil || got.UserID != uid.String() {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	list, err := svc.ListUserMappings(ctx, nil, tenantID, uid, 1, 20)
	if err != nil || list.Meta.TotalItems != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	if _, err := svc.GetMapping(ctx, nil, tenantID, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown mapping")
	}

	// Explicit primary swap via update (EC-07): create secondary, then swap.
	sec, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: uid.String(), DepartmentID: strPtr(dept.ID.String())}, uuid.New())
	if err != nil {
		t.Fatalf("create secondary: %v", err)
	}
	swapped, err := svc.UpdateMapping(ctx, nil, tenantID, mustParse(sec.ID), dto.UpdateMappingRequest{IsPrimary: boolPtr(true)})
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	if !swapped.IsPrimary {
		t.Error("expected secondary to become primary")
	}
	if mappings.byID[mustParse(res.ID)].IsPrimary {
		t.Error("expected old primary demoted")
	}

	// Team/department mismatch → 400 (FR-M004).
	otherDept := &models.Department{Name: "Sales", Status: "active"}
	otherDept.ID = uuid.New()
	otherDept.TenantID = tenantID
	depts.byID[otherDept.ID] = otherDept
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{
		UserID: uid.String(), DepartmentID: strPtr(otherDept.ID.String()), TeamID: strPtr(team.ID.String()),
	}, uuid.New()); err == nil {
		t.Fatal("expected validation error: team not in department")
	}

	// Deactivate with reason, then double → 409 (FR-M007).
	deactivated, err := svc.DeactivateMapping(ctx, nil, tenantID, mustParse(sec.ID), dto.DeactivateMappingRequest{Reason: strPtr("left team")})
	if err != nil || deactivated.Status != "inactive" {
		t.Fatalf("deactivate: %+v err=%v", deactivated, err)
	}
	if _, err := svc.DeactivateMapping(ctx, nil, tenantID, mustParse(sec.ID), dto.DeactivateMappingRequest{}); err == nil {
		t.Fatal("expected conflict on double deactivate")
	}
}

func TestMappingService_ManagerCycle(t *testing.T) {
	mappings, depts, _, svc, users := newMappingSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	userA, userB := uuid.New(), uuid.New()
	users.users[userA] = activeUser(userA)
	users.users[userB] = activeUser(userB)

	dept := &models.Department{Name: "Eng", Status: "active"}
	dept.ID = uuid.New()
	dept.TenantID = tenantID
	depts.byID[dept.ID] = dept

	// A's primary mapping: managed by B.
	mA := &models.Mapping{UserID: userA, DepartmentID: &dept.ID, IsPrimary: true, ManagerUserID: &userB, Status: "active"}
	mA.ID = uuid.New()
	mA.TenantID = tenantID
	mappings.byID[mA.ID] = mA
	mappings.primary[userA] = mA

	// B tries to be managed by A → cycle A→B→A (EC-08).
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{
		UserID: userB.String(), DepartmentID: strPtr(dept.ID.String()), ManagerUserID: strPtr(userA.String()),
	}, uuid.New()); err == nil {
		t.Fatal("expected 409 on manager cycle A→B→A")
	}
}

func boolPtr(b bool) *bool { return &b }

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
