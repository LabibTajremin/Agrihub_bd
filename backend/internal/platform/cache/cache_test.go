package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

func TestLRU_EvictsLeastRecentlyUsed(t *testing.T) {
	if _, err := NewLRU[string, int](0); err != ErrSize {
		t.Fatal("size 0 must fail")
	}
	l, err := NewLRU[string, int](2)
	if err != nil {
		t.Fatal(err)
	}
	l.Add("a", 1)
	l.Add("b", 2)
	l.Get("a")
	l.Add("c", 3)
	if _, ok := l.Get("b"); ok {
		t.Fatal("b should be evicted")
	}
	if v, ok := l.Get("a"); !ok || v != 1 || l.Len() != 2 {
		t.Fatal("a should survive")
	}
}

func storeContract(t *testing.T, s Store, advance func(time.Duration)) {
	t.Helper()
	ctx := context.Background()
	if _, ok, err := s.Get(ctx, "k"); ok || err != nil {
		t.Fatal("miss expected")
	}
	if err := s.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := s.Get(ctx, "k"); !ok || err != nil || string(v) != "v" {
		t.Fatal("hit expected")
	}
	advance(2 * time.Minute)
	if _, ok, _ := s.Get(ctx, "k"); ok {
		t.Fatal("expired")
	}
	_ = s.Set(ctx, "p", []byte("forever"), 0)
	advance(24 * time.Hour)
	if _, ok, _ := s.Get(ctx, "p"); !ok {
		t.Fatal("no-ttl entry must persist")
	}
	if err := s.Delete(ctx, "p"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get(ctx, "p"); ok {
		t.Fatal("deleted")
	}
}

func TestMemoryStore_Contract(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	storeContract(t, NewMemory(c), c.Advance)
}

func TestRedisStore_Contract(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	storeContract(t, NewRedis(client), mr.FastForward)
}

func TestRedisStore_Error(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	defer client.Close()
	mr.Close()
	if _, _, err := NewRedis(client).Get(context.Background(), "k"); err == nil {
		t.Fatal("expected error")
	}
}
