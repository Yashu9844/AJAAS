package organization

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/organization/models"
	"gorm.io/gorm"
)

// The methods below are what HR/Core needs from HR/Organization (core design C3). The HR root adapts them to core's
// port, so core never imports this package.

// DepartmentState reports whether the department exists in the tenant and whether it is active.
func (m *Module) DepartmentState(ctx context.Context, tenantID, id uuid.UUID) (exists, active bool, err error) {
	d, err := m.deptRepo.FindByID(ctx, m.db, tenantID, id)
	if err != nil || d == nil {
		return false, false, err
	}
	return true, d.Status == "active", nil
}

// LocationState reports whether the location exists in the tenant and whether it is active (FR-L).
func (m *Module) LocationState(ctx context.Context, tenantID, id uuid.UUID) (exists, active bool, err error) {
	l, err := m.locRepo.FindByID(ctx, m.db, tenantID, id)
	if err != nil || l == nil {
		return false, false, err
	}
	return true, l.Status == "active", nil
}

// PrimaryPlacement returns the department, location and manager user of the user's active primary mapping (all nil
// when there is none). HR/Core back-fill reads it (CO-042).
func (m *Module) PrimaryPlacement(ctx context.Context, tenantID, userID uuid.UUID) (dept, location, managerUser *uuid.UUID, err error) {
	p, err := m.mappingRepo.FindPrimaryByUser(ctx, m.db, tenantID, userID)
	if err != nil || p == nil || p.Status != "active" {
		return nil, nil, nil, err
	}
	return p.DepartmentID, p.LocationID, p.ManagerUserID, nil
}

// ProjectPrimaryMapping makes the user's active primary mapping reflect HR/Core's assignment (core CO-040): it updates
// department, location and manager of the existing primary mapping (dropping a team that belongs to another
// department) or creates one. Without a department and without a mapping there is nothing to project. It runs in the
// caller's transaction and drops the cached org chart.
func (m *Module) ProjectPrimaryMapping(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, dept, location, managerUser *uuid.UUID) error {
	p, err := m.mappingRepo.FindPrimaryByUser(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	if p == nil && dept != nil {
		// The primary index ignores status: a deactivated primary (e.g. after a termination) is reactivated, not duplicated.
		var old models.Mapping
		q := tx.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND is_primary = TRUE", tenantID, userID).Order("updated_at DESC").Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			old.Status, old.Reason, p = "active", nil, &old
		}
	}
	if p != nil {
		if p.TeamID != nil && dept != nil && (p.DepartmentID == nil || *p.DepartmentID != *dept) {
			p.TeamID = nil
		}
		if dept != nil {
			p.DepartmentID = dept
		}
		p.LocationID, p.ManagerUserID = location, managerUser
		err = m.mappingRepo.Update(ctx, tx, p)
	} else if dept != nil {
		mp := &models.Mapping{UserID: userID, DepartmentID: dept, LocationID: location, ManagerUserID: managerUser, IsPrimary: true, Status: "active"}
		mp.TenantID = tenantID
		err = m.mappingRepo.Create(ctx, tx, mp)
	}
	if err == nil {
		if inv := m.ChartInvalidator(); inv != nil {
			inv.InvalidateChart(ctx, tenantID)
		}
	}
	return err
}
