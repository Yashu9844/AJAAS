package projects

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestImportBoundaries keeps M2 a separate service (design.md §1): it never imports another module's packages, apart from
// the published permission contract internal/identity/access and the shared packages, so it can be deployed on its own.
func TestImportBoundaries(t *testing.T) {
	const mod = "github.com/jaas/jaas/internal/"
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(p, mod) {
				continue
			}
			rel := strings.TrimPrefix(p, mod)
			ok := strings.HasPrefix(rel, "projects") || strings.HasPrefix(rel, "shared/") || rel == "identity/access"
			if !ok {
				t.Errorf("%s imports %s; M2 may use only shared packages, internal/identity/access and its own packages", filepath.ToSlash(path), p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
