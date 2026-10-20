package organization

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/models"
)

// ManagerUserIDs lists the users of the tenant who currently have at least one active direct report (automatic
// Manager role, docs/ARCHITECTURE.md §17).
func (m *Module) ManagerUserIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := m.db.WithContext(ctx).Model(&models.Mapping{}).
		Where("tenant_id = ? AND status = ? AND manager_user_id IS NOT NULL", tenantID, "active").
		Distinct().Pluck("manager_user_id", &ids).Error
	return ids, err
}

// HasDirectReports reports whether the user manages at least one active mapping in the tenant.
func (m *Module) HasDirectReports(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	var n int64
	err := m.db.WithContext(ctx).Model(&models.Mapping{}).
		Where("tenant_id = ? AND status = ? AND manager_user_id = ?", tenantID, "active", userID).Count(&n).Error
	return n > 0, err
}
