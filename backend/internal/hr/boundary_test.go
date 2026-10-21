package hr

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestImportBoundaries enforces docs/modules/m1-hr-management/design.md §2–4:
//   - HR never imports M0 internals; only identityport/inprocess may, and every HR package may use the published
//     contract package internal/identity/access;
//   - a context reaches another context only through that context's dto or api package;
//   - kernel and identityport are shared by every context.
func TestImportBoundaries(t *testing.T) {
	const mod = "github.com/jaas/jaas/internal/"
	contexts := map[string]bool{"organization": true, "employee": true, "attendance": true, "leave": true,
		"payroll": true, "recruitment": true, "performance": true, "core": true}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		slash := filepath.ToSlash(path)
		owner := strings.SplitN(slash, "/", 2)[0]
		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(p, mod) {
				continue
			}
			rel := strings.TrimPrefix(p, mod)
			switch {
			case strings.HasPrefix(rel, "identity/") || rel == "identity":
				if rel != "identity/access" && !strings.HasPrefix(slash, "identityport/inprocess/") {
					t.Errorf("%s imports M0 internals %s (use identityport)", slash, p)
				}
			case strings.HasPrefix(rel, "hr/"):
				parts := strings.Split(strings.TrimPrefix(rel, "hr/"), "/")
				target := parts[0]
				if !contexts[target] || target == owner || !contexts[owner] {
					continue
				}
				if len(parts) < 2 || (parts[1] != "dto" && parts[1] != "api") {
					t.Errorf("%s imports %s; other contexts are reachable only through their dto or api package", slash, p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
