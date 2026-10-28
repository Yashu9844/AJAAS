// Package testdb gives M2 tests a throw-away PostgreSQL database with the projects schema migrated. Tests skip when the
// server is unreachable unless JAAS_REQUIRE_INTEGRATION=1.
package testdb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"github.com/jaas/jaas/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func dsn(db string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=3 search_path=projects,public",
		env("JAAS_TEST_DB_HOST", "localhost"), env("JAAS_TEST_DB_PORT", "5432"), env("JAAS_TEST_DB_USER", "postgres"),
		env("JAAS_TEST_DB_PASSWORD", "postgres"), db)
}

// Open creates a fresh database, migrates the projects schema and drops the database when the test ends.
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	quiet := &gorm.Config{TranslateError: true, Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn("postgres")), quiet)
	if err != nil {
		return unavailable(t, err)
	}
	name := "m2_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		return unavailable(t, err)
	}
	db, err := gorm.Open(postgres.Open(dsn(name)), quiet)
	if err != nil {
		return unavailable(t, err)
	}
	sets := []database.MigrationSet{{Schema: "projects", FS: migrations.Projects}}
	if _, err := database.RunMigrationSets(context.Background(), db, sets); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sql, e := db.DB(); e == nil {
			_ = sql.Close()
		}
		_ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error
		if sql, e := admin.DB(); e == nil {
			_ = sql.Close()
		}
	})
	return db
}

func unavailable(t testing.TB, err error) *gorm.DB {
	t.Helper()
	if os.Getenv("JAAS_REQUIRE_INTEGRATION") == "1" {
		t.Fatalf("integration infrastructure required but unavailable: %v", err)
	}
	t.Skipf("PostgreSQL unavailable: %v", err)
	return nil
}
