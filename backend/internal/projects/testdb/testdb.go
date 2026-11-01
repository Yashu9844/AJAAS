// Package testdb gives M2 tests a throw-away PostgreSQL database with the projects schema migrated. Tests skip when the
// server is unreachable unless JAAS_REQUIRE_INTEGRATION=1.
package testdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
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
	tpl, err := ensureTemplate(admin, quiet)
	if err != nil {
		return unavailable(t, err)
	}
	name := "m2_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	if err := admin.Exec("CREATE DATABASE " + name + " TEMPLATE " + tpl).Error; err != nil {
		return unavailable(t, err)
	}
	db, err := gorm.Open(postgres.Open(dsn(name)), quiet)
	if err != nil {
		return unavailable(t, err)
	}
	// Migrations seed worker cursors with the migration time; a copy of the template must start at the time of the test.
	if err := db.Exec(`UPDATE projects_dispatch_cursor SET last_created_at = now(); UPDATE automation_cursor SET last_created_at = now()`).Error; err != nil {
		t.Fatalf("reset cursors: %v", err)
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

// ensureTemplate returns the name of a database with the projects schema and the stand-in identity table already migrated.
// Tests copy it with CREATE DATABASE ... TEMPLATE, which is far faster than migrating every test database. The name carries a
// hash of the migration files so a changed migration builds a new template; a lock serialises packages that start together.
func ensureTemplate(admin *gorm.DB, cfg *gorm.Config) (string, error) {
	h := sha256.New()
	entries, err := fs.ReadDir(migrations.Projects, ".")
	if err != nil {
		return "", err
	}
	_ = fs.WalkDir(migrations.Projects, ".", func(p string, d fs.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			b, _ := fs.ReadFile(migrations.Projects, p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	_ = entries
	name := "m2_tpl_" + hex.EncodeToString(h.Sum(nil))[:12]
	sqlDB, err := admin.DB()
	if err != nil {
		return "", err
	}
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock(727201)"); err != nil {
		return "", err
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(727201)")
	var exists bool
	if err := conn.QueryRowContext(context.Background(), "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil || exists {
		return name, err
	}
	building := name + "_b"
	if err := buildTemplate(conn, building, cfg); err != nil {
		return "", err
	}
	_, err = conn.ExecContext(context.Background(), "ALTER DATABASE "+building+" RENAME TO "+name)
	return name, err
}

// buildTemplate creates the named database and migrates it.
func buildTemplate(conn *sql.Conn, name string, cfg *gorm.Config) error {
	ctx := context.Background()
	_, _ = conn.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		return err
	}
	db, err := gorm.Open(postgres.Open(dsn(name)), cfg)
	if err != nil {
		return err
	}
	defer func() {
		if inner, e := db.DB(); e == nil {
			_ = inner.Close()
		}
	}()
	sets := []database.MigrationSet{{Schema: "projects", FS: migrations.Projects}}
	if _, err := database.RunMigrationSets(ctx, db, sets); err != nil {
		return err
	}
	return db.Exec(FakeRoleTable).Error
}

func unavailable(t testing.TB, err error) *gorm.DB {
	t.Helper()
	if os.Getenv("JAAS_REQUIRE_INTEGRATION") == "1" {
		t.Fatalf("integration infrastructure required but unavailable: %v", err)
	}
	t.Skipf("PostgreSQL unavailable: %v", err)
	return nil
}
