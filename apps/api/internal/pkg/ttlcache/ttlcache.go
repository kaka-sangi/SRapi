// Package ttlcache provides a tiny single-value read-through cache used to keep
// per-request hot paths from re-reading slow-changing configuration (admin
// settings, transform rules) from the database on every gateway request.
package ttlcache

import (
	"context"
	"sync"
	"time"
)

// Value caches one value loaded through a caller-supplied loader for a fixed
// TTL. It is safe for concurrent use. Concurrent cache misses collapse into a
// single load; on loader failure the last successfully loaded value (even an
// expired one) is served so a transient store outage does not take the hot
// path down with it. Invalidate discards the cached value entirely, so the
// next Get observes writes immediately on this instance.
type Value[T any] struct {
	ttl time.Duration
	now func() time.Time

	mu       sync.Mutex
	value    T
	loadedAt time.Time
	loaded   bool
}

// New builds a cache holding values for ttl. A non-positive ttl disables
// caching (every Get loads). now overrides the clock for tests; nil uses
// time.Now.
func New[T any](ttl time.Duration, now func() time.Time) *Value[T] {
	if now == nil {
		now = time.Now
	}
	return &Value[T]{ttl: ttl, now: now}
}

// Get returns the cached value when it is still fresh, otherwise loads a new
// one. When the loader fails but an earlier load succeeded, the stale value is
// returned with a nil error (stale-while-error) — callers treating these reads
// as best-effort configuration prefer slightly old data over hard failure.
func (v *Value[T]) Get(ctx context.Context, load func(context.Context) (T, error)) (T, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loaded && v.ttl > 0 && v.now().Sub(v.loadedAt) < v.ttl {
		return v.value, nil
	}
	loadedValue, err := load(ctx)
	if err != nil {
		if v.loaded {
			return v.value, nil
		}
		var zero T
		return zero, err
	}
	v.value = loadedValue
	v.loadedAt = v.now()
	v.loaded = true
	return v.value, nil
}

// Invalidate drops the cached value so the next Get reloads. Mutators call it
// after a successful write to make same-instance read-after-write exact.
func (v *Value[T]) Invalidate() {
	v.mu.Lock()
	defer v.mu.Unlock()
	var zero T
	v.value = zero
	v.loaded = false
}
