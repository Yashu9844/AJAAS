package integration

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestSwaggerMatchesRoutes keeps backend/api/swagger.yaml (the contract the test console is generated from) and the
// mounted router in step: every /api/v1 route is documented and every documented operation is mounted.
func TestSwaggerMatchesRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../api/swagger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, item := range doc.Paths {
		for method := range item {
			if m := strings.ToUpper(method); m == "GET" || m == "POST" || m == "PUT" || m == "PATCH" || m == "DELETE" {
				documented[m+" "+path] = true
			}
		}
	}
	param := regexp.MustCompile(`:([A-Za-z_]+)`)
	mounted := map[string]bool{}
	for _, r := range engine.Routes() {
		if !strings.HasPrefix(r.Path, "/api/v1/") {
			continue
		}
		mounted[r.Method+" "+param.ReplaceAllString(strings.TrimPrefix(r.Path, "/api/v1"), "{$1}")] = true
	}
	var missing, stale []string
	for k := range mounted {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !mounted[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("swagger drift\nmounted but undocumented:\n  %s\ndocumented but not mounted:\n  %s", strings.Join(missing, "\n  "), strings.Join(stale, "\n  "))
	}
}
