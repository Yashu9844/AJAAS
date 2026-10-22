package employee

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/employee/api"
	"github.com/jaas/jaas/internal/hr/employee/models"
	"gorm.io/gorm"
)

// documentsPort implements employee/api.Documents.
type documentsPort struct{}

func (documentsPort) VerifiedDocument(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, documentType string) (*uuid.UUID, error) {
	var doc models.EmployeeDocument
	err := db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND document_type = ? AND verified_at IS NOT NULL",
		tenantID, employeeID, documentType).Order("verified_at DESC").Take(&doc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &doc.ID, nil
}

// Documents is the document-verification port other HR contexts use (employee/api).
func (m *Module) Documents() api.Documents { return documentsPort{} }
