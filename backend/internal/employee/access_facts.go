package employee

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/models"
)

// employedStatuses are the profile statuses of someone currently employed (automatic Employee role,
// docs/ARCHITECTURE.md §17).
var employedStatuses = []string{"active", "probation", "notice"}

// EmployedUserIDs lists the users of the tenant with a current employee record.
func (m *Module) EmployedUserIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := m.db.WithContext(ctx).Model(&models.EmployeeProfile{}).
		Where("tenant_id = ? AND status IN ?", tenantID, employedStatuses).Pluck("user_id", &ids).Error
	return ids, err
}

// IsEmployed reports whether the user has a current employee record in the tenant.
func (m *Module) IsEmployed(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	var n int64
	err := m.db.WithContext(ctx).Model(&models.EmployeeProfile{}).
		Where("tenant_id = ? AND user_id = ? AND status IN ?", tenantID, userID, employedStatuses).Count(&n).Error
	return n > 0, err
}
