// Package cache provides a bounded LRU for hot in-process lookups and a
// byte-oriented Store interface with Redis and in-memory implementations.
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/redis/go-redis/v9"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

// ErrSize is returned for a non-positive LRU size.
var ErrSize = errors.New("cache: size must be positive")

// LRU is a thread-safe bounded least-recently-used cache.
type LRU[K comparable, V any] struct {
	c *lru.Cache[K, V]
}

// NewLRU returns an LRU holding at most size entries.
func NewLRU[K comparable, V any](size int) (*LRU[K, V], error) {
	if size <= 0 {
		return nil, ErrSize
	}
	c, _ := lru.New[K, V](size) // only fails for size <= 0, rejected above
	return &LRU[K, V]{c: c}, nil
}

// Get returns the cached value and marks it recently used.
func (l *LRU[K, V]) Get(k K) (V, bool) { return l.c.Get(k) }

// Add inserts or replaces a value, evicting the least recently used on overflow.
func (l *LRU[K, V]) Add(k K, v V) { l.c.Add(k, v) }

// Len returns the number of entries.
func (l *LRU[K, V]) Len() int { return l.c.Len() }

// Store is a TTL key/value store for cross-instance caching.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// Redis implements Store on a Redis client.
type Redis struct{ client redis.Cmdable }

// NewRedis wraps a Redis client.
func NewRedis(client redis.Cmdable) *Redis { return &Redis{client: client} }

// Get implements Store.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// Set implements Store.
func (r *Redis) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, val, ttl).Err()
}

// Delete implements Store.
func (r *Redis) Delete(ctx context.Context, key string) error { return r.client.Del(ctx, key).Err() }

// Memory implements Store in-process with clock-driven expiry.
type Memory struct {
	mu    sync.Mutex
	clock clock.Clock
	items map[string]memItem
}

type memItem struct {
	val     []byte
	expires time.Time
}

// NewMemory returns an in-memory Store.
func NewMemory(c clock.Clock) *Memory { return &Memory{clock: c, items: map[string]memItem{}} }

// Get implements Store.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if !it.expires.IsZero() && !m.clock.Now().Before(it.expires) {
		delete(m.items, key)
		return nil, false, nil
	}
	return append([]byte(nil), it.val...), true, nil
}

// Set implements Store; ttl <= 0 means no expiry.
func (m *Memory) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := memItem{val: append([]byte(nil), val...)}
	if ttl > 0 {
		it.expires = m.clock.Now().Add(ttl)
	}
	m.items[key] = it
	return nil
}

// Delete implements Store.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}
