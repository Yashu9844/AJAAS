// Package migrations embeds the versioned SQL schema so the server binary can apply it (production never relies on
// GORM AutoMigrate, whose constraints drift from these files).
package migrations

import "embed"

// FS holds every NNNNNN_name.up.sql / .down.sql file.
//
//go:embed *.sql
var FS embed.FS
