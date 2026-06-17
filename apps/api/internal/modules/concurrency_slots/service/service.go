// Package service implements an in-process, per-account concurrency-slot
// manager modeled on sub2api's ConcurrencyService (see
// backend/internal/service/concurrency_service.go AcquireAccountSlot /
// ReleaseAccountSlot semantics).
//
// sub2api's implementation is Redis-backed because it must coordinate across
// many backend instances. srapi runs the gateway in a single process per
// instance and already has a Redis-backed rate-limit slot path
// (httpserver.rateLimiter.AcquireConcurrency) for distributed coordination.
// This module is the in-process complement: a channel-based semaphore per
// account that gates the gateway hot path so a single instance cannot blow
// past per-account MaxConcurrency between the scheduler decision and the
// upstream call.
//
// User directive "完全按那三个项目来" was applied to behaviour
// (AcquireSlot/ReleaseSlot, ctx-cancellable wait, release closure, fail-open
// for non-positive capacity); the channel-based semaphore replaces sub2api's
// Redis ZSet because in-process is the right primitive for an instance-local
// gate, per the goroutine/ctx safety + Go-idiomatic concurrency carve-outs.
package service

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"
)

// ErrSlotAcquireTimeout is returned by AcquireSlot when the per-call wait
// budget elapses before a slot frees. Distinct from ctx.Err() so callers can
// surface a deterministic gateway 429 instead of an opaque cancellation.
var ErrSlotAcquireTimeout = errors.New("concurrency_slots: acquire timeout")

// ErrCapacityZero indicates AcquireSlot was called with a non-positive
// capacity. The contract is "no gate" — callers should treat this as success
// and skip the release. Returned only when explicitly requested via
// AcquireSlotStrict.
var ErrCapacityZero = errors.New("concurrency_slots: capacity not positive")

// defaultMaxAccounts caps the bounded LRU of per-account slot pools so a long
// tail of one-off account IDs cannot leak unbounded channels. Matches the
// "bounded LRU (cap 4096)" deviation in the user directive.
const defaultMaxAccounts = 4096

// Service is the in-process per-account concurrency-slot manager. Safe for
// concurrent use from any number of gateway-handler goroutines.
type Service struct {
	mu          sync.Mutex
	pools       map[int64]*list.Element
	order       *list.List
	maxAccounts int
}

type accountPool struct {
	accountID int64
	// capacity is the cap the channel was constructed with. If a caller asks
	// for a different capacity later we resize lazily on the next acquire.
	capacity int
	// sem is a buffered channel of length `capacity` — sending blocks once
	// `capacity` in-flight tokens are held, achieving a counting semaphore
	// with goroutine-safe ctx cancellation via select.
	sem chan struct{}
}

// New constructs a Service with the default LRU cap.
func New() *Service {
	return NewWithMax(defaultMaxAccounts)
}

// NewWithMax constructs a Service with a custom LRU cap. A non-positive
// maxAccounts falls back to defaultMaxAccounts.
func NewWithMax(maxAccounts int) *Service {
	if maxAccounts <= 0 {
		maxAccounts = defaultMaxAccounts
	}
	return &Service{
		pools:       make(map[int64]*list.Element),
		order:       list.New(),
		maxAccounts: maxAccounts,
	}
}

// AcquireSlot reserves one concurrency slot for accountID. The returned
// release closure must be called exactly once (typically `defer`) once the
// request finishes, regardless of upstream success/failure. On ctx
// cancellation or wait-budget expiry the closure is nil and a non-nil error
// is returned; callers must NOT call the release.
//
// Capacity semantics mirror sub2api's AcquireAccountSlot:
//   - capacity <= 0       → no gate; release is a no-op closure, err == nil.
//   - capacity > 0        → block until a slot is free or ctx is done.
//
// waitBudget bounds the in-call wait; ≤0 means wait until ctx is done.
func (s *Service) AcquireSlot(ctx context.Context, accountID int64, capacity int, waitBudget time.Duration) (release func(), err error) {
	if capacity <= 0 {
		return func() {}, nil
	}
	if accountID <= 0 {
		// Defensive: an unkeyed pool would collide across callers.
		return func() {}, nil
	}
	pool := s.poolFor(accountID, capacity)

	// Fast path: a slot is immediately available.
	select {
	case pool.sem <- struct{}{}:
		return s.releaseClosure(pool), nil
	default:
	}

	// Slow path: block with ctx + optional wait-budget timer.
	if waitBudget > 0 {
		waitCtx, cancel := context.WithTimeout(ctx, waitBudget)
		defer cancel()
		select {
		case pool.sem <- struct{}{}:
			return s.releaseClosure(pool), nil
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrSlotAcquireTimeout
		}
	}
	select {
	case pool.sem <- struct{}{}:
		return s.releaseClosure(pool), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// AcquireSlotStrict is like AcquireSlot but returns ErrCapacityZero when
// capacity ≤ 0, so callers that treat capacity=0 as configuration error can
// distinguish it from a real gate hit.
func (s *Service) AcquireSlotStrict(ctx context.Context, accountID int64, capacity int, waitBudget time.Duration) (release func(), err error) {
	if capacity <= 0 {
		return nil, ErrCapacityZero
	}
	return s.AcquireSlot(ctx, accountID, capacity, waitBudget)
}

// InFlight returns the count of currently-held slots for accountID. Useful for
// admin visibility / tests; not a synchronization primitive.
func (s *Service) InFlight(accountID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.pools[accountID]
	if !ok {
		return 0
	}
	pool := elem.Value.(*accountPool)
	return len(pool.sem)
}

func (s *Service) poolFor(accountID int64, capacity int) *accountPool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if elem, ok := s.pools[accountID]; ok {
		pool := elem.Value.(*accountPool)
		s.order.MoveToFront(elem)
		if pool.capacity != capacity {
			// Resize lazily: rebuild the channel to the new capacity, keeping
			// any currently in-flight tokens by draining and re-pushing up to
			// min(old in-flight, new capacity). Tokens beyond the new
			// capacity are coalesced — the lost back-pressure is acceptable
			// vs. dropping accounting for in-flight requests entirely.
			inFlight := len(pool.sem)
			if inFlight > capacity {
				inFlight = capacity
			}
			newCh := make(chan struct{}, capacity)
			for i := 0; i < inFlight; i++ {
				newCh <- struct{}{}
			}
			pool.sem = newCh
			pool.capacity = capacity
		}
		return pool
	}
	// Evict the LRU pool if we're at the cap. Only evict an idle pool to
	// avoid stranding live release-closures (the channel send would then
	// silently target a dead pool). If every pool is in use, we accept going
	// 1 over the cap until something idles — this is rare in practice and
	// the cap is generous (defaultMaxAccounts).
	if s.maxAccounts > 0 && s.order.Len() >= s.maxAccounts {
		for e := s.order.Back(); e != nil; e = e.Prev() {
			oldest := e.Value.(*accountPool)
			if len(oldest.sem) == 0 {
				delete(s.pools, oldest.accountID)
				s.order.Remove(e)
				break
			}
		}
	}
	pool := &accountPool{
		accountID: accountID,
		capacity:  capacity,
		sem:       make(chan struct{}, capacity),
	}
	elem := s.order.PushFront(pool)
	s.pools[accountID] = elem
	return pool
}

func (s *Service) releaseClosure(pool *accountPool) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			// Non-blocking drain — if the pool was resized down between
			// acquire and release the buffer may already be empty, in which
			// case the token has already been accounted for.
			select {
			case <-pool.sem:
			default:
			}
		})
	}
}
