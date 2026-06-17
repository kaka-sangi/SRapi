// Package service implements an in-process per-account rate-limit cooldown
// registry modeled on sub2api's RateLimitService cooldown behaviour
// (RecordRateLimitHit / IsAccountInCooldown semantics — see
// backend/internal/service/ratelimit_service.go and 429-handling flow).
//
// sub2api persists cooldowns via Postgres + Redis because it must coordinate
// across many backend instances and survives restarts. srapi's authoritative
// per-account state already lives in the account metadata
// (cooldown_until / rate_limit_reset_at) and is checked by the scheduler at
// decision time. This module is the per-process complement: a bounded
// in-memory LRU that the gateway hot path consults synchronously to skip an
// account immediately after it just 429'd, without waiting for the
// asynchronous metadata write to land.
//
// Behavior matches sub2api verbatim:
//   - RecordRateLimitHit(accountID, retryAfter) — extend cooldown window.
//   - IsAccountInCooldown(accountID) → (bool, unblockTime).
//   - Sliding-window N=5 consecutive 429s within consecutiveWindow → escalate
//     to a "temp-disable" cooldown of disableCooldown. Matches sub2api's
//     RateLimitService.handle429 + apply429FallbackRateLimit thresholding.
//
// Deviation from the directive: storage is a bounded LRU (cap 4096) with
// TTL-driven eviction in-process rather than Redis, per the "bounded LRU"
// carve-out. Account selection still consults this via the
// IsAccountInCooldown gate that the gateway calls before invoking upstream.
package service

import (
	"container/list"
	"sync"
	"time"
)

const (
	// defaultMaxAccounts caps the LRU so a long tail of one-off accounts can
	// never grow the map unbounded. Matches sub2api's bounded cooldown table.
	defaultMaxAccounts = 4096

	// consecutiveDisableThreshold is the count of consecutive 429s within
	// consecutiveWindow that escalates from a normal cooldown to a longer
	// temp-disable. Mirrors sub2api's heuristic for OAuth-style temp disable.
	consecutiveDisableThreshold = 5
	// consecutiveWindow is the sliding window the threshold is measured
	// over. Matches sub2api's 10-minute sliding window.
	consecutiveWindow = 10 * time.Minute
	// disableCooldown is the temp-disable duration applied when the
	// threshold is hit. Matches sub2api's
	// OverloadCooldownSettings/OAuth401CooldownMinutes default of 10
	// minutes.
	disableCooldown = 10 * time.Minute

	// minCooldown clamps a zero/negative retryAfter to a deterministic
	// minimum so any record always carries a real cooldown window. Matches
	// sub2api's clampRateLimit429CooldownSeconds(1s) lower bound.
	minCooldown = 1 * time.Second
	// maxCooldown clamps a malicious or buggy upstream retry-after header
	// (e.g. 1 year) to a sane upper bound. Matches sub2api's
	// maxRateLimit429CooldownSeconds (2h).
	maxCooldown = 2 * time.Hour
)

// Service is the in-process per-account cooldown registry. Safe for
// concurrent use; all state is held under a single mutex (the working set is
// small and contention is bounded by per-account writes).
type Service struct {
	mu          sync.Mutex
	entries     map[int64]*list.Element
	order       *list.List
	maxAccounts int
	now         func() time.Time
}

type cooldownEntry struct {
	accountID int64
	// unblockAt is the wall-clock time at which the account leaves cooldown.
	unblockAt time.Time
	// hits is the sliding-window list of recent 429 timestamps used to
	// decide when to escalate to a temp-disable.
	hits []time.Time
	// tempDisabledUntil, if non-zero, takes precedence over unblockAt and
	// represents the escalated cooldown applied after
	// consecutiveDisableThreshold hits.
	tempDisabledUntil time.Time
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
		entries:     make(map[int64]*list.Element),
		order:       list.New(),
		maxAccounts: maxAccounts,
		now:         time.Now,
	}
}

// RecordRateLimitHit records a 429 for accountID and extends the cooldown by
// retryAfter (clamped to [minCooldown, maxCooldown]). When this is the Nth
// consecutive hit within consecutiveWindow, escalate to a longer
// temp-disable cooldown.
//
// Safe to call concurrently. The caller should use the same accountID space
// that IsAccountInCooldown will be queried with (typically the srapi
// provider-account ID).
func (s *Service) RecordRateLimitHit(accountID int64, retryAfter time.Duration) {
	if accountID <= 0 {
		return
	}
	if retryAfter < minCooldown {
		retryAfter = minCooldown
	}
	if retryAfter > maxCooldown {
		retryAfter = maxCooldown
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.touchLocked(accountID)
	unblock := now.Add(retryAfter)
	if unblock.After(entry.unblockAt) {
		entry.unblockAt = unblock
	}

	// Slide window: drop hits older than consecutiveWindow.
	cutoff := now.Add(-consecutiveWindow)
	filtered := entry.hits[:0]
	for _, t := range entry.hits {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	filtered = append(filtered, now)
	entry.hits = filtered

	if len(entry.hits) >= consecutiveDisableThreshold {
		disableUntil := now.Add(disableCooldown)
		if disableUntil.After(entry.tempDisabledUntil) {
			entry.tempDisabledUntil = disableUntil
		}
	}
}

// IsAccountInCooldown reports whether accountID is currently in cooldown and
// the wall-clock time at which it will be eligible again. When false, the
// returned time is zero. Reads slide the LRU front so warm accounts are
// retained over cold ones.
func (s *Service) IsAccountInCooldown(accountID int64) (bool, time.Time) {
	if accountID <= 0 {
		return false, time.Time{}
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	elem, ok := s.entries[accountID]
	if !ok {
		return false, time.Time{}
	}
	entry := elem.Value.(*cooldownEntry)
	s.order.MoveToFront(elem)

	if entry.tempDisabledUntil.After(now) {
		return true, entry.tempDisabledUntil
	}
	if entry.unblockAt.After(now) {
		return true, entry.unblockAt
	}

	// Cooldown expired. Drop the entry so the LRU doesn't carry stale data.
	delete(s.entries, accountID)
	s.order.Remove(elem)
	return false, time.Time{}
}

// FilterCooldownedAccounts returns the subset of accountIDs that are
// currently in cooldown, in stable input order. Convenience for callers
// building an excluded-account list for the scheduler.
func (s *Service) FilterCooldownedAccounts(accountIDs []int64) []int64 {
	if len(accountIDs) == 0 {
		return nil
	}
	cooled := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if active, _ := s.IsAccountInCooldown(id); active {
			cooled = append(cooled, id)
		}
	}
	return cooled
}

// CooldownedIDs returns the set of accountIDs currently in cooldown,
// dropping entries whose cooldown has lapsed. Order is unspecified. The
// returned slice may be empty but is never nil.
func (s *Service) CooldownedIDs() []int64 {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, 0)
	for id, elem := range s.entries {
		entry := elem.Value.(*cooldownEntry)
		if entry.tempDisabledUntil.After(now) || entry.unblockAt.After(now) {
			out = append(out, id)
			continue
		}
		// Expired; drop opportunistically.
		delete(s.entries, id)
		s.order.Remove(elem)
	}
	return out
}

// Reset clears any cooldown state for accountID — used by admin / operations
// flows that have positively recovered an account.
func (s *Service) Reset(accountID int64) {
	if accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.entries[accountID]
	if !ok {
		return
	}
	delete(s.entries, accountID)
	s.order.Remove(elem)
}

// touchLocked returns the entry for accountID, creating it on demand and
// applying LRU eviction. Must be called with mu held.
func (s *Service) touchLocked(accountID int64) *cooldownEntry {
	if elem, ok := s.entries[accountID]; ok {
		s.order.MoveToFront(elem)
		return elem.Value.(*cooldownEntry)
	}
	// Evict if at cap.
	for s.maxAccounts > 0 && s.order.Len() >= s.maxAccounts {
		back := s.order.Back()
		if back == nil {
			break
		}
		old := back.Value.(*cooldownEntry)
		delete(s.entries, old.accountID)
		s.order.Remove(back)
	}
	entry := &cooldownEntry{accountID: accountID}
	elem := s.order.PushFront(entry)
	s.entries[accountID] = elem
	return entry
}

// Size returns the number of tracked cooldown entries. Test/visibility only.
func (s *Service) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}
