package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

func TestTenantService_CreateTenant(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	roleRepo := &MockRoleRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}

	svc := NewTenantService(tenantRepo, roleRepo, publisher, auditSvc)
	ctx := context.Background()
	correlationID := uuid.New()

	// 1. Success case
	tenantRepo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		return nil, nil // slug available
	}
	tenantRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
		tenant.ID = uuid.New()
		return nil
	}
	roleRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, role *models.Role) error {
		return nil
	}

	req := dto.CreateTenantRequest{
		Name: "Acme Corp",
		Slug: "acme",
	}

	resp, err := svc.CreateTenant(ctx, nil, req, correlationID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "Acme Corp" || resp.Slug != "acme" || resp.Status != "active" {
		t.Errorf("unexpected response: %+v", resp)
	}

	// 2. Invalid slug format
	req.Slug = "Invalid_Slug!"
	_, err = svc.CreateTenant(ctx, nil, req, correlationID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 3. Reserved slug
	req.Slug = "admin"
	_, err = svc.CreateTenant(ctx, nil, req, correlationID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 4. Duplicate slug
	req.Slug = "acme"
	tenantRepo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		return &models.Tenant{Slug: "acme"}, nil
	}
	_, err = svc.CreateTenant(ctx, nil, req, correlationID)
	if !errors.Is(err, sharedErrors.ErrConflict) {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestTenantService_GetTenant(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	svc := NewTenantService(tenantRepo, nil, nil, nil)
	ctx := context.Background()
	id := uuid.New()

	// FindByID - Success
	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, targetID uuid.UUID) (*models.Tenant, error) {
		if targetID == id {
			return &models.Tenant{Name: "Acme"}, nil
		}
		return nil, nil
	}
	resp, err := svc.GetTenantByID(ctx, nil, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "Acme" {
		t.Errorf("expected Acme, got %s", resp.Name)
	}

	// FindByID - Not Found
	_, err = svc.GetTenantByID(ctx, nil, uuid.New())
	if !errors.Is(err, sharedErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// FindBySlug - Success
	tenantRepo.FindBySlugFunc = func(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error) {
		if slug == "acme" {
			return &models.Tenant{Name: "Acme"}, nil
		}
		return nil, nil
	}
	resp, err = svc.GetTenantBySlug(ctx, nil, "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "Acme" {
		t.Errorf("expected Acme, got %s", resp.Name)
	}

	// FindBySlug - Not Found
	_, err = svc.GetTenantBySlug(ctx, nil, "nonexistent")
	if !errors.Is(err, sharedErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTenantService_ListTenants(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	svc := NewTenantService(tenantRepo, nil, nil, nil)
	ctx := context.Background()

	tenantRepo.FindAllFunc = func(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error) {
		return []models.Tenant{{Name: "Acme"}}, 1, nil
	}

	resp, err := svc.ListTenants(ctx, nil, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 || resp.Meta.TotalItems != 1 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestTenantService_UpdateTenant(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	svc := NewTenantService(tenantRepo, nil, nil, nil)
	ctx := context.Background()
	id := uuid.New()

	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, targetID uuid.UUID) (*models.Tenant, error) {
		return &models.Tenant{Name: "Acme"}, nil
	}
	tenantRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
		return nil
	}

	newName := "Acme Updated"
	resp, err := svc.UpdateTenant(ctx, nil, id, dto.UpdateTenantRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != newName {
		t.Errorf("expected name to be updated, got %s", resp.Name)
	}
}

func TestTenantService_ActivateSuspendTenant(t *testing.T) {
	tenantRepo := &MockTenantRepository{}
	publisher := &MockEventPublisher{}
	auditSvc := &MockAuditService{}
	svc := NewTenantService(tenantRepo, nil, publisher, auditSvc)
	ctx := context.Background()
	id := uuid.New()

	var updatedStatus string
	tenantRepo.UpdateFunc = func(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error {
		updatedStatus = tenant.Status
		return nil
	}

	// 1. Activate suspended tenant (Success)
	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, targetID uuid.UUID) (*models.Tenant, error) {
		return &models.Tenant{Status: "suspended"}, nil
	}
	resp, err := svc.ActivateTenant(ctx, nil, id, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "active" || updatedStatus != "active" {
		t.Errorf("unexpected status: %s", resp.Status)
	}

	// Activate active tenant (Conflict)
	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, targetID uuid.UUID) (*models.Tenant, error) {
		return &models.Tenant{Status: "active"}, nil
	}
	_, err = svc.ActivateTenant(ctx, nil, id, uuid.New())
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	// 2. Suspend active tenant (Success)
	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, targetID uuid.UUID) (*models.Tenant, error) {
		return &models.Tenant{Status: "active"}, nil
	}
	resp, err = svc.SuspendTenant(ctx, nil, id, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "suspended" || updatedStatus != "suspended" {
		t.Errorf("unexpected status: %s", resp.Status)
	}

	// Suspend suspended tenant (Conflict)
	tenantRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, targetID uuid.UUID) (*models.Tenant, error) {
		return &models.Tenant{Status: "suspended"}, nil
	}
	_, err = svc.SuspendTenant(ctx, nil, id, uuid.New())
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}
