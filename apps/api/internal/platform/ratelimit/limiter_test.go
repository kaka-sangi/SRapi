package ratelimit

import (
	"context"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
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

func TestLimiterReleaseRefundsReservation(t *testing.T) {
	limiter, redisServer, closeRedis := newLimiter(t)
	defer closeRedis()

	ctx := context.Background()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	check := Check{Name: "t", Key: "k", Limit: 10, Cost: 3, Window: time.Minute}

	first, err := limiter.Allow(ctx, []Check{check}, now)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if !first.Allowed {
		t.Fatalf("expected first allow within limit, got %+v", first)
	}
	if got := redisValue(t, redisServer, "srapi:rl:k"); got != "3" {
		t.Fatalf("expected counter at 3 after reservation, got %q", got)
	}

	if err := limiter.Release(ctx, []Check{check}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := redisValue(t, redisServer, "srapi:rl:k"); got != "0" {
		t.Fatalf("expected counter refunded to 0, got %q", got)
	}

	// With the 3 refunded (used == 0), a fresh Cost 8 reservation (0+8 <= 10) must
	// be allowed. Without the refund (3+8 > 10) this would be limited.
	second, err := limiter.Allow(ctx, []Check{{Name: "t", Key: "k", Limit: 10, Cost: 8, Window: time.Minute}}, now.Add(time.Second))
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	if !second.Allowed {
		t.Fatalf("expected second allow after refund, got %+v", second)
	}
}

func TestLimiterReleaseClampsAtZero(t *testing.T) {
	limiter, redisServer, closeRedis := newLimiter(t)
	defer closeRedis()

	ctx := context.Background()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)

	if _, err := limiter.Allow(ctx, []Check{{Name: "t", Key: "clamp", Limit: 10, Cost: 1, Window: time.Minute}}, now); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// Refund more than was reserved; the counter must clamp at 0, never go negative.
	if err := limiter.Release(ctx, []Check{{Name: "t", Key: "clamp", Limit: 10, Cost: 5, Window: time.Minute}}); err != nil {
		t.Fatalf("over-release: %v", err)
	}
	if got := redisValue(t, redisServer, "srapi:rl:clamp"); got != "0" {
		t.Fatalf("expected counter clamped to 0, got %q", got)
	}
}

func TestLimiterReleaseNoopForUnreservedKey(t *testing.T) {
	limiter, redisServer, closeRedis := newLimiter(t)
	defer closeRedis()

	ctx := context.Background()
	if err := limiter.Release(ctx, []Check{{Name: "t", Key: "missing", Limit: 10, Cost: 5, Window: time.Minute}}); err != nil {
		t.Fatalf("release of unreserved key: %v", err)
	}
	if redisServer.Exists("srapi:rl:missing") {
		t.Fatal("expected release of unreserved key to not create a counter")
	}
}

func TestLimiterConcurrencyLeaseRejectsReleasesAndExpires(t *testing.T) {
	limiter, redisServer, closeRedis := newLimiter(t)
	defer closeRedis()

	ctx := context.Background()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	lease, decision, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{
		Name:  "concurrency",
		Key:   "api-key-4:concurrency",
		Limit: 1,
		TTL:   time.Minute,
	}, now)
	if err != nil {
		t.Fatalf("acquire concurrency: %v", err)
	}
	if !decision.Allowed || lease.Token == "" || !lease.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("expected acquired lease and allowed decision, lease=%+v decision=%+v", lease, decision)
	}
	if members, err := redisServer.ZMembers("srapi:rl:api-key-4:concurrency"); err != nil || len(members) != 1 {
		t.Fatalf("expected one concurrency zset member, members=%v err=%v", members, err)
	}

	_, limited, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{
		Name:  "concurrency",
		Key:   "api-key-4:concurrency",
		Limit: 1,
		TTL:   time.Minute,
	}, now.Add(time.Second))
	if err != nil {
		t.Fatalf("second acquire concurrency: %v", err)
	}
	if limited.Allowed || limited.Name != "concurrency" || limited.RetryAfter <= 0 {
		t.Fatalf("expected concurrency rejection with retry-after, got %+v", limited)
	}

	if err := limiter.ReleaseConcurrency(ctx, lease); err != nil {
		t.Fatalf("release concurrency: %v", err)
	}
	if redisServer.Exists("srapi:rl:api-key-4:concurrency") {
		t.Fatal("expected release to remove empty concurrency key")
	}

	expiringLease, decision, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{
		Name:  "concurrency",
		Key:   "api-key-4:concurrency",
		Limit: 1,
		TTL:   time.Minute,
	}, now)
	if err != nil {
		t.Fatalf("reacquire concurrency: %v", err)
	}
	if !decision.Allowed || expiringLease.Token == "" {
		t.Fatalf("expected reacquired lease, lease=%+v decision=%+v", expiringLease, decision)
	}
	_, expiredDecision, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{
		Name:  "concurrency",
		Key:   "api-key-4:concurrency",
		Limit: 1,
		TTL:   time.Minute,
	}, now.Add(time.Minute+time.Millisecond))
	if err != nil {
		t.Fatalf("acquire after expiry: %v", err)
	}
	if !expiredDecision.Allowed || expiredDecision.Used != 1 {
		t.Fatalf("expected expired lease to be pruned, got %+v", expiredDecision)
	}
}

func TestLimiterConcurrencySkipsUnsetLimitAndRejectsInvalidCheck(t *testing.T) {
	limiter, _, closeRedis := newLimiter(t)
	defer closeRedis()

	_, decision, err := limiter.AcquireConcurrency(context.Background(), ConcurrencyCheck{
		Name:  "concurrency",
		Key:   "api-key-5:concurrency",
		Limit: 0,
	}, time.Now())
	if err != nil {
		t.Fatalf("unset concurrency limit: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected unset concurrency limit to allow, got %+v", decision)
	}

	_, _, err = limiter.AcquireConcurrency(context.Background(), ConcurrencyCheck{
		Name:  "concurrency",
		Limit: 1,
	}, time.Now())
	if !errors.Is(err, ErrInvalidCheck) {
		t.Fatalf("expected invalid concurrency check error, got %v", err)
	}
}

func TestLimiterP99Budget(t *testing.T) {
	if os.Getenv("SRAPI_RATE_LIMIT_P99_GUARD") != "1" {
		t.Skip("set SRAPI_RATE_LIMIT_P99_GUARD=1 to run the rate limiter p99 guard")
	}
	addr := strings.TrimSpace(os.Getenv("SRAPI_RATE_LIMIT_P99_REDIS_ADDR"))
	if addr == "" {
		t.Fatal("SRAPI_RATE_LIMIT_P99_REDIS_ADDR is required for the rate limiter p99 guard")
	}
	limiter, closeRedis := newExternalRedisLimiter(t, addr)
	defer closeRedis()

	ctx := context.Background()
	samples := envInt("SRAPI_RATE_LIMIT_P99_SAMPLES", 2000)
	budget := time.Duration(envInt("SRAPI_RATE_LIMIT_P99_BUDGET_MS", 2)) * time.Millisecond
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)

	pingP99 := measureP99(t, samples, func(int) {
		if err := limiter.client.Ping(ctx).Err(); err != nil {
			t.Fatalf("redis ping: %v", err)
		}
	})
	if pingP99 > budget {
		t.Fatalf("redis ping p99 %s exceeds budget %s over %d samples; run this guard against low-latency Redis before evaluating limiter p99", pingP99, budget, samples)
	}

	warmLimiter(t, limiter, ctx, now)

	allowP99 := measureP99(t, samples, func(i int) {
		decision, err := limiter.Allow(ctx, []Check{
			{Name: "rpm", Key: "bench:p99:rpm", Limit: samples * 2, Cost: 1, Window: time.Minute},
			{Name: "tpm", Key: "bench:p99:tpm", Limit: samples * 400, Cost: 100, Window: time.Minute},
		}, now.Add(time.Duration(i)*time.Millisecond))
		if err != nil {
			t.Fatalf("allow sample %d: %v", i, err)
		}
		if !decision.Allowed {
			t.Fatalf("allow sample %d unexpectedly limited: %+v", i, decision)
		}
	})
	if allowP99 > budget {
		t.Fatalf("rate limiter Allow p99 %s exceeds budget %s over %d samples; redis ping p99=%s", allowP99, budget, samples, pingP99)
	}

	leases := make([]ConcurrencyLease, 0, samples)
	acquireP99 := measureP99(t, samples, func(i int) {
		lease, decision, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{
			Name:  "concurrency",
			Key:   "bench:p99:concurrency",
			Limit: samples * 2,
			TTL:   time.Minute,
		}, now.Add(time.Duration(i)*time.Millisecond))
		if err != nil {
			t.Fatalf("acquire concurrency sample %d: %v", i, err)
		}
		if !decision.Allowed {
			t.Fatalf("acquire concurrency sample %d unexpectedly limited: %+v", i, decision)
		}
		leases = append(leases, lease)
	})
	if acquireP99 > budget {
		t.Fatalf("rate limiter AcquireConcurrency p99 %s exceeds budget %s over %d samples; redis ping p99=%s", acquireP99, budget, samples, pingP99)
	}

	releaseP99 := measureP99(t, samples, func(i int) {
		lease := leases[i]
		if err := limiter.ReleaseConcurrency(ctx, lease); err != nil {
			t.Fatalf("release concurrency sample %d: %v", i, err)
		}
	})
	if releaseP99 > budget {
		t.Fatalf("rate limiter ReleaseConcurrency p99 %s exceeds budget %s over %d samples; redis ping p99=%s", releaseP99, budget, samples, pingP99)
	}

	t.Logf("rate limiter p99 budget passed: ping=%s allow=%s acquire=%s release=%s budget=%s samples=%d redis=%s", pingP99, allowP99, acquireP99, releaseP99, budget, samples, addr)
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

func newExternalRedisLimiter(t *testing.T, addr string) (*Limiter, func()) {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("SRAPI_RATE_LIMIT_P99_REDIS_PASSWORD"),
		DB:       envInt("SRAPI_RATE_LIMIT_P99_REDIS_DB", 15),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("ping Redis %s: %v", addr, err)
	}
	limiter, err := New(client)
	if err != nil {
		_ = client.Close()
		t.Fatalf("new limiter: %v", err)
	}
	limiter.prefix = defaultKeyPrefix + "bench:" + strconv.FormatInt(time.Now().UnixNano(), 36) + ":"
	return limiter, func() {
		_ = client.Close()
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

func warmLimiter(t *testing.T, limiter *Limiter, ctx context.Context, now time.Time) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if _, err := limiter.Allow(ctx, []Check{
			{Name: "rpm", Key: "bench:warm:rpm", Limit: 1000, Cost: 1, Window: time.Minute},
			{Name: "tpm", Key: "bench:warm:tpm", Limit: 100000, Cost: 100, Window: time.Minute},
		}, now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatalf("warm allow: %v", err)
		}
		lease, _, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{
			Name:  "concurrency",
			Key:   "bench:warm:concurrency",
			Limit: 1000,
			TTL:   time.Minute,
		}, now.Add(time.Duration(i)*time.Millisecond))
		if err != nil {
			t.Fatalf("warm concurrency acquire: %v", err)
		}
		if err := limiter.ReleaseConcurrency(ctx, lease); err != nil {
			t.Fatalf("warm concurrency release: %v", err)
		}
	}
}

func measureP99(t *testing.T, samples int, run func(int)) time.Duration {
	t.Helper()
	if samples <= 0 {
		t.Fatal("p99 samples must be positive")
	}
	durations := make([]time.Duration, samples)
	for i := 0; i < samples; i++ {
		startedAt := time.Now()
		run(i)
		durations[i] = time.Since(startedAt)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	index := ((samples * 99) + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > samples {
		index = samples
	}
	return durations[index-1]
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
