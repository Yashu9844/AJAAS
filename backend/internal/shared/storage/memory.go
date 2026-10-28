package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"time"
)

// Memory is an in-process Store for unit tests. Put plays the part of the client's upload to a presigned URL.
type Memory struct {
	mu      sync.Mutex
	objects map[string]ObjectInfo
	now     func() time.Time
	// FailHead and FailDelete let tests inject provider failures.
	FailHead, FailDelete error
}

// NewMemory returns an empty Memory store.
func NewMemory() *Memory { return &Memory{objects: map[string]ObjectInfo{}, now: time.Now} }

func (m *Memory) PresignPut(_ context.Context, key, contentType string, size int64, sum string, ttl time.Duration) (PutTarget, error) {
	h := map[string]string{"Content-Type": contentType}
	if sum != "" {
		h["x-amz-meta-sha256"] = sum
	}
	return PutTarget{URL: "memory://" + url.PathEscape(key), Method: "PUT", Headers: h, Expires: m.now().Add(ttl)}, nil
}

func (m *Memory) PresignGet(_ context.Context, key, filename string, ttl time.Duration) (string, error) {
	return fmt.Sprintf("memory://%s?filename=%s&ttl=%d", url.PathEscape(key), url.QueryEscape(filename), int(ttl.Seconds())), nil
}

func (m *Memory) Head(_ context.Context, key string) (ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailHead != nil {
		return ObjectInfo{}, m.FailHead
	}
	o, ok := m.objects[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return o, nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailDelete != nil {
		return m.FailDelete
	}
	delete(m.objects, key)
	return nil
}

// Put stores data under key, as a client would after PresignPut.
func (m *Memory) Put(key, contentType string, data []byte) {
	sum := sha256.Sum256(data)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = ObjectInfo{Size: int64(len(data)), ContentType: contentType, SHA256: hex.EncodeToString(sum[:])}
}

// Has reports whether key exists.
func (m *Memory) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok
}

// Len is the number of stored objects.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}
