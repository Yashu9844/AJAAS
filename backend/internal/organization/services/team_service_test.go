package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
)

func newTeamSvc() (*stubDeptRepo, *stubTeamRepo, *stubMappingRepo, *stubUserChecker, TeamService) {
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}, byName: map[string]*models.Department{}}
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	users := &stubUserChecker{users: map[uuid.UUID]*identityDTO.UserResponse{}}
	svc := NewTeamService(teams, depts, mappings, users, &stubPublisher{}, &stubAudit{})
	return depts, teams, mappings, users, svc
}

func seedDept(t *testing.T, depts *stubDeptRepo, tenantID uuid.UUID, name string) *models.Department {
	t.Helper()
	d := &models.Department{Name: name, Status: "active"}
	d.ID = uuid.New()
	d.TenantID = tenantID
	depts.byID[d.ID] = d
	depts.byName[strings.ToLower(name)] = d
	return d
}

func TestTeamService_CreateTeam(t *testing.T) {
	depts, _, _, users, svc := newTeamSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	dept := seedDept(t, depts, tenantID, "Engineering")

	res, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "Platform", DepartmentID: dept.ID.String()}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "Platform" || res.Status != "active" || res.DepartmentID != dept.ID.String() {
		t.Errorf("unexpected response: %+v", res)
	}

	// Duplicate name in the same department → conflict (FR-T002).
	if _, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "platform", DepartmentID: dept.ID.String()}, uuid.New()); err == nil {
		t.Fatal("expected conflict on duplicate team name within department")
	}

	// Same name in another department is allowed (FR-T002 scope).
	other := seedDept(t, depts, tenantID, "Product")
	if _, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "Platform", DepartmentID: other.ID.String()}, uuid.New()); err != nil {
		t.Fatalf("same name in another department must be allowed: %v", err)
	}

	// Unknown department → 404.
	if _, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "X", DepartmentID: uuid.New().String()}, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown department")
	}

	// Unknown lead user → 404 (FR-T001 via Module 0 check).
	lead := uuid.New().String()
	_ = users
	if _, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "WithLead", DepartmentID: dept.ID.String(), LeadUserID: &lead}, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown lead user")
	}
}

func TestTeamService_MoveAndDeactivate(t *testing.T) {
	depts, teams, mappings, _, svc := newTeamSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	deptA := seedDept(t, depts, tenantID, "A")
	deptB := seedDept(t, depts, tenantID, "B")

	team := &models.Team{DepartmentID: deptA.ID, Name: "Field", Status: "active"}
	team.ID = uuid.New()
	team.TenantID = tenantID
	teams.byID[team.ID] = team

	// Move across departments (FR-T005).
	res, err := svc.UpdateTeam(ctx, nil, tenantID, team.ID, dto.UpdateTeamRequest{DepartmentID: strPtr(deptB.ID.String())})
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	if res.DepartmentID != deptB.ID.String() {
		t.Errorf("expected move to %s, got %s", deptB.ID, res.DepartmentID)
	}

	// Move to unknown department → 404.
	if _, err := svc.UpdateTeam(ctx, nil, tenantID, team.ID, dto.UpdateTeamRequest{DepartmentID: strPtr(uuid.New().String())}); err == nil {
		t.Fatal("expected not-found moving to unknown department")
	}

	// Deactivate blocked while active mappings reference the team (FR-T006).
	m := &models.Mapping{UserID: uuid.New(), TeamID: &team.ID, Status: "active"}
	m.ID = uuid.New()
	m.TenantID = tenantID
	mappings.byID[m.ID] = m
	if _, err := svc.DeactivateTeam(ctx, nil, tenantID, team.ID, nil); err == nil {
		t.Fatal("expected conflict deactivating team with active mappings")
	}

	// Clear mappings → deactivation succeeds.
	m.Status = "inactive"
	if _, err := svc.DeactivateTeam(ctx, nil, tenantID, team.ID, nil); err != nil {
		t.Fatalf("expected deactivate to succeed, got: %v", err)
	}

	// Already inactive → 409.
	if _, err := svc.DeactivateTeam(ctx, nil, tenantID, team.ID, nil); err == nil {
		t.Fatal("expected conflict on double deactivate")
	}
}

func TestTeamService_GetListRenameConflict(t *testing.T) {
	depts, _, _, users, svc := newTeamSvc()
	ctx := context.Background()
	tenantID := uuid.New()
	dept := seedDept(t, depts, tenantID, "Engineering")

	a, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "Alpha", DepartmentID: dept.ID.String()}, uuid.New())
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := svc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "Beta", DepartmentID: dept.ID.String()}, uuid.New()); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	// Get found / not found.
	got, err := svc.GetTeam(ctx, nil, tenantID, mustParse(a.ID))
	if err != nil || got.Name != "Alpha" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := svc.GetTeam(ctx, nil, tenantID, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown team")
	}

	// List with and without department filter.
	list, err := svc.ListTeams(ctx, nil, tenantID, &dept.ID, 1, 20)
	if err != nil || list.Meta.TotalItems != 2 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	all, err := svc.ListTeams(ctx, nil, tenantID, nil, 1, 20)
	if err != nil || all.Meta.TotalItems != 2 {
		t.Fatalf("list all: %+v err=%v", all, err)
	}

	// Rename to existing name in same department → 409.
	if _, err := svc.UpdateTeam(ctx, nil, tenantID, mustParse(a.ID), dto.UpdateTeamRequest{Name: strPtr("beta")}); err == nil {
		t.Fatal("expected conflict renaming to existing team name")
	}

	// Lead change to an active user succeeds; unknown lead → 404.
	leadID := uuid.New()
	users.users[leadID] = activeUser(leadID)
	updated, err := svc.UpdateTeam(ctx, nil, tenantID, mustParse(a.ID), dto.UpdateTeamRequest{LeadUserID: strPtr(leadID.String())})
	if err != nil || updated.LeadUserID == nil || *updated.LeadUserID != leadID.String() {
		t.Fatalf("lead update: %+v err=%v", updated, err)
	}
	if _, err := svc.UpdateTeam(ctx, nil, tenantID, mustParse(a.ID), dto.UpdateTeamRequest{LeadUserID: strPtr(uuid.New().String())}); err == nil {
		t.Fatal("expected not-found on unknown lead")
	}
	if _, err := svc.UpdateTeam(ctx, nil, tenantID, mustParse(a.ID), dto.UpdateTeamRequest{LeadUserID: strPtr("bad-uuid")}); err == nil {
		t.Fatal("expected validation error on malformed lead id")
	}
}

func strPtr(s string) *string { return &s }
