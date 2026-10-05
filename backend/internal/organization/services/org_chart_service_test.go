package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
)

func newChartSvc() (*stubDeptRepo, *stubTeamRepo, *stubMappingRepo, OrgChartService) {
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}, byName: map[string]*models.Department{}, children: map[uuid.UUID][]models.Department{}}
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	desigs := &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}
	return depts, teams, mappings, NewOrgChartService(depts, teams, mappings, desigs, nil)
}

func TestOrgChart_EmptyTenant(t *testing.T) {
	_, _, _, svc := newChartSvc()
	res, err := svc.GetChart(context.Background(), nil, uuid.New(), 10, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || len(res.Data) != 0 {
		t.Errorf("EC-12: empty tenant must return empty data, got %+v", res)
	}
}

func TestOrgChart_ForestBuild(t *testing.T) {
	depts, teams, mappings, svc := newChartSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	root := seedDept(t, depts, tenantID, "Company")
	depts.roots = []models.Department{*root}
	child := &models.Department{Name: "Engineering", Status: "active", ParentDepartmentID: &root.ID}
	child.ID = uuid.New()
	child.TenantID = tenantID
	depts.byID[child.ID] = child
	depts.children[root.ID] = []models.Department{*child}

	team := &models.Team{DepartmentID: child.ID, Name: "Platform", Status: "active"}
	team.ID = uuid.New()
	team.TenantID = tenantID
	teams.byID[team.ID] = team

	m := &models.Mapping{UserID: uuid.New(), DepartmentID: &child.ID, TeamID: &team.ID, Status: "active"}
	m.ID = uuid.New()
	m.TenantID = tenantID
	mappings.byID[m.ID] = m

	res, err := svc.GetChart(ctx, nil, tenantID, 10, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Data) != 1 || res.Data[0].Name != "Company" {
		t.Fatalf("expected single root Company, got %+v", res.Data)
	}
	eng := res.Data[0].Children
	if len(eng) != 1 || eng[0].Name != "Engineering" {
		t.Fatalf("expected nested Engineering child, got %+v", eng)
	}
	if eng[0].Headcount != 1 {
		t.Errorf("expected headcount 1 on Engineering, got %d", eng[0].Headcount)
	}
	if len(eng[0].Teams) != 1 || eng[0].Teams[0].MemberCount != 1 {
		t.Errorf("expected team Platform with 1 member, got %+v", eng[0].Teams)
	}

	// Depth pruning: maxDepth=1 returns roots without children.
	pruned, err := svc.GetChart(ctx, nil, tenantID, 1, false)
	if err != nil {
		t.Fatalf("pruned chart: %v", err)
	}
	if len(pruned.Data) != 1 || len(pruned.Data[0].Children) != 0 {
		t.Errorf("FR-O004: max_depth=1 must prune children, got %+v", pruned.Data[0])
	}
}

func TestOrgChart_UserChain(t *testing.T) {
	depts, teams, mappings, svc := newChartSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	root := seedDept(t, depts, tenantID, "Company")
	child := &models.Department{Name: "Engineering", Status: "active", ParentDepartmentID: &root.ID}
	child.ID = uuid.New()
	child.TenantID = tenantID
	depts.byID[child.ID] = child

	team := &models.Team{DepartmentID: child.ID, Name: "Platform", Status: "active"}
	team.ID = uuid.New()
	team.TenantID = tenantID
	teams.byID[team.ID] = team

	uid := uuid.New()
	m := &models.Mapping{UserID: uid, DepartmentID: &child.ID, TeamID: &team.ID, IsPrimary: true, Status: "active"}
	m.ID = uuid.New()
	m.TenantID = tenantID
	mappings.byID[m.ID] = m
	mappings.primary[uid] = m

	reportID := uuid.New()
	r := &models.Mapping{UserID: reportID, ManagerUserID: &uid, Status: "active"}
	r.ID = uuid.New()
	r.TenantID = tenantID
	mappings.byID[r.ID] = r

	res, err := svc.GetUserChain(ctx, nil, tenantID, uid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Department == nil || res.Department.Name != "Engineering" {
		t.Errorf("expected department Engineering, got %+v", res.Department)
	}
	if res.Team == nil || res.Team.Name != "Platform" {
		t.Errorf("expected team Platform, got %+v", res.Team)
	}
	if len(res.Ancestors) != 1 || res.Ancestors[0].Name != "Company" {
		t.Errorf("expected ancestor Company, got %+v", res.Ancestors)
	}
	if len(res.DirectReports) != 1 || res.DirectReports[0] != reportID.String() {
		t.Errorf("expected direct report %s, got %+v", reportID, res.DirectReports)
	}

	// Unmapped user → empty chain, no error (FR-O002).
	empty, err := svc.GetUserChain(ctx, nil, tenantID, uuid.New())
	if err != nil || empty.Mapping != nil || len(empty.DirectReports) != 0 {
		t.Errorf("unmapped user must return empty chain, got %+v err=%v", empty, err)
	}
}

// fakeChartCache is an in-memory chartCache for tests.
type fakeChartCache struct {
	store map[string]string
	gets  int
	sets  int
}

func (f *fakeChartCache) Get(ctx context.Context, key string) (string, bool, error) {
	f.gets++
	v, ok := f.store[key]
	return v, ok, nil
}
func (f *fakeChartCache) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	f.sets++
	f.store[key] = value
	return nil
}

func TestOrgChart_CacheHitAndSet(t *testing.T) {
	depts, teams, mappings, _ := newChartSvc()
	cache := &fakeChartCache{store: map[string]string{}}
	desigs := &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}
	svc := NewOrgChartService(depts, teams, mappings, desigs, cache)

	ctx := context.Background()
	tenantID := uuid.New()
	root := seedDept(t, depts, tenantID, "Company")
	depts.roots = []models.Department{*root}

	// First call computes and caches; second call hits the cache (FR-O003).
	first, err := svc.GetChart(ctx, nil, tenantID, 10, false)
	if err != nil || len(first.Data) != 1 {
		t.Fatalf("first chart: %+v err=%v", first, err)
	}
	if cache.sets != 1 {
		t.Fatalf("expected cache set after compute, got %d", cache.sets)
	}

	// Poison the repo: a cache hit must return the same payload without touching it.
	depts.roots = nil
	second, err := svc.GetChart(ctx, nil, tenantID, 10, false)
	if err != nil || len(second.Data) != 1 || second.Data[0].Name != "Company" {
		t.Fatalf("cache hit must serve stored payload, got %+v err=%v", second, err)
	}
	if cache.sets != 1 {
		t.Errorf("cache hit must not recompute/set, sets=%d", cache.sets)
	}
}

func TestDepartmentService_ForceCascadeDeactivatesChildren(t *testing.T) {
	depts, _, _, svc := newDeptSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	parent, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "HQ"}, uuid.New())
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Region", ParentDepartmentID: &parent.ID}, uuid.New())
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	// Register child in the stub's children index (service Create only writes byID/byName).
	pid := mustParse(parent.ID)
	depts.children[pid] = []models.Department{*depts.byID[mustParse(child.ID)]}

	// Force=true cascades to child departments only (FR-D007).
	if _, err := svc.DeactivateDepartment(ctx, nil, tenantID, pid, true, strPtr("reorg")); err != nil {
		t.Fatalf("force deactivate: %v", err)
	}
	if depts.byID[pid].Status != "inactive" {
		t.Error("expected parent inactive")
	}
	if got := depts.byID[mustParse(child.ID)].Status; got != "inactive" {
		// children slice holds a copy; the service updates via repo Update — verify through byID
		t.Errorf("expected child inactive, got %q", got)
	}
}
