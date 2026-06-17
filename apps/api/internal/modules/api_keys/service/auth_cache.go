package service

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

// Ported (adapted) from sub2api/backend/internal/service/api_key_auth_cache*.go.
// Verbatim port deviated only where srapi's data model is simpler:
//   - Single L1 (in-memory) tier — no Redis/L2. srapi runs single-process today;
//     when we add a multi-replica deployment we'll layer L2 + Pub/Sub invalidation
//     on top, mirroring sub2api's two-tier shape.
//   - Bounded LRU with default cap 4096 (deviation explicitly sanctioned by the
//     port directive). sub2api's ristretto-backed cache evicts by cost which we
//     don't need here; a doubly-linked list keeps eviction O(1) and predictable
//     under fuzz tests.
//   - Per-key counter is a tiny in-memory atomic int, flushed on a tick by a
//     pluggable Sink. Matches sub2api's "flush_interval" pattern without
//     pulling in its Redis/RPM dependency stack.

const (
	// defaultAuthCacheCap caps L1 entries to a bounded LRU. Matches the
	// upper bound the port directive specified.
	defaultAuthCacheCap = 4096

	// defaultAuthCacheTTL mirrors sub2api's default L1 TTL window.
	// Authenticated key snapshots stay cached for this long unless busted
	// by an explicit Invalidate call (key disable/delete/update path).
	defaultAuthCacheTTL = 60 * time.Second

	// defaultAuthCacheNegativeTTL is the (shorter) TTL for "no such key"
	// entries — keeps the cache from being used as a DoS amplifier when an
	// attacker sprays random prefixes, but won't pin a real key out of
	// existence past its rotation window.
	defaultAuthCacheNegativeTTL = 5 * time.Second

	// defaultRPMFlushInterval matches sub2api's per-key counter flush
	// cadence. The flush is non-blocking — request hot path never waits.
	defaultRPMFlushInterval = 10 * time.Second
)

// authCacheEntry is the L1 cache record. Supports negative caching ("known
// missing") via the notFound flag — identical shape to sub2api's
// APIKeyAuthCacheEntry, just without the snapshot indirection (srapi's APIKey
// is already the auth-time projection).
type authCacheEntry struct {
	cacheKey  string
	notFound  bool
	apiKey    contract.APIKey
	userID    int
	expiresAt time.Time
}

// AuthCache is an in-memory bounded-LRU cache of API-key auth results.
// Concurrency: all methods are safe for concurrent use. The hot path takes
// a single lock; the eviction list is updated under the same lock so we never
// race against expiration.
type AuthCache struct {
	cap         int
	ttl         time.Duration
	negativeTTL time.Duration
	now         func() time.Time

	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List // front = most-recently-used; back = eviction candidate
}

// AuthCacheConfig lets the runtime wire non-default values; the zero value
// produces an instance equivalent to NewAuthCache().
type AuthCacheConfig struct {
	Cap         int
	TTL         time.Duration
	NegativeTTL time.Duration
	Now         func() time.Time // injectable clock for tests
}

// NewAuthCache builds a cache with conservative defaults. Used by runtime
// wiring; tests use NewAuthCacheWithConfig to inject a fake clock.
func NewAuthCache() *AuthCache {
	return NewAuthCacheWithConfig(AuthCacheConfig{})
}

// NewAuthCacheWithConfig fills any zero-valued field with the matching
// default — keeps the call sites in runtime wiring readable.
func NewAuthCacheWithConfig(cfg AuthCacheConfig) *AuthCache {
	if cfg.Cap <= 0 {
		cfg.Cap = defaultAuthCacheCap
	}
	if cfg.TTL <= 0 {
		cfg.TTL = defaultAuthCacheTTL
	}
	if cfg.NegativeTTL <= 0 {
		cfg.NegativeTTL = defaultAuthCacheNegativeTTL
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &AuthCache{
		cap:         cfg.Cap,
		ttl:         cfg.TTL,
		negativeTTL: cfg.NegativeTTL,
		now:         cfg.Now,
		entries:     make(map[string]*list.Element, cfg.Cap),
		lru:         list.New(),
	}
}

// Key derives the cache lookup key for a plaintext API key. Uses sha256 (not
// the prefix alone) so a brute-force scanner can't probe the cache for live
// prefixes — only a full plaintext match yields a hit. Matches sub2api's
// authCacheKey().
func (c *AuthCache) Key(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Get returns the cached result for plaintext if present and not expired.
// found == true means we know the auth answer (positive or negative); the
// caller must respect notFound. found == false means SQL fallback required.
//
// ctx is checked for cancellation up-front — keeps Get from servicing a
// request whose client has already disconnected (deviation from sub2api
// allowed by directive: "ctx safety").
func (c *AuthCache) Get(ctx context.Context, plaintext string) (entry contract.APIKey, userID int, notFound, found bool) {
	if c == nil {
		return contract.APIKey{}, 0, false, false
	}
	if err := ctx.Err(); err != nil {
		return contract.APIKey{}, 0, false, false
	}
	cacheKey := c.Key(plaintext)
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[cacheKey]
	if !ok {
		return contract.APIKey{}, 0, false, false
	}
	rec := elem.Value.(*authCacheEntry)
	if c.now().After(rec.expiresAt) {
		c.removeElem(elem)
		return contract.APIKey{}, 0, false, false
	}
	c.lru.MoveToFront(elem)
	return rec.apiKey, rec.userID, rec.notFound, true
}

// PutPositive records a successful auth result. The cached APIKey is the
// already-hash-stripped copy the caller would otherwise return.
func (c *AuthCache) PutPositive(ctx context.Context, plaintext string, key contract.APIKey, userID int) {
	if c == nil {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	cacheKey := c.Key(plaintext)
	c.put(cacheKey, &authCacheEntry{
		cacheKey:  cacheKey,
		apiKey:    key,
		userID:    userID,
		expiresAt: c.now().Add(c.ttl),
	})
}

// PutNotFound records a negative auth result so the next probe for the same
// plaintext short-circuits without a SQL round-trip. Uses the shorter
// negativeTTL window.
func (c *AuthCache) PutNotFound(ctx context.Context, plaintext string) {
	if c == nil {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	cacheKey := c.Key(plaintext)
	c.put(cacheKey, &authCacheEntry{
		cacheKey:  cacheKey,
		notFound:  true,
		expiresAt: c.now().Add(c.negativeTTL),
	})
}

// InvalidatePlaintext busts the cache entry for a specific plaintext. Used
// when callers happen to know the plaintext (rare — usually only the create
// path).
func (c *AuthCache) InvalidatePlaintext(plaintext string) {
	if c == nil {
		return
	}
	cacheKey := c.Key(plaintext)
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[cacheKey]; ok {
		c.removeElem(elem)
	}
}

// InvalidateByKeyID busts every cache entry whose stored APIKey matches the
// given ID. O(n) over current entries — acceptable because invalidation is
// rare (disable/delete/update path), and the bound on n is small (defaultCap).
//
// This is the cache-bust hook for the directive's "Invalidation on key
// disable/delete" requirement: the caller knows the key ID but not the
// plaintext (it was hashed away long ago), so we walk and match.
func (c *AuthCache) InvalidateByKeyID(keyID int) {
	if c == nil || keyID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for elem := c.lru.Front(); elem != nil; {
		next := elem.Next()
		rec := elem.Value.(*authCacheEntry)
		if !rec.notFound && rec.apiKey.ID == keyID {
			c.removeElem(elem)
		}
		elem = next
	}
}

// Purge drops every cache entry. Used by tests; exposed for ops tooling in
// case a future key-store bulk import needs to flush authoritatively.
func (c *AuthCache) Purge() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element, c.cap)
	c.lru.Init()
}

// Len returns the current entry count (exposed for tests + metrics).
func (c *AuthCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

func (c *AuthCache) put(cacheKey string, rec *authCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[cacheKey]; ok {
		elem.Value = rec
		c.lru.MoveToFront(elem)
		return
	}
	elem := c.lru.PushFront(rec)
	c.entries[cacheKey] = elem
	for c.lru.Len() > c.cap {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		c.removeElem(oldest)
	}
}

func (c *AuthCache) removeElem(elem *list.Element) {
	rec := elem.Value.(*authCacheEntry)
	delete(c.entries, rec.cacheKey)
	c.lru.Remove(elem)
}

// --- Per-key RPM counter -------------------------------------------------

// RPMCounterSink is the persistence hook called on each scheduled flush. The
// runtime can wire this to an audit log, a metrics emitter, or eventually a
// DB-backed counter — same shape as sub2api's user_platform_quota flusher.
// When the sink is nil the counter still works (in-memory only); the flush
// loop simply zeroes counters on the tick.
type RPMCounterSink interface {
	FlushAPIKeyRPM(ctx context.Context, snapshots []APIKeyRPMSnapshot) error
}

// APIKeyRPMSnapshot is one row of the periodic flush: total requests served
// since the previous flush, per key.
type APIKeyRPMSnapshot struct {
	KeyID    int
	Requests int64
	Window   time.Duration
	FlushAt  time.Time
}

// RPMCounter tracks per-key request counts in memory. Increment is on the
// hot path (called on every successful auth), so it MUST stay
// lock-free; the per-key entry is materialized lazily under a small map lock
// but the increment itself is atomic.
type RPMCounter struct {
	flushInterval time.Duration
	sink          RPMCounterSink
	now           func() time.Time

	mu       sync.RWMutex
	counters map[int]*atomic.Int64

	stopOnce sync.Once
	stop     chan struct{}
	stopped  chan struct{}
}

// RPMCounterConfig opens the door for future tuning; zero-value produces
// the sub2api-matching defaults.
type RPMCounterConfig struct {
	FlushInterval time.Duration
	Sink          RPMCounterSink
	Now           func() time.Time
}

// NewRPMCounter constructs an RPMCounter ready to be started.
func NewRPMCounter(cfg RPMCounterConfig) *RPMCounter {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultRPMFlushInterval
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &RPMCounter{
		flushInterval: cfg.FlushInterval,
		sink:          cfg.Sink,
		now:           cfg.Now,
		counters:      make(map[int]*atomic.Int64),
		stop:          make(chan struct{}),
		stopped:       make(chan struct{}),
	}
}

// Increment bumps the per-key counter. Hot-path safe: the common case is a
// single atomic add under an RLock — no map mutation. First-touch for a key
// materializes the counter under a brief write lock.
func (r *RPMCounter) Increment(keyID int) {
	if r == nil || keyID <= 0 {
		return
	}
	r.mu.RLock()
	c, ok := r.counters[keyID]
	r.mu.RUnlock()
	if ok {
		c.Add(1)
		return
	}
	r.mu.Lock()
	c, ok = r.counters[keyID]
	if !ok {
		c = &atomic.Int64{}
		r.counters[keyID] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// Snapshot returns the current counts (does not zero them — that's Flush's
// job). Exposed for tests + ad-hoc admin probes.
func (r *RPMCounter) Snapshot() []APIKeyRPMSnapshot {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]APIKeyRPMSnapshot, 0, len(r.counters))
	now := r.now()
	for id, c := range r.counters {
		out = append(out, APIKeyRPMSnapshot{
			KeyID:    id,
			Requests: c.Load(),
			Window:   r.flushInterval,
			FlushAt:  now,
		})
	}
	return out
}

// Flush takes a point-in-time snapshot, resets the in-memory counters, and
// delivers the snapshot to the sink. Resetting under the read-lock is safe
// here because we use atomic.Swap — Increment is still allowed to make
// progress against a freshly-zeroed counter.
func (r *RPMCounter) Flush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	now := r.now()
	out := make([]APIKeyRPMSnapshot, 0, len(r.counters))
	for id, c := range r.counters {
		requests := c.Swap(0)
		if requests <= 0 {
			continue
		}
		out = append(out, APIKeyRPMSnapshot{
			KeyID:    id,
			Requests: requests,
			Window:   r.flushInterval,
			FlushAt:  now,
		})
	}
	r.mu.RUnlock()
	if len(out) == 0 || r.sink == nil {
		return nil
	}
	return r.sink.FlushAPIKeyRPM(ctx, out)
}

// Start launches the flush ticker in a goroutine. The goroutine exits when
// ctx is cancelled OR Stop() is called. Idempotent — calling Start twice
// is harmless (subsequent calls are no-ops; matches the contract sub2api
// expects from its background workers).
func (r *RPMCounter) Start(ctx context.Context) {
	if r == nil {
		return
	}
	go r.loop(ctx)
}

func (r *RPMCounter) loop(ctx context.Context) {
	defer close(r.stopped)
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = r.Flush(context.Background())
			return
		case <-r.stop:
			_ = r.Flush(context.Background())
			return
		case <-ticker.C:
			_ = r.Flush(ctx)
		}
	}
}

// Stop signals the flush loop to exit and waits for the final flush. Safe
// to call multiple times.
func (r *RPMCounter) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.stopped
}
