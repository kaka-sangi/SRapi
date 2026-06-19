package sessionaffinity

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestBindLookupRoundTrip(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	if err := store.Bind(ctx, "scope-a", "sid:pck:abc", 7, time.Minute); err != nil {
		t.Fatalf("bind: %v", err)
	}
	binding, err := store.Lookup(ctx, "scope-a", "sid:pck:abc", time.Minute)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !binding.Found() || binding.AccountID != 7 {
		t.Fatalf("expected account 7, got %+v", binding)
	}
}

func TestAccountSessionCount(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	if err := store.AddAccountSession(ctx, 7, "sessA", time.Second); err != nil {
		t.Fatalf("add sessA: %v", err)
	}
	if err := store.AddAccountSession(ctx, 7, "sessA", time.Second); err != nil {
		t.Fatalf("refresh sessA: %v", err)
	}
	if n, err := store.CountAccountSessionsExcluding(ctx, 7, "other"); err != nil || n != 1 {
		t.Fatalf("expected 1 distinct session, got n=%d err=%v", n, err)
	}
	if err := store.AddAccountSession(ctx, 7, "sessB", time.Second); err != nil {
		t.Fatalf("add sessB: %v", err)
	}
	if n, err := store.CountAccountSessionsExcluding(ctx, 7, "other"); err != nil || n != 2 {
		t.Fatalf("expected 2 distinct sessions, got n=%d err=%v", n, err)
	}
	if n, err := store.CountAccountSessionsExcluding(ctx, 7, "sessA"); err != nil || n != 1 {
		t.Fatalf("expected 1 when excluding sessA, got n=%d err=%v", n, err)
	}
}

func TestAccountSessionCountDropsExpiredSessions(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	if err := store.AddAccountSession(ctx, 7, "expired", time.Millisecond); err != nil {
		t.Fatalf("add expired session: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if n, err := store.CountAccountSessionsExcluding(ctx, 7, "other"); err != nil || n != 0 {
		t.Fatalf("expected 0 after expiry, got n=%d err=%v", n, err)
	}
}

func TestAddAccountSessionDoesNotShortenExistingKeyTTL(t *testing.T) {
	store, server, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	if err := store.AddAccountSession(ctx, 9, "long", 10*time.Second); err != nil {
		t.Fatalf("add long session: %v", err)
	}
	if err := store.AddAccountSession(ctx, 9, "short", 2*time.Second); err != nil {
		t.Fatalf("add short session: %v", err)
	}

	key := accountSessionsKey(9)
	longTTL := server.TTL(key)
	if longTTL < 9*time.Second {
		t.Fatalf("expected long key TTL, got %s", longTTL)
	}
	server.FastForward(3 * time.Second)
	if !server.Exists(key) {
		t.Fatalf("short session write shortened account session key TTL")
	}
	if ttl := server.TTL(key); ttl < 5*time.Second {
		t.Fatalf("expected key TTL to keep the long session alive, got %s", ttl)
	}
}

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, server, func() {
		_ = client.Close()
		server.Close()
	}
}
