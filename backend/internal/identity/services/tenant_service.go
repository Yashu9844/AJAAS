package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/jaas/jaas/internal/identity/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// TenantService handles tenant lifecycle orchestrations.
type TenantService interface {
	CreateTenant(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, correlationID uuid.UUID) (*dto.TenantResponse, error)
	GetTenantByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*dto.TenantResponse, error)
	GetTenantBySlug(ctx context.Context, db *gorm.DB, slug string) (*dto.TenantResponse, error)
	ListTenants(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error)
	UpdateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error)
	ActivateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)
	SuspendTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)
}

type tenantService struct {
	tenantRepo repositories.TenantRepository
	roleRepo   repositories.RoleRepository
	publisher  queue.EventPublisher
	auditSvc   AuditService
}

// NewTenantService creates a new TenantService.
func NewTenantService(
	tenantRepo repositories.TenantRepository,
	roleRepo repositories.RoleRepository,
	publisher queue.EventPublisher,
	auditSvc AuditService,
) TenantService {
	return &tenantService{
		tenantRepo: tenantRepo,
		roleRepo:   roleRepo,
		publisher:  publisher,
		auditSvc:   auditSvc,
	}
}

func (s *tenantService) CreateTenant(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, correlationID uuid.UUID) (*dto.TenantResponse, error) {
	// 1. Validate slug regex
	if !validators.ValidateSlug(req.Slug) {
		return nil, &sharedErrors.AppError{
			Code:       "VALIDATION_ERROR",
			Message:    "Invalid tenant slug format",
			StatusCode: 400,
		}
	}

	// 2. Check reserved words
	if validators.IsReservedSlug(req.Slug) {
		return nil, &sharedErrors.AppError{
			Code:       "CONFLICT",
			Message:    "Slug is a reserved word",
			StatusCode: 409,
		}
	}

	// 3. Validate slug unique index
	existingSlug, err := s.tenantRepo.FindBySlug(ctx, tx, req.Slug)
	if err != nil {
		return nil, err
	}
	if existingSlug != nil {
		return nil, sharedErrors.ErrConflict
	}

	tenant := &models.Tenant{
		Name:   req.Name,
		Slug:   req.Slug,
		Status: "active",
		Plan:   "free",
	}
	if req.Domain != "" {
		tenant.Domain = &req.Domain
	}
	if req.Plan != "" {
		tenant.Plan = req.Plan
	}

	// Create Tenant
	if err := s.tenantRepo.Create(ctx, tx, tenant); err != nil {
		return nil, err
	}

	// Create Default System Roles for Tenant (tenant_admin and member)
	roles := []models.Role{
		{
			TenantID:    tenant.ID,
			Name:        "tenant_admin",
			Description: pointerString("System role with full administrative permissions"),
			IsSystem:    true,
		},
		{
			TenantID:    tenant.ID,
			Name:        "member",
			Description: pointerString("Default system role for tenant members"),
			IsSystem:    true,
		},
	}
	for i := range roles {
		if err := s.roleRepo.Create(ctx, tx, &roles[i]); err != nil {
			return nil, fmt.Errorf("failed to auto-create default system role %s: %w", roles[i].Name, err)
		}
	}

	res := mapTenantToResponse(tenant)

	// Publish Event & Audit Log inside transaction wrapper
	eventPayload := events.TenantCreatedPayload{
		TenantID:  tenant.ID,
		Name:      tenant.Name,
		Slug:      tenant.Slug,
		Plan:      tenant.Plan,
		CreatedAt: tenant.CreatedAt,
	}
	evt := events.NewEvent(events.TypeTenantCreated, events.RoutingKeyTenantCreated, tenant.ID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyTenantCreated, evt)

	_ = s.auditSvc.Log(ctx, tx, tenant.ID.String(), "", "tenant.created", "tenant", tenant.ID.String(), eventPayload, "", "")

	return res, nil
}

func (s *tenantService) GetTenantByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*dto.TenantResponse, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return mapTenantToResponse(tenant), nil
}

func (s *tenantService) GetTenantBySlug(ctx context.Context, db *gorm.DB, slug string) (*dto.TenantResponse, error) {
	tenant, err := s.tenantRepo.FindBySlug(ctx, db, slug)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return mapTenantToResponse(tenant), nil
}

func (s *tenantService) ListTenants(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error) {
	tenants, total, err := s.tenantRepo.FindAll(ctx, db, page, perPage)
	if err != nil {
		return nil, err
	}

	data := make([]dto.TenantResponse, len(tenants))
	for i := range tenants {
		data[i] = *mapTenantToResponse(&tenants[i])
	}

	totalPages := int(total / int64(perPage))
	if total%int64(perPage) != 0 {
		totalPages++
	}

	return &dto.TenantListResponse{
		Data: data,
		Meta: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *tenantService) UpdateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if req.Name != nil {
		tenant.Name = *req.Name
	}
	if req.Domain != nil {
		tenant.Domain = req.Domain
	}
	if req.Plan != nil {
		tenant.Plan = *req.Plan
	}

	if err := s.tenantRepo.Update(ctx, tx, tenant); err != nil {
		return nil, err
	}

	return mapTenantToResponse(tenant), nil
}

func (s *tenantService) ActivateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if tenant.Status != "suspended" {
		return nil, &sharedErrors.AppError{
			Code:       "CONFLICT",
			Message:    "Tenant is not suspended",
			StatusCode: 409,
		}
	}

	tenant.Status = "active"
	if err := s.tenantRepo.Update(ctx, tx, tenant); err != nil {
		return nil, err
	}

	eventPayload := events.TenantActivatedPayload{
		TenantID:       tenant.ID,
		PreviousStatus: "suspended",
		NewStatus:      "active",
		ActivatedAt:    time.Now(),
	}
	evt := events.NewEvent(events.TypeTenantActivated, events.RoutingKeyTenantActivated, tenant.ID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyTenantActivated, evt)

	_ = s.auditSvc.Log(ctx, tx, tenant.ID.String(), "", "tenant.activated", "tenant", tenant.ID.String(), eventPayload, "", "")

	return mapTenantToResponse(tenant), nil
}

func (s *tenantService) SuspendTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if tenant.Status != "active" {
		return nil, &sharedErrors.AppError{
			Code:       "CONFLICT",
			Message:    "Tenant is not active",
			StatusCode: 409,
		}
	}

	tenant.Status = "suspended"
	if err := s.tenantRepo.Update(ctx, tx, tenant); err != nil {
		return nil, err
	}

	eventPayload := events.TenantSuspendedPayload{
		TenantID:       tenant.ID,
		PreviousStatus: "active",
		NewStatus:      "suspended",
		SuspendedAt:    time.Now(),
	}
	evt := events.NewEvent(events.TypeTenantSuspended, events.RoutingKeyTenantSuspended, tenant.ID, correlationID, eventPayload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyTenantSuspended, evt)

	_ = s.auditSvc.Log(ctx, tx, tenant.ID.String(), "", "tenant.suspended", "tenant", tenant.ID.String(), eventPayload, "", "")

	return mapTenantToResponse(tenant), nil
}

func mapTenantToResponse(t *models.Tenant) *dto.TenantResponse {
	return &dto.TenantResponse{
		ID:        t.ID.String(),
		Name:      t.Name,
		Slug:      t.Slug,
		Domain:    t.Domain,
		Status:    t.Status,
		Plan:      t.Plan,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func pointerString(s string) *string {
	return &s
}
