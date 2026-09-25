// Package ratelimit implements token-bucket rate limiting per key with a Redis
// backend and an in-memory fallback, so local dev and tests need no Redis.
package ratelimit

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/redis/go-redis/v9"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

// Rate is a bucket of Limit tokens refilled evenly over Per.
type Rate struct {
	Limit int
	Per   time.Duration
}

// PerMinute is a convenience constructor.
func PerMinute(n int) Rate { return Rate{Limit: n, Per: time.Minute} }

// PerHour is a convenience constructor.
func PerHour(n int) Rate { return Rate{Limit: n, Per: time.Hour} }

func (r Rate) refillPerMs() float64 { return float64(r.Limit) / float64(r.Per.Milliseconds()) }

// Decision is the outcome of one Allow call.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Limiter takes one token from the bucket identified by key.
type Limiter interface {
	Allow(ctx context.Context, key string, rate Rate) (Decision, error)
}

// Memory is a process-local token bucket store bounded by an LRU.
type Memory struct {
	mu      sync.Mutex
	clock   clock.Clock
	buckets *lru.Cache[string, *bucket]
}

type bucket struct {
	tokens float64
	last   time.Time
}

// MemoryCapacity bounds the number of tracked keys.
const MemoryCapacity = 100_000

// NewMemory returns an in-memory limiter.
func NewMemory(c clock.Clock) *Memory {
	cache, _ := lru.New[string, *bucket](MemoryCapacity) // size is a positive constant
	return &Memory{clock: c, buckets: cache}
}

// Allow implements Limiter.
func (m *Memory) Allow(_ context.Context, key string, rate Rate) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	b, ok := m.buckets.Get(key)
	if !ok {
		b = &bucket{tokens: float64(rate.Limit), last: now}
		m.buckets.Add(key, b)
	}
	elapsed := float64(max(now.Sub(b.last).Milliseconds(), 0))
	b.tokens = math.Min(float64(rate.Limit), b.tokens+elapsed*rate.refillPerMs())
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return Decision{Allowed: true}, nil
	}
	wait := math.Ceil((1 - b.tokens) / rate.refillPerMs())
	return Decision{RetryAfter: time.Duration(wait) * time.Millisecond}, nil
}

// bucketLua atomically refills and takes a token. Time comes from the
// caller's injected clock so the script is deterministic under test.
const bucketLua = `
local cap = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local b = redis.call('HMGET', KEYS[1], 't', 'ts')
local tokens = tonumber(b[1])
local ts = tonumber(b[2])
if tokens == nil then tokens = cap; ts = now end
tokens = math.min(cap, tokens + math.max(0, now - ts) * rate)
local allowed = 0
local retry = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  retry = math.ceil((1 - tokens) / rate)
end
redis.call('HSET', KEYS[1], 't', tostring(tokens), 'ts', tostring(now))
redis.call('PEXPIRE', KEYS[1], ttl)
return {allowed, retry}
`

// Redis is a distributed token bucket.
type Redis struct {
	client redis.Scripter
	clock  clock.Clock
	script *redis.Script
}

// NewRedis returns a Redis-backed limiter.
func NewRedis(client redis.Scripter, c clock.Clock) *Redis {
	return &Redis{client: client, clock: c, script: redis.NewScript(bucketLua)}
}

// Allow implements Limiter.
func (r *Redis) Allow(ctx context.Context, key string, rate Rate) (Decision, error) {
	now := r.clock.Now().UnixMilli()
	res, err := r.script.Run(ctx, r.client, []string{"rl:" + key},
		rate.Limit, strconv.FormatFloat(rate.refillPerMs(), 'f', -1, 64), now, rate.Per.Milliseconds()*2).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	return Decision{Allowed: res[0] == 1, RetryAfter: time.Duration(res[1]) * time.Millisecond}, nil
}

// Fallback uses Primary and degrades to Secondary when Primary errors
// (e.g. Redis unreachable), so rate limiting never takes the API down.
type Fallback struct {
	Primary   Limiter
	Secondary Limiter
	Logger    *slog.Logger
}

// Allow implements Limiter.
func (f Fallback) Allow(ctx context.Context, key string, rate Rate) (Decision, error) {
	d, err := f.Primary.Allow(ctx, key, rate)
	if err == nil {
		return d, nil
	}
	f.Logger.WarnContext(ctx, "rate limiter primary failed; using fallback", "error", err)
	return f.Secondary.Allow(ctx, key, rate)
}
