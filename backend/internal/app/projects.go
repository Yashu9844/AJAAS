package app

import (
	"context"
	"fmt"

	"github.com/jaas/jaas/internal/projects"
	"github.com/jaas/jaas/internal/projects/ports"
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
		Identity: idp, Storage: store, People: people, Background: o.Background}
	if o.Redis != nil { // a nil *RedisClient inside the interface would not be nil
		opts.RateLimits = o.Redis
	}
	return projects.New(context.Background(), opts)
}
