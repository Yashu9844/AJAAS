package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
)

func newDesigSvc() (*stubDesigRepo, *stubMappingRepo, DesignationService) {
	desigs := &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	return desigs, mappings, NewDesignationService(desigs, mappings, &stubPublisher{}, &stubAudit{})
}

func TestDesignationService_CreateAndDuplicate(t *testing.T) {
	_, _, svc := newDesigSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	res, err := svc.CreateDesignation(ctx, nil, tenantID, dto.CreateDesignationRequest{Title: "Senior Engineer"}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Title != "Senior Engineer" || res.Status != "active" {
		t.Errorf("unexpected response: %+v", res)
	}

	// Duplicate title (case-insensitive) → conflict (FR-DG002).
	if _, err := svc.CreateDesignation(ctx, nil, tenantID, dto.CreateDesignationRequest{Title: "senior engineer"}, uuid.New()); err == nil {
		t.Fatal("expected conflict on duplicate title")
	}
}

func TestDesignationService_UpdateAndDeactivate(t *testing.T) {
	desigs, mappings, svc := newDesigSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	res, err := svc.CreateDesignation(ctx, nil, tenantID, dto.CreateDesignationRequest{Title: "Lead"}, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := mustParse(res.ID)

	// Rename collision with an existing title → conflict.
	if _, err := svc.CreateDesignation(ctx, nil, tenantID, dto.CreateDesignationRequest{Title: "Manager"}, uuid.New()); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := svc.UpdateDesignation(ctx, nil, tenantID, id, dto.UpdateDesignationRequest{Title: strPtr("Manager")}); err == nil {
		t.Fatal("expected conflict renaming to existing title")
	}

	// Deactivation blocked while active mappings reference it (FR-DG004).
	desig := desigs.byID[id]
	m := &models.Mapping{UserID: uuid.New(), DesignationID: &desig.ID, Status: "active"}
	m.ID = uuid.New()
	m.TenantID = tenantID
	mappings.byID[m.ID] = m
	if _, err := svc.DeactivateDesignation(ctx, nil, tenantID, id, nil); err == nil {
		t.Fatal("expected conflict deactivating referenced designation")
	}

	// Reference cleared → deactivation succeeds; second attempt → 409.
	m.Status = "inactive"
	if _, err := svc.DeactivateDesignation(ctx, nil, tenantID, id, nil); err != nil {
		t.Fatalf("expected deactivate to succeed: %v", err)
	}
	if _, err := svc.DeactivateDesignation(ctx, nil, tenantID, id, nil); err == nil {
		t.Fatal("expected conflict on double deactivate")
	}
}

func TestDesignationService_CaseInsensitiveTitleRepo(t *testing.T) {
	desigs, _, _ := newDesigSvc()
	d := &models.Designation{Title: "Architect", Status: "active"}
	d.ID = uuid.New()
	d.TenantID = uuid.New()
	desigs.byID[d.ID] = d
	desigs.byTitle[strings.ToLower(d.Title)] = d
	got, err := desigs.FindByTitle(context.Background(), nil, d.TenantID, "architect")
	if err != nil || got == nil {
		t.Fatal("stub FindByTitle must be case-insensitive")
	}
}

func TestDesignationService_GetListHappyUpdate(t *testing.T) {
	_, _, svc := newDesigSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	lvl := 3
	res, err := svc.CreateDesignation(ctx, nil, tenantID, dto.CreateDesignationRequest{Title: "Staff Engineer", Level: &lvl}, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := mustParse(res.ID)

	got, err := svc.GetDesignation(ctx, nil, tenantID, id)
	if err != nil || got.Title != "Staff Engineer" || got.Level == nil || *got.Level != 3 {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := svc.GetDesignation(ctx, nil, tenantID, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown designation")
	}

	list, err := svc.ListDesignations(ctx, nil, tenantID, 1, 20)
	if err != nil || list.Meta.TotalItems != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}

	// Happy update: title + level + description.
	updated, err := svc.UpdateDesignation(ctx, nil, tenantID, id, dto.UpdateDesignationRequest{
		Title: strPtr("Principal Engineer"), Level: intPtr(5), Description: strPtr("IC5"),
	})
	if err != nil || updated.Title != "Principal Engineer" || *updated.Level != 5 {
		t.Fatalf("update: %+v err=%v", updated, err)
	}

	// Update unknown id → 404.
	if _, err := svc.UpdateDesignation(ctx, nil, tenantID, uuid.New(), dto.UpdateDesignationRequest{Title: strPtr("X")}); err == nil {
		t.Fatal("expected not-found updating unknown designation")
	}
}

func intPtr(i int) *int { return &i }
