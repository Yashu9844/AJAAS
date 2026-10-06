package database

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gorm.io/gorm"
)

const migrationsTable = "schema_migrations"

// RunMigrations applies, in order and each inside its own transaction, every *.up.sql file of fsys that has not been
// applied yet, recording versions in schema_migrations. A Postgres advisory lock serialises concurrent starters.
// It returns the versions applied by this call.
func RunMigrations(ctx context.Context, db *gorm.DB, fsys fs.FS) ([]string, error) {
	if err := db.WithContext(ctx).Exec(
		"CREATE TABLE IF NOT EXISTS " + migrationsTable + " (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())",
	).Error; err != nil {
		return nil, fmt.Errorf("create %s: %w", migrationsTable, err)
	}

	entries, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)

	var applied []string
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// released automatically when the transaction ends
		if err := tx.Exec("SELECT pg_advisory_xact_lock(727274)").Error; err != nil {
			return err
		}
		done := map[string]bool{}
		var versions []string
		if err := tx.Raw("SELECT version FROM " + migrationsTable).Scan(&versions).Error; err != nil {
			return err
		}
		for _, v := range versions {
			done[v] = true
		}
		for _, name := range entries {
			version := strings.TrimSuffix(name, ".up.sql")
			if done[version] {
				continue
			}
			sqlBytes, err := fs.ReadFile(fsys, name)
			if err != nil {
				return err
			}
			if err := tx.Exec(string(sqlBytes)).Error; err != nil {
				return fmt.Errorf("migration %s failed: %w", name, err)
			}
			if err := tx.Exec("INSERT INTO "+migrationsTable+" (version) VALUES (?)", version).Error; err != nil {
				return err
			}
			applied = append(applied, version)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return applied, nil
}
