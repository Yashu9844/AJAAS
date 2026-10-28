// Package storage is the object-storage port shared by every service (docs/modules/m2-projects-tasks/design.md §5).
// Callers depend on Store; the adapter behind it is chosen at startup (S3-compatible, with MinIO in development) and
// tests use Memory. Buckets are private: only short-lived presigned URLs leave the system.
package storage

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Head when the object does not exist.
var ErrNotFound = errors.New("storage: object not found")

// PutTarget tells a client how to upload one object.
type PutTarget struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Expires time.Time         `json:"expires_at"`
}

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Size        int64
	ContentType string
	SHA256      string // lowercase hex, when the object carries a checksum
}

// Store is the provider interface. Keys are opaque, tenant-scoped paths built by the caller.
type Store interface {
	// PresignPut returns a URL the client PUTs the bytes to. contentType and size are bound into the signature, and so
	// is sha256 (lowercase hex) when it is not empty.
	PresignPut(ctx context.Context, key, contentType string, size int64, sha256 string, ttl time.Duration) (PutTarget, error)
	// PresignGet returns a download URL that names the file filename.
	PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error)
	// Head returns the stored object's metadata, or ErrNotFound.
	Head(ctx context.Context, key string) (ObjectInfo, error)
	// Delete removes the object; deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}
