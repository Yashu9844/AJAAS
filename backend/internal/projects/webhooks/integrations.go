package webhooks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// IntegrationInput creates a git integration.
type IntegrationInput struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

// incomingPath is the path (below /api/v1) a provider posts to.
func incomingPath(provider string, id uuid.UUID) string {
	return "/integrations/" + provider + "/" + id.String()
}

// Integrations lists the integrations of a project (project.update).
func (s *Service) Integrations(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Integration, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var rows []Integration
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// CreateIntegration adds an integration (project.update); a project has at most 10. The secret is returned once.
func (s *Service) CreateIntegration(ctx context.Context, a api.Actor, projectID uuid.UUID, in IntegrationInput) (*IntegrationWithSecret, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider != ProviderGitHub && provider != ProviderGitea {
		return nil, kernel.Invalid("provider", "must be github or gitea")
	}
	name, err := cleanName(in.Name)
	if err != nil {
		return nil, err
	}
	secret, sealed, err := s.newSealedSecret()
	if err != nil {
		return nil, err
	}
	it := Integration{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Provider: provider, Name: name, SecretEnc: sealed,
		SecretHint: Hint(secret), Active: true, CreatedBy: a.UserID, CreatedAt: s.stamp(), UpdatedAt: s.stamp()}
	err = s.inTx(ctx, func(tx *gorm.DB) error { return s.insertIntegration(ctx, tx, a, &it) })
	if isDup(err) {
		return nil, kernel.Conflict("INTEGRATION_NAME_TAKEN", "an integration with this name already exists")
	}
	if err != nil {
		return nil, err
	}
	return &IntegrationWithSecret{Integration: it, Secret: secret, Path: incomingPath(it.Provider, it.ID)}, nil
}

func (s *Service) insertIntegration(ctx context.Context, tx *gorm.DB, a api.Actor, it *Integration) error {
	if err := lockProject(tx, it.ProjectID); err != nil {
		return err
	}
	n, err := countRows(tx, "integrations", "project_id = ? AND tenant_id = ?", it.ProjectID, a.TenantID)
	if err != nil {
		return err
	}
	if n >= maxIntegrationsPerProject {
		return kernel.Conflict("INTEGRATION_LIMIT", "a project has at most 10 integrations")
	}
	if err := tx.Create(it).Error; err != nil {
		return err
	}
	s.audit(ctx, tx, a, "integration.created", ref{"integration", it.ID, map[string]any{"project_id": it.ProjectID, "provider": it.Provider}})
	return nil
}

// DeleteIntegration removes an integration; the links it created stay (project.update).
func (s *Service) DeleteIntegration(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Delete(&Integration{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("integration")
		}
		s.audit(ctx, tx, a, "integration.deleted", ref{"integration", id, map[string]any{"project_id": projectID}})
		return nil
	})
}

// RotateIntegrationSecret replaces the integration secret (project.update) and returns the new one once.
func (s *Service) RotateIntegrationSecret(ctx context.Context, a api.Actor, projectID, id uuid.UUID) (*IntegrationWithSecret, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	secret, sealed, err := s.newSealedSecret()
	if err != nil {
		return nil, err
	}
	var it Integration
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		err := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&it).Error
		if err != nil {
			return mapMissing(err, "integration")
		}
		it.SecretEnc, it.SecretHint, it.UpdatedAt = sealed, Hint(secret), s.stamp()
		if err = tx.Save(&it).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "integration.secret_rotated", ref{"integration", it.ID, map[string]any{"project_id": projectID}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &IntegrationWithSecret{Integration: it, Secret: secret, Path: incomingPath(it.Provider, it.ID)}, nil
}
