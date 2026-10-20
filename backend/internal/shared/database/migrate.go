package database

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// SearchPath is the schema search path of the in-process monolith: every service's schema, then public. Each
// service's tables live in its own schema; the shared path lets the single process reach both while separate
// deployments get one schema each (docs/ARCHITECTURE.md §12).
const SearchPath = "identity,hr,public"

const migrationsTable = "schema_migrations"

// migrationLockKey serialises concurrent starters across every schema.
const migrationLockKey = 727274

var schemaName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// MigrationSet is one service's migrations, applied into its own schema.
type MigrationSet struct {
	Schema string
	FS     fs.FS
}

// RunMigrationSets applies every set in order. Within a set each pending *.up.sql runs in its own transaction with
// search_path set to the set's schema, and is recorded in <schema>.schema_migrations. It returns "schema/version" for
// every migration applied by this call.
func RunMigrationSets(ctx context.Context, db *gorm.DB, sets []MigrationSet) ([]string, error) {
	var applied []string
	for _, set := range sets {
		got, err := runSet(ctx, db, set)
		if err != nil {
			return applied, err
		}
		applied = append(applied, got...)
	}
	return applied, nil
}

func runSet(ctx context.Context, db *gorm.DB, set MigrationSet) ([]string, error) {
	if !schemaName.MatchString(set.Schema) {
		return nil, fmt.Errorf("invalid schema name %q", set.Schema)
	}
	table := set.Schema + "." + migrationsTable
	if err := db.WithContext(ctx).Exec("CREATE SCHEMA IF NOT EXISTS " + set.Schema).Error; err != nil {
		return nil, fmt.Errorf("create schema %s: %w", set.Schema, err)
	}
	if err := db.WithContext(ctx).Exec("CREATE TABLE IF NOT EXISTS " + table +
		" (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())").Error; err != nil {
		return nil, fmt.Errorf("create %s: %w", table, err)
	}
	entries, err := fs.Glob(set.FS, "*.up.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	var applied []string
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationLockKey).Error; err != nil {
			return err
		}
		done, err := appliedVersions(tx, table)
		if err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL search_path TO " + set.Schema + ", public").Error; err != nil {
			return err
		}
		for _, name := range entries {
			version := strings.TrimSuffix(name, ".up.sql")
			if done[version] {
				continue
			}
			if err := applyFile(tx, set, name, table, version); err != nil {
				return err
			}
			applied = append(applied, set.Schema+"/"+version)
		}
		return nil
	})
	return applied, err
}

func appliedVersions(tx *gorm.DB, table string) (map[string]bool, error) {
	var versions []string
	if err := tx.Raw("SELECT version FROM " + table).Scan(&versions).Error; err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(versions))
	for _, v := range versions {
		done[v] = true
	}
	return done, nil
}

func applyFile(tx *gorm.DB, set MigrationSet, name, table, version string) error {
	sqlBytes, err := fs.ReadFile(set.FS, name)
	if err != nil {
		return err
	}
	if err := tx.Exec(string(sqlBytes)).Error; err != nil {
		return fmt.Errorf("migration %s/%s failed: %w", set.Schema, name, err)
	}
	return tx.Exec("INSERT INTO "+table+" (version) VALUES (?)", version).Error
}
