package services

import (
	"context"

	"github.com/google/uuid"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"github.com/jaas/jaas/internal/organization/repositories"
	"github.com/jaas/jaas/internal/organization/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// userChecker validates user existence/activity through Module 0 (injected, mocked in tests).
type userChecker interface {
	GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*identityDTO.UserResponse, error)
}

// auditLogger records org mutations through Module 0 audit transport.
type auditLogger interface {
	Log(ctx context.Context, tx *gorm.DB, tenantID, userID, action, resource, resourceID string, metadata interface{}, ip, userAgent string) error
}

// parseUUID parses a UUID string pointer, returning nil when unset.
func parseUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// uuidToString renders a UUID pointer for DTOs.
func uuidToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// assertDeptAcyclic walks ancestors to reject cycles and enforce the depth cap. FR-H001..FR-H004.
func assertDeptAcyclic(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptRepo repositories.DepartmentRepository, startParent *uuid.UUID) error {
	depth := 0
	seen := map[uuid.UUID]bool{}
	cur := startParent
	for cur != nil {
		if seen[*cur] {
			return &sharedErrors.AppError{Code: "CONFLICT", Message: "hierarchy.cycle: department parent chain contains a cycle", StatusCode: 409}
		}
		seen[*cur] = true
		depth++
		if depth > validators.MaxHierarchyDepth {
			return &sharedErrors.AppError{Code: "HIERARCHY_TOO_DEEP", Message: "department hierarchy exceeds maximum depth", StatusCode: 422}
		}
		parent, err := deptRepo.FindByID(ctx, db, tenantID, *cur)
		if err != nil || parent == nil {
			return err
		}
		cur = parent.ParentDepartmentID
	}
	return nil
}

// deptDepth computes depth of a department (root = 0).
func deptDepth(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptRepo repositories.DepartmentRepository, dept *models.Department) (int, []string, error) {
	depth := 0
	path := []string{}
	cur := dept
	seen := map[uuid.UUID]bool{}
	for cur.ParentDepartmentID != nil {
		if seen[*cur.ParentDepartmentID] {
			break
		}
		seen[*cur.ParentDepartmentID] = true
		path = append([]string{cur.ParentDepartmentID.String()}, path...)
		depth++
		parent, err := deptRepo.FindByID(ctx, db, tenantID, *cur.ParentDepartmentID)
		if err != nil || parent == nil {
			return depth, path, err
		}
		cur = parent
	}
	return depth, path, nil
}

func mapDeptToResponse(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptRepo repositories.DepartmentRepository, d *models.Department) (*dto.DepartmentResponse, error) {
	depth, path, err := deptDepth(ctx, db, tenantID, deptRepo, d)
	if err != nil {
		return nil, err
	}
	return &dto.DepartmentResponse{
		ID:                 d.ID.String(),
		TenantID:           d.TenantID.String(),
		Name:               d.Name,
		Code:               d.Code,
		Description:        d.Description,
		ParentDepartmentID: uuidToString(d.ParentDepartmentID),
		Status:             d.Status,
		Depth:              depth,
		Path:               path,
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
	}, nil
}

// publishOrOutbox publishes an org event; queue failure never blocks the API (FR-E005).
func publishOrOutbox(ctx context.Context, publisher queue.EventPublisher, exchange, routingKey string, evt interface{}) {
	_ = publisher.Publish(ctx, exchange, routingKey, evt)
}
