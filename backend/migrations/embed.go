// Package migrations embeds the versioned SQL schema of every service so the server binary can apply it (production
// never relies on GORM AutoMigrate). Each service owns one Postgres schema and one migration set
// (docs/ARCHITECTURE.md §12, DHR-09): identity/ for M0 Identity & Access, hr/ for M1 HR Management.
package migrations

import (
	"embed"
	"io/fs"

	"github.com/jaas/jaas/internal/shared/database"
)

//go:embed identity/*.sql hr/*.sql
var all embed.FS

func sub(dir string) fs.FS {
	f, err := fs.Sub(all, dir)
	if err != nil { // the directories are embedded above, so this cannot fail at runtime
		return all
	}
	return f
}

// Identity is M0's migration set (schema identity).
var Identity = sub("identity")

// HR is M1's migration set (schema hr).
var HR = sub("hr")

// Sets lists every service's migrations in dependency order.
func Sets() []database.MigrationSet {
	return []database.MigrationSet{{Schema: "identity", FS: Identity}, {Schema: "hr", FS: HR}}
}
