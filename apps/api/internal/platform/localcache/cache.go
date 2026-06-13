package localcache

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

const numShards = 16

// Config controls cache behaviour. Zero values are replaced with defaults.
type Config struct {
	MaxEntries    int           // per-shard cap (default 1024)
	DefaultTTL    time.Duration // default 5m
	SweepInterval time.Duration // default 30s
}

func (c *Config) defaults() {
	if c.MaxEntries <= 0 {
		c.MaxEntries = 1024
	}
	if c.DefaultTTL <= 0 {
		c.DefaultTTL = 5 * time.Minute
	}
	if c.SweepInterval <= 0 {
		c.SweepInterval = 30 * time.Second
	}
}

// Stats holds cache-wide counters. Size is a point-in-time snapshot.
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}

// Cache is a sharded, generic, in-memory cache with TTL support.
type Cache[V any] struct {
	cfg    Config
	shards [numShards]shard[V]
	stop   chan struct{}

	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

type entry[V any] struct {
	key       string
	value     V
	expiresAt time.Time
	createdAt time.Time
}

type shard[V any] struct {
	mu      sync.RWMutex
	items   map[string]int // key -> index in entries
	entries []entry[V]
}

// New creates a cache and starts a background sweep goroutine. The caller
// must call Close when the cache is no longer needed.
func New[V any](cfg Config) *Cache[V] {
	cfg.defaults()
	c := &Cache[V]{
		cfg:  cfg,
		stop: make(chan struct{}),
	}
	for i := range c.shards {
		c.shards[i].items = make(map[string]int)
		c.shards[i].entries = make([]entry[V], 0, cfg.MaxEntries)
	}
	go c.sweepLoop()
	return c
}

// Get retrieves a value by key. Expired entries are lazily removed.
func (c *Cache[V]) Get(key string) (V, bool) {
	s := c.shardFor(key)

	s.mu.RLock()
	idx, ok := s.items[key]
	if !ok {
		s.mu.RUnlock()
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	e := s.entries[idx]
	s.mu.RUnlock()

	if time.Now().After(e.expiresAt) {
		// Entry expired — upgrade to write lock and evict.
		s.mu.Lock()
		// Re-check after acquiring write lock.
		idx, ok = s.items[key]
		if ok && time.Now().After(s.entries[idx].expiresAt) {
			c.removeFromShard(s, idx)
			c.evictions.Add(1)
		}
		s.mu.Unlock()
		c.misses.Add(1)
		var zero V
		return zero, false
	}

	c.hits.Add(1)
	return e.value, true
}

// Set stores a value using the default TTL from Config.
func (c *Cache[V]) Set(key string, value V) {
	c.SetWithTTL(key, value, c.cfg.DefaultTTL)
}

// SetWithTTL stores a value with an explicit TTL.
func (c *Cache[V]) SetWithTTL(key string, value V, ttl time.Duration) {
	s := c.shardFor(key)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update in place if the key already exists.
	if idx, ok := s.items[key]; ok {
		s.entries[idx].value = value
		s.entries[idx].expiresAt = now.Add(ttl)
		s.entries[idx].createdAt = now
		return
	}

	// Evict if at capacity.
	if len(s.entries) >= c.cfg.MaxEntries {
		c.evictOne(s, now)
	}

	idx := len(s.entries)
	s.entries = append(s.entries, entry[V]{
		key:       key,
		value:     value,
		expiresAt: now.Add(ttl),
		createdAt: now,
	})
	s.items[key] = idx
}

// GetOrSet returns the cached value if present, otherwise calls fn to compute
// it, stores the result, and returns it. fn is called with the shard lock held,
// so it should be fast and non-blocking. For expensive loads, use Get + Set with
// a singleflight group instead.
func (c *Cache[V]) GetOrSet(key string, fn func() V) V {
	if v, ok := c.Get(key); ok {
		return v
	}
	v := fn()
	c.Set(key, v)
	return v
}

// SetIfAbsent stores a value only if the key does not already exist (or is expired).
// Returns true if the value was set, false if the key already existed.
func (c *Cache[V]) SetIfAbsent(key string, value V) bool {
	if _, ok := c.Get(key); ok {
		return false
	}
	c.Set(key, value)
	return true
}

// SetWithTTLIfAbsent is like SetIfAbsent but uses a custom TTL.
func (c *Cache[V]) SetWithTTLIfAbsent(key string, value V, ttl time.Duration) bool {
	if _, ok := c.Get(key); ok {
		return false
	}
	c.SetWithTTL(key, value, ttl)
	return true
}

// Delete removes a key from the cache.
func (c *Cache[V]) Delete(key string) {
	s := c.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	if idx, ok := s.items[key]; ok {
		c.removeFromShard(s, idx)
	}
}

// Clear removes all entries from every shard.
func (c *Cache[V]) Clear() {
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		s.items = make(map[string]int)
		s.entries = s.entries[:0]
		s.mu.Unlock()
	}
}

// Len returns the total number of live entries across all shards.
func (c *Cache[V]) Len() int {
	n := 0
	for i := range c.shards {
		c.shards[i].mu.RLock()
		n += len(c.shards[i].entries)
		c.shards[i].mu.RUnlock()
	}
	return n
}

// Stats returns a snapshot of cache metrics.
func (c *Cache[V]) Stats() Stats {
	return Stats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
		Size:      c.Len(),
	}
}

// Keys returns all live (non-expired) keys in the cache.
func (c *Cache[V]) Keys() []string {
	now := time.Now()
	var keys []string
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.RLock()
		for _, e := range s.entries {
			if now.Before(e.expiresAt) {
				keys = append(keys, e.key)
			}
		}
		s.mu.RUnlock()
	}
	return keys
}

// ForEach calls fn for every live (non-expired) entry. Iteration order is
// undefined. fn must not call cache methods — doing so would deadlock.
func (c *Cache[V]) ForEach(fn func(key string, value V)) {
	now := time.Now()
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.RLock()
		for _, e := range s.entries {
			if now.Before(e.expiresAt) {
				fn(e.key, e.value)
			}
		}
		s.mu.RUnlock()
	}
}

// InvalidatePrefix removes all entries whose key starts with the given prefix.
func (c *Cache[V]) InvalidatePrefix(prefix string) int {
	removed := 0
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		var toRemove []string
		for key := range s.items {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				toRemove = append(toRemove, key)
			}
		}
		for _, key := range toRemove {
			if idx, ok := s.items[key]; ok {
				c.removeFromShard(s, idx)
				removed++
			}
		}
		s.mu.Unlock()
	}
	return removed
}

// Close stops the background sweep goroutine.
func (c *Cache[V]) Close() {
	close(c.stop)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// shardFor selects a shard using FNV-1a.
func (c *Cache[V]) shardFor(key string) *shard[V] {
	h := fnv.New32a()
	h.Write([]byte(key))
	return &c.shards[h.Sum32()%numShards]
}

// evictOne removes one entry from a full shard. It prefers the oldest expired
// entry; if none are expired it evicts the oldest entry by insertion time.
// Caller must hold s.mu.
func (c *Cache[V]) evictOne(s *shard[V], now time.Time) {
	oldestExpiredIdx := -1
	oldestIdx := 0

	for i, e := range s.entries {
		if now.After(e.expiresAt) {
			if oldestExpiredIdx == -1 || e.createdAt.Before(s.entries[oldestExpiredIdx].createdAt) {
				oldestExpiredIdx = i
			}
		}
		if e.createdAt.Before(s.entries[oldestIdx].createdAt) {
			oldestIdx = i
		}
	}

	victim := oldestIdx
	if oldestExpiredIdx != -1 {
		victim = oldestExpiredIdx
	}

	c.removeFromShard(s, victim)
	c.evictions.Add(1)
}

// removeFromShard deletes the entry at idx by swapping with the last element.
// Caller must hold s.mu.
func (c *Cache[V]) removeFromShard(s *shard[V], idx int) {
	last := len(s.entries) - 1
	delete(s.items, s.entries[idx].key)

	if idx != last {
		s.entries[idx] = s.entries[last]
		s.items[s.entries[idx].key] = idx
	}

	var zero entry[V]
	s.entries[last] = zero // clear ref for GC
	s.entries = s.entries[:last]
}

// sweepLoop periodically removes expired entries from every shard.
func (c *Cache[V]) sweepLoop() {
	ticker := time.NewTicker(c.cfg.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.sweep()
		}
	}
}

func (c *Cache[V]) sweep() {
	now := time.Now()
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		j := 0
		for j < len(s.entries) {
			if now.After(s.entries[j].expiresAt) {
				c.removeFromShard(s, j)
				c.evictions.Add(1)
				// Don't increment j — the swap brought a new entry into this slot.
				continue
			}
			j++
		}
		s.mu.Unlock()
	}
}
