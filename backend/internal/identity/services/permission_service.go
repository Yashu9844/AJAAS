package services

import (
	"context"

	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/repositories"
	"gorm.io/gorm"
)

// PermissionService lists global privilege mappings.
type PermissionService interface {
	ListPermissions(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error)
}

type permissionService struct {
	permRepo repositories.PermissionRepository
}

// NewPermissionService creates a new PermissionService.
func NewPermissionService(permRepo repositories.PermissionRepository) PermissionService {
	return &permissionService{permRepo: permRepo}
}

func (s *permissionService) ListPermissions(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error) {
	perms, total, err := s.permRepo.FindAll(ctx, db, page, perPage)
	if err != nil {
		return nil, err
	}

	data := make([]dto.PermissionResponse, len(perms))
	for i := range perms {
		data[i] = dto.PermissionResponse{
			ID:          perms[i].ID.String(),
			Resource:    perms[i].Resource,
			Action:      perms[i].Action,
			Description: perms[i].Description,
		}
	}

	totalPages := int(total / int64(perPage))
	if total%int64(perPage) != 0 {
		totalPages++
	}

	return &dto.PermissionListResponse{
		Data: data,
		Meta: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}
