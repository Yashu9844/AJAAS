package app

import (
	"context"
	"fmt"

	"github.com/jaas/jaas/internal/projects"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/storage"
)

// buildProjects wires M2 Projects & Tasks, including the object-storage provider when one is configured.
func buildProjects(o Options, idp identityport.Port) (*projects.Module, error) {
	var store storage.Store
	if sc := o.Config.Storage; sc.Bucket != "" {
		s3, err := storage.NewS3(context.Background(), storage.S3Config{Endpoint: sc.Endpoint, Region: sc.Region, Bucket: sc.Bucket,
			AccessKey: sc.AccessKey, SecretKey: sc.SecretKey, PathStyle: sc.PathStyle})
		if err != nil {
			return nil, fmt.Errorf("object storage: %w", err)
		}
		store = s3
	}
	return projects.New(context.Background(), projects.Options{DB: o.DB, Publisher: o.Publisher, Logger: &o.Logger.Logger,
		Identity: idp, Storage: store, Background: o.Background})
}
