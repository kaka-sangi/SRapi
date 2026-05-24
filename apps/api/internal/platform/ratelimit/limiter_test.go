package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLimiterAllowsWithinLimits(t *testing.T) {
	limiter, _, closeRedis := newLimiter(t)
	defer closeRedis()

	decision, err := limiter.Allow(context.Background(), []Check{
		{Name: "rpm", Key: "api-key-1:rpm", Limit: 2, Cost: 1, Window: time.Minute},
		{Name: "tpm", Key: "api-key-1:tpm", Limit: 100, Cost: 20, Window: time.Minute},
	}, time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got %+v", decision)
	}
}

func TestLimiterRejectsAtomicallyWhenOneDimensionExceeds(t *testing.T) {
	limiter, redisServer, closeRedis := newLimiter(t)
	defer closeRedis()

	ctx := context.Background()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	first, err := limiter.Allow(ctx, []Check{
		{Name: "rpm", Key: "api-key-2:rpm", Limit: 2, Cost: 1, Window: time.Minute},
		{Name: "tpm", Key: "api-key-2:tpm", Limit: 30, Cost: 20, Window: time.Minute},
	}, now)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if !first.Allowed {
		t.Fatalf("expected first request allowed, got %+v", first)
	}

	second, err := limiter.Allow(ctx, []Check{
		{Name: "rpm", Key: "api-key-2:rpm", Limit: 2, Cost: 1, Window: time.Minute},
		{Name: "tpm", Key: "api-key-2:tpm", Limit: 30, Cost: 20, Window: time.Minute},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	if second.Allowed || second.Name != "tpm" || second.RetryAfter <= 0 {
		t.Fatalf("expected tpm rejection with retry-after, got %+v", second)
	}
	if got := redisValue(t, redisServer, "srapi:rl:api-key-2:rpm"); got != "1" {
		t.Fatalf("rpm counter should not be incremented on rejected request, got %q", got)
	}
	if got := redisValue(t, redisServer, "srapi:rl:api-key-2:tpm"); got != "20" {
		t.Fatalf("tpm counter should remain at first request, got %q", got)
	}
}

func TestLimiterSkipsUnsetLimits(t *testing.T) {
	limiter, _, closeRedis := newLimiter(t)
	defer closeRedis()

	decision, err := limiter.Allow(context.Background(), []Check{
		{Name: "rpm", Key: "api-key-3:rpm", Limit: 0, Cost: 1, Window: time.Minute},
	}, time.Now())
	if err != nil {
		t.Fatalf("allow unset limit: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected unset limits to allow, got %+v", decision)
	}
}

func TestLimiterRejectsInvalidChecks(t *testing.T) {
	limiter, _, closeRedis := newLimiter(t)
	defer closeRedis()

	_, err := limiter.Allow(context.Background(), []Check{{Name: "rpm", Key: "", Limit: 1, Cost: 1}}, time.Now())
	if !errors.Is(err, ErrInvalidCheck) {
		t.Fatalf("expected invalid check error, got %v", err)
	}
}

func newLimiter(t *testing.T) (*Limiter, *miniredis.Miniredis, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	limiter, err := New(client)
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	return limiter, server, func() {
		_ = client.Close()
		server.Close()
	}
}

func redisValue(t *testing.T, server *miniredis.Miniredis, key string) string {
	t.Helper()
	value, err := server.Get(key)
	if err != nil {
		t.Fatalf("get redis key %s: %v", key, err)
	}
	return value
}
