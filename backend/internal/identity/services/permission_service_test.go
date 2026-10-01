package services

import (
	"context"
	"testing"

	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

func TestPermissionService_ListPermissions(t *testing.T) {
	permRepo := &MockPermissionRepository{}
	svc := NewPermissionService(permRepo)
	ctx := context.Background()

	permRepo.FindAllFunc = func(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error) {
		return []models.Permission{
			{
				Resource: "users",
				Action:   "read",
			},
		}, 1, nil
	}

	resp, err := svc.ListPermissions(ctx, nil, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 || resp.Meta.TotalItems != 1 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.Data[0].Resource != "users" || resp.Data[0].Action != "read" {
		t.Errorf("unexpected permission: %+v", resp.Data[0])
	}
}
