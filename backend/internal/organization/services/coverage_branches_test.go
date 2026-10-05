package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// errMappingRepo injects failures into mapping lookups.
type errMappingRepo struct{ *stubMappingRepo }

func (e *errMappingRepo) FindByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) ([]models.Mapping, int64, error) {
	return nil, 0, errBoom
}

func TestMappingService_UpdateRefsAndBadInput(t *testing.T) {
	mappings, depts, teams, svc, users := newMappingSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	uid := uuid.New()
	users.users[uid] = activeUser(uid)

	dept := &models.Department{Name: "Eng", Status: "active"}
	dept.ID = uuid.New()
	dept.TenantID = tenantID
	depts.byID[dept.ID] = dept

	m := &models.Mapping{UserID: uid, DepartmentID: &dept.ID, IsPrimary: true, Status: "active"}
	m.ID = uuid.New()
	m.TenantID = tenantID
	mappings.byID[m.ID] = m
	mappings.primary[uid] = m

	// Valid refs change.
	team := &models.Team{DepartmentID: dept.ID, Name: "Core", Status: "active"}
	team.ID = uuid.New()
	team.TenantID = tenantID
	teams.byID[team.ID] = team
	updated, err := svc.UpdateMapping(ctx, nil, tenantID, m.ID, dto.UpdateMappingRequest{TeamID: strPtr(team.ID.String())})
	if err != nil || updated.TeamID == nil || *updated.TeamID != team.ID.String() {
		t.Fatalf("update refs: %+v err=%v", updated, err)
	}

	// Invalid UUID strings → 400 each.
	for _, req := range []dto.UpdateMappingRequest{
		{DepartmentID: strPtr("bad")},
		{TeamID: strPtr("bad")},
		{DesignationID: strPtr("bad")},
		{ManagerUserID: strPtr("bad")},
	} {
		if _, err := svc.UpdateMapping(ctx, nil, tenantID, m.ID, req); err == nil {
			t.Errorf("expected validation error for %+v", req)
		}
	}

	// Unknown mapping → 404.
	if _, err := svc.UpdateMapping(ctx, nil, tenantID, uuid.New(), dto.UpdateMappingRequest{}); err == nil {
		t.Fatal("expected not-found on unknown mapping")
	}

	// Unknown manager → 404.
	if _, err := svc.UpdateMapping(ctx, nil, tenantID, m.ID, dto.UpdateMappingRequest{ManagerUserID: strPtr(uuid.New().String())}); err == nil {
		t.Fatal("expected not-found on unknown manager")
	}

	// Unknown team → 404.
	if _, err := svc.UpdateMapping(ctx, nil, tenantID, m.ID, dto.UpdateMappingRequest{TeamID: strPtr(uuid.New().String())}); err == nil {
		t.Fatal("expected not-found on unknown team")
	}

	// Malformed user id on create → 400.
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: "bad"}, uuid.New()); err == nil {
		t.Fatal("expected validation error on malformed user_id")
	}

	// Inactive user on create → 404 (EC-05).
	inactiveID := uuid.New()
	users.users[inactiveID] = &identityDTO.UserResponse{ID: inactiveID.String(), Status: "inactive"}
	if _, err := svc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: inactiveID.String(), DepartmentID: strPtr(dept.ID.String())}, uuid.New()); err == nil {
		t.Fatal("expected not-found mapping inactive user")
	}
}

func TestMappingService_DeactivateUserRepoError(t *testing.T) {
	base := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}}
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}}
	desigs := &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}
	users := &stubUserChecker{users: map[uuid.UUID]*identityDTO.UserResponse{}}
	svc := NewMappingService(&errMappingRepo{base}, depts, teams, desigs, users, &stubPublisher{}, &stubAudit{})

	if err := svc.DeactivateUserMappings(context.Background(), nil, uuid.New(), uuid.New(), uuid.New()); !errors.Is(err, errBoom) {
		t.Fatalf("expected repo error to propagate, got %v", err)
	}
}

func TestOrgChart_IncludeInactiveAndErrors(t *testing.T) {
	depts, teams, mappings, svc := newChartSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	root := seedDept(t, depts, tenantID, "Company")
	root.Status = "inactive"
	depts.roots = []models.Department{*root}

	// Inactive root hidden by default, visible with include_inactive (FR-O004).
	res, err := svc.GetChart(ctx, nil, tenantID, 10, false)
	if err != nil || len(res.Data) != 0 {
		t.Fatalf("inactive root must be hidden by default: %+v err=%v", res, err)
	}
	res2, err := svc.GetChart(ctx, nil, tenantID, 10, true)
	if err != nil || len(res2.Data) != 1 {
		t.Fatalf("include_inactive must show inactive root: %+v err=%v", res2, err)
	}

	// Inactive team hidden by default inside an active department.
	root.Status = "active"
	depts.roots = []models.Department{*root} // roots holds copies; re-sync after status flip
	team := &models.Team{DepartmentID: root.ID, Name: "Ghost", Status: "inactive"}
	team.ID = uuid.New()
	team.TenantID = tenantID
	teams.byID[team.ID] = team
	res3, err := svc.GetChart(ctx, nil, tenantID, 10, false)
	if err != nil || len(res3.Data) != 1 || len(res3.Data[0].Teams) != 0 {
		t.Fatalf("inactive team must be hidden: %+v err=%v", res3, err)
	}
	res4, _ := svc.GetChart(ctx, nil, tenantID, 10, true)
	if len(res4.Data) != 1 || len(res4.Data[0].Teams) != 1 {
		t.Fatalf("include_inactive must show inactive team: %+v", res4)
	}
	_ = mappings
}
