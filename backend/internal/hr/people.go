package hr

import (
	"context"

	"github.com/google/uuid"
)

// PeopleFacts answers organisation questions for other services (M2 project group rules) from live HR data. The
// methods match projects/ports.People structurally, so HR never imports M2.
type PeopleFacts struct{ m *Module }

// People returns the organisation facts view of HR.
func (m *Module) People() PeopleFacts { return PeopleFacts{m: m} }

// DepartmentMembers lists the users placed in the department.
func (p PeopleFacts) DepartmentMembers(ctx context.Context, tenantID, departmentID uuid.UUID) ([]uuid.UUID, error) {
	return p.m.org.DepartmentUserIDs(ctx, tenantID, departmentID)
}

// TeamMembers lists the users placed in the team.
func (p PeopleFacts) TeamMembers(ctx context.Context, tenantID, teamID uuid.UUID) ([]uuid.UUID, error) {
	return p.m.org.TeamUserIDs(ctx, tenantID, teamID)
}

// DirectReports lists the users whose primary placement reports to the manager.
func (p PeopleFacts) DirectReports(ctx context.Context, tenantID, managerUserID uuid.UUID) ([]uuid.UUID, error) {
	return p.m.org.DirectReportUserIDs(ctx, tenantID, managerUserID)
}
