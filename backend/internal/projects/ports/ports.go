// Package ports declares what M2 needs from the rest of the platform. Adapters for them live in the composition root
// (internal/app), so M2 itself never imports another module.
//
//   - Identity: internal/shared/identityport (login identities, request guards, permission registration).
//   - Storage:  internal/shared/storage (presigned uploads and downloads).
//   - People:   below, the HR facts M2 reads for group membership rules.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// People answers organisation questions for project membership rules (department, team and reports-of groups). The HR
// adapter resolves them from live HR data; M2 stores only the rule and resolves the user ids when it needs them.
type People interface {
	DepartmentMembers(ctx context.Context, tenantID, departmentID uuid.UUID) ([]uuid.UUID, error)
	TeamMembers(ctx context.Context, tenantID, teamID uuid.UUID) ([]uuid.UUID, error)
	// DirectReports lists the users whose primary active placement reports to managerUserID.
	DirectReports(ctx context.Context, tenantID, managerUserID uuid.UUID) ([]uuid.UUID, error)
}

// Names resolves display names ("First Last") for users of a tenant, so a project can show who is a member or an author
// without the UI needing access to the user directory. It never returns e-mail addresses. Implementations omit unknown ids.
type Names interface {
	DisplayNames(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
}
