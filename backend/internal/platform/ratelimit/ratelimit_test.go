package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func exercise(t *testing.T, l Limiter, c *clock.Fake) {
	t.Helper()
	ctx := context.Background()
	rate := PerMinute(2)
	for i := range 2 {
		if d, err := l.Allow(ctx, "k", rate); err != nil || !d.Allowed {
			t.Fatalf("call %d should pass: %+v %v", i, d, err)
		}
	}
	d, err := l.Allow(ctx, "k", rate)
	if err != nil || d.Allowed || d.RetryAfter != 30*time.Second {
		t.Fatalf("third call must be limited with 30s retry: %+v %v", d, err)
	}
	if d, _ := l.Allow(ctx, "other", rate); !d.Allowed {
		t.Fatal("keys are independent")
	}
	c.Advance(30 * time.Second)
	if d, _ := l.Allow(ctx, "k", rate); !d.Allowed {
		t.Fatal("refill after 30s")
	}
	c.Advance(time.Hour)
	for range 2 {
		if d, _ := l.Allow(ctx, "k", rate); !d.Allowed {
			t.Fatal("bucket capped at limit then refilled")
		}
	}
	if d, _ := l.Allow(ctx, "k", rate); d.Allowed {
		t.Fatal("capacity must not exceed limit")
	}
}

func TestMemory_TokenBucket(t *testing.T) {
	c := clock.NewFake(t0)
	exercise(t, NewMemory(c), c)
}

func TestMemory_ClockSkewBackwardsIsSafe(t *testing.T) {
	c := clock.NewFake(t0)
	m := NewMemory(c)
	_, _ = m.Allow(context.Background(), "k", PerHour(1))
	c.Set(t0.Add(-time.Hour))
	if d, _ := m.Allow(context.Background(), "k", PerHour(1)); d.Allowed {
		t.Fatal("negative elapsed must not refill")
	}
}

func TestRedis_TokenBucket(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	c := clock.NewFake(t0)
	exercise(t, NewRedis(client, c), c)
	if ttl := mr.TTL("rl:k"); ttl != 2*time.Minute {
		t.Fatalf("ttl %v", ttl)
	}
}

func TestRedis_ErrorPropagates(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	defer client.Close()
	mr.Close()
	if _, err := NewRedis(client, clock.NewFake(t0)).Allow(context.Background(), "k", PerMinute(1)); err == nil {
		t.Fatal("expected error")
	}
}

type failing struct{}

func (failing) Allow(context.Context, string, Rate) (Decision, error) {
	return Decision{}, errors.New("down")
}

func TestFallback(t *testing.T) {
	c := clock.NewFake(t0)
	f := Fallback{Primary: failing{}, Secondary: NewMemory(c), Logger: logger.Discard()}
	if d, err := f.Allow(context.Background(), "k", PerMinute(1)); err != nil || !d.Allowed {
		t.Fatal(d, err)
	}
	ok := Fallback{Primary: NewMemory(c), Secondary: failing{}, Logger: logger.Discard()}
	if d, err := ok.Allow(context.Background(), "k", PerMinute(1)); err != nil || !d.Allowed {
		t.Fatal(d, err)
	}
}

func TestMemory_BoundedByLRU(t *testing.T) {
	m := NewMemory(clock.NewFake(t0))
	for i := range MemoryCapacity + 10 {
		_, _ = m.Allow(context.Background(), fmt.Sprint(i), PerMinute(1))
	}
	if m.buckets.Len() != MemoryCapacity {
		t.Fatal(m.buckets.Len())
	}
}
