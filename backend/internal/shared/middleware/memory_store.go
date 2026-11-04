package middleware

import (
	"context"
	"sync"
	"time"
)

// memoryMaxKeys bounds the in-memory store; expired keys are dropped when it is full.
const memoryMaxKeys = 10000

type memoryCounter struct {
	n   int64
	exp time.Time
}

// MemoryRateStore is a per-process RateLimitStore for routes that must stay limited when Redis is not configured
// (development, tests). Counters are not shared between instances; production passes the Redis client instead.
type MemoryRateStore struct {
	mu   sync.Mutex
	keys map[string]*memoryCounter
	now  func() time.Time
}

// NewMemoryRateStore returns an empty store.
func NewMemoryRateStore() *MemoryRateStore {
	return &MemoryRateStore{keys: map[string]*memoryCounter{}, now: time.Now}
}

func (m *MemoryRateStore) live(key string) *memoryCounter {
	now := m.now()
	if len(m.keys) >= memoryMaxKeys {
		for k, c := range m.keys {
			if now.After(c.exp) {
				delete(m.keys, k)
			}
		}
	}
	c := m.keys[key]
	if c != nil && now.After(c.exp) {
		delete(m.keys, key)
		return nil
	}
	return c
}

// IncrWithExpire increments key, starting a window of length window on first use.
func (m *MemoryRateStore) IncrWithExpire(_ context.Context, key string, window time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.live(key)
	if c == nil {
		c = &memoryCounter{exp: m.now().Add(window)}
		m.keys[key] = c
	}
	c.n++
	return c.n, nil
}

// Decr decrements key, never below zero.
func (m *MemoryRateStore) Decr(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.live(key); c != nil && c.n > 0 {
		c.n--
	}
	return nil
}

// TTL returns the time left in the window of key (0 when absent).
func (m *MemoryRateStore) TTL(_ context.Context, key string) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.live(key); c != nil {
		return c.exp.Sub(m.now()), nil
	}
	return 0, nil
}
