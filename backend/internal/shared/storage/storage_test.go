package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	put, err := m.PresignPut(ctx, "t/1/a", "text/plain", 5, "abc", time.Minute)
	if err != nil || put.Method != "PUT" || put.Headers["Content-Type"] != "text/plain" || put.Headers["x-amz-meta-sha256"] != "abc" {
		t.Fatalf("presign put: %+v %v", put, err)
	}
	if _, err := m.Head(ctx, "t/1/a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object: %v", err)
	}
	m.Put("t/1/a", "text/plain", []byte("hello"))
	info, err := m.Head(ctx, "t/1/a")
	if err != nil || info.Size != 5 || info.ContentType != "text/plain" || len(info.SHA256) != 64 {
		t.Fatalf("head: %+v %v", info, err)
	}
	if u, _ := m.PresignGet(ctx, "t/1/a", "a b.txt", time.Minute); u == "" {
		t.Fatal("empty download url")
	}
	if err := m.Delete(ctx, "t/1/a"); err != nil || m.Has("t/1/a") || m.Len() != 0 {
		t.Fatalf("delete: %v", err)
	}
	m.FailHead, m.FailDelete = errors.New("down"), errors.New("down")
	if _, err := m.Head(ctx, "k"); err == nil {
		t.Fatal("injected head failure ignored")
	}
	if err := m.Delete(ctx, "k"); err == nil {
		t.Fatal("injected delete failure ignored")
	}
}

// TestS3RoundTrip drives the real adapter against an S3-compatible server (docker-compose service `s3`): upload through
// the presigned URL, head, presigned download, delete. It skips when the server is unreachable unless
// JAAS_REQUIRE_INTEGRATION=1.
func TestS3RoundTrip(t *testing.T) {
	ctx := context.Background()
	endpoint := envOr("JAAS_TEST_S3_ENDPOINT", "http://localhost:9090")
	s, err := NewS3(ctx, S3Config{Endpoint: endpoint, Region: "us-east-1", Bucket: "jaas-files", AccessKey: "test", SecretKey: "test", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBucket(ctx); err != nil {
		if os.Getenv("JAAS_REQUIRE_INTEGRATION") == "1" {
			t.Fatalf("object storage required but unavailable: %v", err)
		}
		t.Skipf("object storage unavailable: %v", err)
	}
	key := fmt.Sprintf("t/test/%d/file.txt", time.Now().UnixNano())
	body := []byte("hello object storage")
	if _, err := s.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("head of a missing key: %v", err)
	}
	target, err := s.PresignPut(ctx, key, "text/plain", int64(len(body)), "deadbeef", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(target.Method, target.URL, bytes.NewReader(body))
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode/100 != 2 {
		t.Fatalf("upload: %v %v", resp, err)
	}
	resp.Body.Close()
	info, err := s.Head(ctx, key)
	if err != nil || info.Size != int64(len(body)) || info.ContentType != "text/plain" || info.SHA256 != "deadbeef" {
		t.Fatalf("head: %+v %v", info, err)
	}
	u, err := s.PresignGet(ctx, key, "notes.txt", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	g, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(g.Body)
	g.Body.Close()
	if !bytes.Equal(got, body) || g.Header.Get("Content-Disposition") == "" {
		t.Fatalf("download: %q %v", got, g.Header)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing object must succeed: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
