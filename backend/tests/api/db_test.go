package api

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// psql runs a query against the Docker PG and returns trimmed stdout.
func psql(t *testing.T, query string) string {
	t.Helper()
	cmd := exec.Command("docker", "exec", "ajaas-postgres-1", "psql", "-U", "postgres", "-d", "jaas_dev", "-A", "-t", "-c", query)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("psql %q: %v (%s)", query, err, errBuf.String())
	}
	return strings.TrimSpace(out.String())
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}
