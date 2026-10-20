package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/employee"
	"github.com/jaas/jaas/internal/identity"
	"github.com/jaas/jaas/internal/hr/organization"
)

// automaticRoleSweep is how often every tenant's automatic roles are reconciled; logins sync the user immediately.
const automaticRoleSweep = 5 * time.Minute

// hrRoleFacts adapts HR/Employee and HR/Organization to identity's AutomaticRoleSource: employees hold the automatic
// Employee role, people with direct reports the Manager role (docs/ARCHITECTURE.md §17).
type hrRoleFacts struct {
	employees *employee.Module
	org       *organization.Module
}

func (f hrRoleFacts) Members(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	emps, err := f.employees.EmployedUserIDs(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	mgrs, err := f.org.ManagerUserIDs(ctx, tenantID)
	return emps, mgrs, err
}

func (f hrRoleFacts) MemberOf(ctx context.Context, tenantID, userID uuid.UUID) (bool, bool, error) {
	emp, err := f.employees.IsEmployed(ctx, tenantID, userID)
	if err != nil {
		return false, false, err
	}
	mgr, err := f.org.HasDirectReports(ctx, tenantID, userID)
	return emp, mgr, err
}

// wireAutomaticRoles connects the HR facts and, when background work is enabled, starts the periodic sweep.
func wireAutomaticRoles(o Options, id *identity.Module, emp *employee.Module, org *organization.Module) {
	id.SetAutomaticRoleSource(hrRoleFacts{employees: emp, org: org})
	if o.Background != nil {
		go id.RunAutomaticRoleSync(o.Background, automaticRoleSweep)
	}
}
