package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"github.com/jaas/jaas/internal/hr/payroll/validators"
	"gorm.io/gorm"
)

// CTCPort implements payroll/api.CTC on top of the assignment rules (PY-014): it closes the open assignment and opens
// a new one with the same structure and the new CTC.
type CTCPort struct{ s *assignmentService }

// NewCTCPort builds the port.
func NewCTCPort(d Deps) *CTCPort { return &CTCPort{s: &assignmentService{base{d}}} }

// SetCTC sets the annual CTC from a date inside the caller's transaction; without a payroll assignment it is a no-op.
func (p *CTCPort) SetCTC(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, annual kernel.Money, from time.Time, actor uuid.UUID) (bool, error) {
	if err := validators.ValidateCTC(annual); err != nil {
		return false, invalid("annual_ctc", err.Error())
	}
	cur, err := p.s.Repos.Assignments.Current(ctx, tx, tenantID, employeeID)
	if err != nil || cur == nil {
		return false, err
	}
	next := &models.Assignment{TenantID: tenantID, EmployeeProfileID: employeeID, StructureID: cur.StructureID, AnnualCTC: annual,
		EffectiveFrom: from, CreatedByUserID: actor}
	if err := p.s.assignTx(ctx, tx, next); err != nil {
		return false, err
	}
	return true, nil
}
