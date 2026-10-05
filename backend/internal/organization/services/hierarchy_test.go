package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

// depthStubRepo builds a linear chain Root -> L1 -> ... for depth/cycle tests.
type depthStubRepo struct {
	stubDeptRepo
	depthChain []uuid.UUID // depthChain[0] = root
}

// seedChain materializes a linear hierarchy of the given depth.
func seedChain(t *testing.T, tenantID uuid.UUID, depth int) (*depthStubRepo, map[uuid.UUID]*models.Department) {
	t.Helper()
	byID := map[uuid.UUID]*models.Department{}
	chain := make([]uuid.UUID, depth)
	var parent *uuid.UUID
	for i := 0; i < depth; i++ {
		d := &models.Department{Name: string(rune('A' + i)), Status: "active", ParentDepartmentID: parent}
		d.ID = uuid.New()
		d.TenantID = tenantID
		byID[d.ID] = d
		chain[i] = d.ID
		p := d.ID
		parent = &p
	}
	repo := &depthStubRepo{stubDeptRepo: stubDeptRepo{byID: byID, byName: map[string]*models.Department{}, children: map[uuid.UUID][]models.Department{}}, depthChain: chain}
	return repo, byID
}

func TestHierarchyDepthCap(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()

	// Chain of 11 rooted departments: parenting under the 11th walks 11 > cap (FR-H004).
	repo, _ := seedChain(t, tenantID, 11)
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewDepartmentService(repo, teams, mappings, &stubPublisher{}, &stubAudit{})

	deepest := repo.depthChain[10]
	pid := deepest.String()
	if _, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "TooDeep", ParentDepartmentID: &pid}, uuid.New()); err == nil {
		t.Fatal("expected HIERARCHY_TOO_DEEP at walk depth 11")
	}

	// Chain of 10: parenting under the 10th node walks 10 (== cap) → allowed.
	repo10, _ := seedChain(t, tenantID, 10)
	svc10 := NewDepartmentService(repo10, teams, mappings, &stubPublisher{}, &stubAudit{})
	ninth := repo10.depthChain[9].String()
	res, err := svc10.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Level11", ParentDepartmentID: &ninth}, uuid.New())
	if err != nil {
		t.Fatalf("walk-10 child must be allowed: %v", err)
	}
	if res.Depth != 10 {
		t.Errorf("expected depth 10 for 11th node, got %d", res.Depth)
	}
}

func TestHierarchyMoveValidation(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	repo, _ := seedChain(t, tenantID, 4) // A -> B -> C -> D
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewDepartmentService(repo, teams, mappings, &stubPublisher{}, &stubAudit{})

	root, lvl1, lvl2, leaf := repo.depthChain[0], repo.depthChain[1], repo.depthChain[2], repo.depthChain[3]

	// Move root under leaf → cycle (FR-H003/EC-03).
	leafStr := leaf.String()
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, root, dto.UpdateDepartmentRequest{ParentDepartmentID: &leafStr}); err == nil {
		t.Fatal("expected cycle error moving root under descendant")
	}
	// Move middle under leaf → cycle.
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, lvl1, dto.UpdateDepartmentRequest{ParentDepartmentID: &leafStr}); err == nil {
		t.Fatal("expected cycle error moving middle under descendant")
	}
	// Self-parent → 400.
	self := lvl2.String()
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, lvl2, dto.UpdateDepartmentRequest{ParentDepartmentID: &self}); err == nil {
		t.Fatal("expected self-parent error")
	}
	// Legal move: leaf to root → succeeds, new depth 1.
	rootStr := root.String()
	moved, err := svc.UpdateDepartment(ctx, nil, tenantID, leaf, dto.UpdateDepartmentRequest{ParentDepartmentID: &rootStr})
	if err != nil {
		t.Fatalf("legal move failed: %v", err)
	}
	if moved.Depth != 1 {
		t.Errorf("expected depth 1 after move to root, got %d", moved.Depth)
	}
	// Clear parent → root again.
	empty := ""
	cleared, err := svc.UpdateDepartment(ctx, nil, tenantID, leaf, dto.UpdateDepartmentRequest{ParentDepartmentID: &empty})
	if err != nil || cleared.Depth != 0 {
		t.Fatalf("clear parent: %+v err=%v", cleared, err)
	}
}

func TestDeactivateWithActiveMappingsGuard(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}, byName: map[string]*models.Department{}, children: map[uuid.UUID][]models.Department{}}
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewDepartmentService(depts, teams, mappings, &stubPublisher{}, &stubAudit{})

	res, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Ops"}, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := mustParse(res.ID)

	// Active mapping referencing the department blocks non-force deactivate (FR-D007).
	m := &models.Mapping{UserID: uuid.New(), DepartmentID: &id, Status: "active"}
	m.ID = uuid.New()
	m.TenantID = tenantID
	mappings.byID[m.ID] = m
	if _, err := svc.DeactivateDepartment(ctx, nil, tenantID, id, false, nil); err == nil {
		t.Fatal("expected conflict with active mappings")
	}
	// Force still succeeds (children cascade only).
	if _, err := svc.DeactivateDepartment(ctx, nil, tenantID, id, true, strPtr("restructure")); err != nil {
		t.Fatalf("force deactivate with mapping refs: %v", err)
	}
}

func TestUpdateRepoErrorPropagation(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	repo, _ := seedChain(t, tenantID, 2)
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewDepartmentService(repo, teams, mappings, &stubPublisher{}, &stubAudit{})

	// Parent lookup misses → not-found.
	ghost := uuid.New().String()
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, repo.depthChain[0], dto.UpdateDepartmentRequest{ParentDepartmentID: &ghost}); err == nil {
		t.Fatal("expected not-found on missing parent")
	}
	// Unknown department id → not-found.
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, uuid.New(), dto.UpdateDepartmentRequest{Name: strPtr("Z")}); err == nil {
		t.Fatal("expected not-found on unknown department")
	}
	// Unknown department id on deactivate → not-found.
	if _, err := svc.DeactivateDepartment(ctx, nil, tenantID, uuid.New(), false, nil); err == nil {
		t.Fatal("expected not-found deactivating unknown department")
	}
}

var _ = gorm.ErrRecordNotFound
