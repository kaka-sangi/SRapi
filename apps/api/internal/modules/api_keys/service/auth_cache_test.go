package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

// TestAuthCacheHitReturnsCachedSnapshotWithoutTouchingStore is the regression
// guard for the wired hot-path: once an auth succeeds, the next call for the
// same plaintext must short-circuit the SQL store (proven by replacing the
// store with one that panics on FindByPrefix).
func TestAuthCacheHitReturnsCachedSnapshotWithoutTouchingStore(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	svc.SetAuthCache(NewAuthCache())

	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 11, Name: "cached"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Prime the cache.
	if _, err := svc.Authenticate(context.Background(), created.PlaintextKey); err != nil {
		t.Fatalf("authenticate (prime): %v", err)
	}

	// Wrap the store so any subsequent FindByPrefix would panic the test —
	// proving the second auth hit was served entirely from cache.
	svc.store = panicStore{inner: store}

	result, err := svc.Authenticate(context.Background(), created.PlaintextKey)
	if err != nil {
		t.Fatalf("authenticate (cached): %v", err)
	}
	if result.UserID != 11 || result.Key.Prefix != created.Key.Prefix {
		t.Fatalf("unexpected cached result: %+v", result)
	}
}

func TestAuthCacheNegativeEntryShortCircuitsRepeatedMisses(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	cache := NewAuthCache()
	svc.SetAuthCache(cache)

	// A well-formed plaintext that doesn't exist in the store.
	bogus, _, err := GeneratePlaintextKey()
	if err != nil {
		t.Fatalf("generate plaintext: %v", err)
	}
	_, err = svc.Authenticate(context.Background(), bogus)
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("first auth: expected ErrInvalidKey, got %v", err)
	}

	// Negative entry now seeded — switch to panic store to prove the
	// second probe stays out of SQL.
	svc.store = panicStore{inner: store}
	_, err = svc.Authenticate(context.Background(), bogus)
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("second auth: expected cached ErrInvalidKey, got %v", err)
	}
}

func TestAuthCacheInvalidatedOnDelete(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	svc.SetAuthCache(NewAuthCache())

	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 13, Name: "delete-me"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), created.PlaintextKey); err != nil {
		t.Fatalf("authenticate (prime): %v", err)
	}
	if err := svc.Delete(context.Background(), 13, created.Key.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), created.PlaintextKey); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("auth after delete: expected ErrInvalidKey, got %v", err)
	}
}

func TestAuthCacheInvalidatedOnDisableStatusUpdate(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	svc.SetAuthCache(NewAuthCache())

	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 17, Name: "disable-me"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), created.PlaintextKey); err != nil {
		t.Fatalf("authenticate (prime): %v", err)
	}
	disabled := contract.StatusDisabled
	if _, err := svc.Update(context.Background(), contract.UpdateRequest{UserID: 17, KeyID: created.Key.ID, Status: &disabled}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), created.PlaintextKey); !errors.Is(err, ErrKeyDisabled) {
		t.Fatalf("auth after disable: expected ErrKeyDisabled, got %v", err)
	}
}

func TestAuthCacheTTLExpiryForcesStoreFallback(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	now := time.Now().UTC()
	cache := NewAuthCacheWithConfig(AuthCacheConfig{
		TTL:         50 * time.Millisecond,
		NegativeTTL: 50 * time.Millisecond,
		Now:         func() time.Time { return now },
	})
	svc.SetAuthCache(cache)

	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 19, Name: "ttl"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), created.PlaintextKey); err != nil {
		t.Fatalf("prime: %v", err)
	}

	// Advance the cache clock past TTL.
	now = now.Add(time.Second)
	if _, _, _, found := cache.Get(context.Background(), created.PlaintextKey); found {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestAuthCacheEvictsLeastRecentlyUsedAtCap(t *testing.T) {
	cache := NewAuthCacheWithConfig(AuthCacheConfig{Cap: 2})
	for i := 0; i < 3; i++ {
		cache.PutPositive(context.Background(), "plaintext-"+strconv.Itoa(i), contract.APIKey{ID: i + 1}, i+1)
	}
	if cache.Len() != 2 {
		t.Fatalf("cap=2 not enforced: len=%d", cache.Len())
	}
	// The first inserted entry should have been evicted (LRU back).
	if _, _, _, found := cache.Get(context.Background(), "plaintext-0"); found {
		t.Fatal("expected LRU eviction of oldest entry")
	}
	if _, _, _, found := cache.Get(context.Background(), "plaintext-2"); !found {
		t.Fatal("expected most-recent entry to be retained")
	}
}

func TestAuthCacheCanceledContextSkipsLookup(t *testing.T) {
	cache := NewAuthCache()
	cache.PutPositive(context.Background(), "x", contract.APIKey{ID: 99}, 99)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, found := cache.Get(ctx, "x"); found {
		t.Fatal("cancelled ctx must not return a cached result")
	}
}

func TestRPMCounterIncrementsAndFlushZeroes(t *testing.T) {
	sink := &captureSink{}
	counter := NewRPMCounter(RPMCounterConfig{Sink: sink, FlushInterval: time.Second})
	counter.Increment(7)
	counter.Increment(7)
	counter.Increment(9)
	if err := counter.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got := sink.Snapshots()
	if len(got) != 2 {
		t.Fatalf("expected 2 keys flushed, got %d (%+v)", len(got), got)
	}
	totals := map[int]int64{}
	for _, snap := range got {
		totals[snap.KeyID] = snap.Requests
	}
	if totals[7] != 2 || totals[9] != 1 {
		t.Fatalf("unexpected totals: %+v", totals)
	}
	// A second flush with no increments must not call the sink again.
	if err := counter.Flush(context.Background()); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if len(sink.Snapshots()) != 2 {
		t.Fatal("sink should not be invoked when counters are already zero")
	}
}

func TestRPMCounterConcurrentIncrementsAreAtomic(t *testing.T) {
	counter := NewRPMCounter(RPMCounterConfig{FlushInterval: time.Hour})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				counter.Increment(42)
			}
		}()
	}
	wg.Wait()
	snaps := counter.Snapshot()
	if len(snaps) != 1 || snaps[0].KeyID != 42 || snaps[0].Requests != 5000 {
		t.Fatalf("expected 5000 increments under key 42, got %+v", snaps)
	}
}

// --- helpers -------------------------------------------------------------

type panicStore struct {
	inner contract.Store
}

func (p panicStore) Create(ctx context.Context, input contract.CreateStoredKey) (contract.APIKey, error) {
	return p.inner.Create(ctx, input)
}
func (p panicStore) Update(ctx context.Context, key contract.APIKey) (contract.APIKey, error) {
	return p.inner.Update(ctx, key)
}
func (p panicStore) Delete(ctx context.Context, id int) error {
	return p.inner.Delete(ctx, id)
}
func (p panicStore) FindByPrefix(ctx context.Context, prefix string) (contract.APIKey, error) {
	panic("FindByPrefix should not be called when cache is warm: " + prefix)
}
func (p panicStore) FindDeletedByPrefix(ctx context.Context, prefix string) (contract.APIKey, error) {
	return p.inner.FindDeletedByPrefix(ctx, prefix)
}
func (p panicStore) FindByID(ctx context.Context, id int) (contract.APIKey, error) {
	return p.inner.FindByID(ctx, id)
}
func (p panicStore) List(ctx context.Context) ([]contract.APIKey, error) {
	return p.inner.List(ctx)
}
func (p panicStore) ListByUser(ctx context.Context, userID int) ([]contract.APIKey, error) {
	return p.inner.ListByUser(ctx, userID)
}
func (p panicStore) TouchLastUsed(ctx context.Context, id int, usedAt time.Time) error {
	return p.inner.TouchLastUsed(ctx, id, usedAt)
}
func (p panicStore) ApplyCostUsage(ctx context.Context, input contract.CostUsageUpdate) (contract.APIKey, error) {
	return p.inner.ApplyCostUsage(ctx, input)
}
func (p panicStore) ResetUsage(ctx context.Context, id int) (contract.APIKey, error) {
	return p.inner.ResetUsage(ctx, id)
}

type captureSink struct {
	mu        sync.Mutex
	snapshots []APIKeyRPMSnapshot
}

func (s *captureSink) FlushAPIKeyRPM(_ context.Context, snaps []APIKeyRPMSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots = append(s.snapshots, snaps...)
	return nil
}

func (s *captureSink) Snapshots() []APIKeyRPMSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]APIKeyRPMSnapshot, len(s.snapshots))
	copy(out, s.snapshots)
	return out
}
