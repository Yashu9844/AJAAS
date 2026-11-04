// Package testdb gives M3 package tests a throw-away PostgreSQL database with the meetings schema migrated, plus
// in-memory stand-ins for the M0 ports. Tests skip when the server is unreachable unless JAAS_REQUIRE_INTEGRATION=1.
package testdb

import (
	"context"
	"crypto/sha256"
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
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=3 search_path=meetings,public",
		env("JAAS_TEST_DB_HOST", "localhost"), env("JAAS_TEST_DB_PORT", "5432"), env("JAAS_TEST_DB_USER", "postgres"),
		env("JAAS_TEST_DB_PASSWORD", "postgres"), db)
}

// Open creates a fresh database copied from a migrated template and drops it when the test ends.
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
	name := "m3_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	if err := admin.Exec("CREATE DATABASE " + name + " TEMPLATE " + tpl).Error; err != nil {
		return unavailable(t, err)
	}
	db, err := gorm.Open(postgres.Open(dsn(name)), quiet)
	if err != nil {
		return unavailable(t, err)
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

// ensureTemplate returns a database with the meetings schema migrated, named after a hash of the migration files.
func ensureTemplate(admin *gorm.DB, cfg *gorm.Config) (string, error) {
	h := sha256.New()
	_ = fs.WalkDir(migrations.Meetings, ".", func(p string, d fs.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			b, _ := fs.ReadFile(migrations.Meetings, p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	name := "m3_tpl_" + hex.EncodeToString(h.Sum(nil))[:12]
	sqlDB, err := admin.DB()
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(727301)"); err != nil {
		return "", err
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock(727301)")
	var exists bool
	if err := conn.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil || exists {
		return name, err
	}
	building := name + "_b"
	_, _ = conn.ExecContext(ctx, "DROP DATABASE IF EXISTS "+building+" WITH (FORCE)")
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+building); err != nil {
		return "", err
	}
	db, err := gorm.Open(postgres.Open(dsn(building)), cfg)
	if err != nil {
		return "", err
	}
	_, err = database.RunMigrationSets(ctx, db, []database.MigrationSet{{Schema: "meetings", FS: migrations.Meetings}})
	if inner, e := db.DB(); e == nil {
		_ = inner.Close()
	}
	if err != nil {
		return "", err
	}
	_, err = conn.ExecContext(ctx, "ALTER DATABASE "+building+" RENAME TO "+name)
	return name, err
}

func unavailable(t testing.TB, err error) *gorm.DB {
	t.Helper()
	if os.Getenv("JAAS_REQUIRE_INTEGRATION") == "1" {
		t.Fatalf("integration infrastructure required but unavailable: %v", err)
	}
	t.Skipf("PostgreSQL unavailable: %v", err)
	return nil
}
