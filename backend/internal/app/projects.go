package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jaas/jaas/internal/projects"
	"github.com/jaas/jaas/internal/projects/ports"
	"github.com/jaas/jaas/internal/projects/webhooks"
	"github.com/jaas/jaas/internal/shared/config"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/storage"
)

// buildProjects wires M2 Projects & Tasks, including the object-storage provider when one is configured.
func buildProjects(o Options, idp identityport.Port, people ports.People) (*projects.Module, error) {
	var store storage.Store
	if sc := o.Config.Storage; sc.Bucket != "" {
		s3, err := storage.NewS3(context.Background(), storage.S3Config{Endpoint: sc.Endpoint, Region: sc.Region, Bucket: sc.Bucket,
			AccessKey: sc.AccessKey, SecretKey: sc.SecretKey, PathStyle: sc.PathStyle})
		if err != nil {
			return nil, fmt.Errorf("object storage: %w", err)
		}
		store = s3
	}
	opts := projects.Options{DB: o.DB, Publisher: o.Publisher, Logger: &o.Logger.Logger,
		Identity: idp, Storage: store, People: people, Background: o.Background, SecretsKey: projectsSecretsKey(o.Config)}
	if n, ok := idp.(ports.Names); ok { // the in-process identity adapter can resolve display names
		opts.Names = n
	}
	if o.Redis != nil { // a nil *RedisClient inside the interface would not be nil
		opts.RateLimits = o.Redis
	}
	return projects.New(context.Background(), opts)
}

// projectsSecretsKey is the key that encrypts webhook and integration secrets: the configured one, or outside production
// a development key derived from the JWT secret (production requires an explicit key, see validateProjectsKey).
func projectsSecretsKey(cfg *config.Config) string {
	if cfg.Projects.SecretsKey != "" || cfg.Server.Env == "production" {
		return cfg.Projects.SecretsKey
	}
	return webhooks.DeriveDevelopmentKey(cfg.JWT.Secret)
}

// validateProjectsKey requires a valid projects.secrets_key in production and rejects an invalid one anywhere.
func validateProjectsKey(cfg *config.Config) error {
	if cfg.Projects.SecretsKey == "" {
		if cfg.Server.Env == "production" {
			return errors.New("projects.secrets_key (base64 32-byte key) is required in production (set PROJECTS_SECRETS_KEY)")
		}
		return nil
	}
	if _, err := webhooks.ParseSecretsKey(cfg.Projects.SecretsKey); err != nil {
		return fmt.Errorf("projects.secrets_key: %w", err)
	}
	return nil
}
