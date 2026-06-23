// Package service implements an in-process per-(account, model)
// rate-limit cooldown registry. The original sub2api parity was
// per-account only; SRapi extends it to a two-level key so a 429 on
// gemini-2.5-pro never blocks a different model on the same account.
//
// Behavior:
//   - RecordRateLimitHit(accountID, model, retryAfter) — extend cooldown
//     for that (accountID, model) entry. An empty model targets the
//     account-wide entry, used for failures the caller knows affect the
//     whole credential (e.g. 401 auth errors, account suspension).
//   - IsAccountInCooldown(accountID, model) reports whether the model
//     would be skipped right now. The check ORs the (accountID, model)
//     entry with the (accountID, "") entry — an account-wide hit blocks
//     every model regardless of per-model state.
//   - Sliding-window escalation (N=5 hits in 10min → 10min temp-disable)
//     is scoped to each (accountID, model) pair so a chatty model can't
//     drag the rest of an account into temp-disable.
//
// State is bounded by a single LRU cap across all (accountID, model)
// entries — the same eviction story as the original per-account map.
package service

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

const (
	// defaultMaxAccounts caps the LRU so a long tail of one-off entries
	// can never grow the map unbounded. Each unique (accountID, model)
	// pair consumes one slot, so the cap covers both account-level and
	// model-level entries in the same budget.
	defaultMaxAccounts = 4096

	// consecutiveDisableThreshold is the count of consecutive 429s within
	// consecutiveWindow that escalates from a normal cooldown to a longer
	// temp-disable.
	consecutiveDisableThreshold = 5
	consecutiveWindow           = 10 * time.Minute
	// baseCooldown is the temp-disable duration when hits first reach
	// consecutiveDisableThreshold.  Each additional hit doubles it up to
	// maxDisableCooldown:
	//   5 hits → 10 min, 6 → 20 min, 7+ → 30 min (cap).
	baseCooldown       = 10 * time.Minute
	maxDisableCooldown = 30 * time.Minute

	// minCooldown clamps a zero/negative retryAfter to a deterministic
	// minimum so any record always carries a real cooldown window.
	minCooldown = 1 * time.Second
	// maxCooldown clamps a malicious or buggy upstream retry-after header
	// (e.g. 1 year) to a sane upper bound.
	maxCooldown = 2 * time.Hour
)

// Key identifies a single cooldown entry. Model="" represents the
// account-wide entry that blocks every model for the given AccountID.
type Key struct {
	AccountID int64
	Model     string
}

// HitCounter is an optional distributed hit counter for cross-node
// escalation. When set, RecordRateLimitHit increments the distributed
// counter and uses its value for the escalation threshold instead of the
// local hits slice. This ensures that N hits spread across M nodes still
// trigger escalation when the cluster-wide total reaches the threshold.
type HitCounter interface {
	// IncrHit atomically increments the hit count for the key and returns
	// the new total. The counter auto-expires after the consecutive window.
	IncrHit(key string, window time.Duration) (int64, error)
}

// Service is the in-process per-(account, model) cooldown registry.
// Safe for concurrent use; all state is held under a single mutex.
type Service struct {
	mu          sync.Mutex
	entries     map[Key]*list.Element
	order       *list.List
	maxAccounts int
	now         func() time.Time
	hitCounter  HitCounter
}

type cooldownEntry struct {
	key       Key
	unblockAt time.Time
	hits      []time.Time
	// tempDisabledUntil takes precedence over unblockAt and represents
	// the escalated cooldown applied after consecutiveDisableThreshold
	// hits.
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
		entries:     make(map[Key]*list.Element),
		order:       list.New(),
		maxAccounts: maxAccounts,
		now:         time.Now,
	}
}

// SetHitCounter wires an optional distributed hit counter. When set,
// escalation checks use the cluster-wide count instead of local hits.
func (s *Service) SetHitCounter(hc HitCounter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hitCounter = hc
}

// RecordRateLimitHit records a 429 for (accountID, model). Model is
// normalized (trimmed) so callers can pass canonical model names
// directly. An empty model targets the account-wide entry.
func (s *Service) RecordRateLimitHit(accountID int64, model string, retryAfter time.Duration) {
	if accountID <= 0 {
		return
	}
	if retryAfter < minCooldown {
		retryAfter = minCooldown
	}
	if retryAfter > maxCooldown {
		retryAfter = maxCooldown
	}
	key := normalizeKey(accountID, model)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.touchLocked(key)
	unblock := now.Add(retryAfter)
	if unblock.After(entry.unblockAt) {
		entry.unblockAt = unblock
	}

	cutoff := now.Add(-consecutiveWindow)
	filtered := entry.hits[:0]
	for _, t := range entry.hits {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	filtered = append(filtered, now)
	entry.hits = filtered

	// Determine the hit count for escalation: prefer the distributed counter
	// (cluster-wide) when available, fall back to the local hits slice.
	hitCount := len(entry.hits)
	if s.hitCounter != nil {
		counterKey := "rlc:" + key.hitCounterKey()
		if n, err := s.hitCounter.IncrHit(counterKey, consecutiveWindow); err == nil && int(n) > hitCount {
			hitCount = int(n)
		}
	}

	if hitCount >= consecutiveDisableThreshold {
		disableUntil := now.Add(escalatedCooldown(hitCount))
		if disableUntil.After(entry.tempDisabledUntil) {
			entry.tempDisabledUntil = disableUntil
		}
	}
}

// IsAccountInCooldown reports whether (accountID, model) is currently
// in cooldown and the wall-clock time at which it will be eligible
// again. The check ORs the per-model entry with the account-wide entry
// — an account-wide block trumps the per-model state.
func (s *Service) IsAccountInCooldown(accountID int64, model string) (bool, time.Time) {
	if accountID <= 0 {
		return false, time.Time{}
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	perModel := normalizeKey(accountID, model)
	accountWide := Key{AccountID: accountID}

	active1, unblock1 := s.checkLocked(perModel, now)
	if perModel == accountWide {
		// Same key — single lookup covers both planes.
		return active1, unblock1
	}
	active2, unblock2 := s.checkLocked(accountWide, now)
	switch {
	case active1 && active2:
		// Pick the later unblock so callers see the strictest wait.
		if unblock1.After(unblock2) {
			return true, unblock1
		}
		return true, unblock2
	case active1:
		return true, unblock1
	case active2:
		return true, unblock2
	}
	return false, time.Time{}
}

// FilterCooldownedAccounts returns the subset of accountIDs that are
// currently in cooldown for the supplied model, in stable input order.
// An account-wide cooldown counts as cooldowned for every model.
func (s *Service) FilterCooldownedAccounts(accountIDs []int64, model string) []int64 {
	if len(accountIDs) == 0 {
		return nil
	}
	cooled := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if active, _ := s.IsAccountInCooldown(id, model); active {
			cooled = append(cooled, id)
		}
	}
	return cooled
}

// CooldownedIDs returns the distinct accountIDs currently in cooldown
// for the supplied model. Order is unspecified. Account-wide entries
// always appear; per-model entries only when their model matches.
func (s *Service) CooldownedIDs(model string) []int64 {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	model = normalizeModel(model)
	seen := make(map[int64]struct{})
	for key, elem := range s.entries {
		entry := elem.Value.(*cooldownEntry)
		if !(entry.tempDisabledUntil.After(now) || entry.unblockAt.After(now)) {
			delete(s.entries, key)
			s.order.Remove(elem)
			continue
		}
		// Account-wide entries always match; per-model entries only
		// when their model equals the requested model.
		if key.Model != "" && key.Model != model {
			continue
		}
		seen[key.AccountID] = struct{}{}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// Reset clears the cooldown entry for the given (accountID, model).
func (s *Service) Reset(accountID int64, model string) {
	if accountID <= 0 {
		return
	}
	key := normalizeKey(accountID, model)
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.entries[key]
	if !ok {
		return
	}
	delete(s.entries, key)
	s.order.Remove(elem)
}

// ResetAccount clears every cooldown entry for accountID across all
// models — used by admin tools that mark an account positively
// recovered.
func (s *Service) ResetAccount(accountID int64) {
	if accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, elem := range s.entries {
		if key.AccountID != accountID {
			continue
		}
		delete(s.entries, key)
		s.order.Remove(elem)
	}
}

// touchLocked returns the entry for key, creating it on demand and
// applying LRU eviction. Must be called with mu held.
func (s *Service) touchLocked(key Key) *cooldownEntry {
	if elem, ok := s.entries[key]; ok {
		s.order.MoveToFront(elem)
		return elem.Value.(*cooldownEntry)
	}
	for s.maxAccounts > 0 && s.order.Len() >= s.maxAccounts {
		back := s.order.Back()
		if back == nil {
			break
		}
		old := back.Value.(*cooldownEntry)
		delete(s.entries, old.key)
		s.order.Remove(back)
	}
	entry := &cooldownEntry{key: key}
	elem := s.order.PushFront(entry)
	s.entries[key] = elem
	return entry
}

// checkLocked reports whether the entry at key is currently
// cooldowned, opportunistically dropping expired entries. Must be
// called with mu held.
func (s *Service) checkLocked(key Key, now time.Time) (bool, time.Time) {
	elem, ok := s.entries[key]
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
	delete(s.entries, key)
	s.order.Remove(elem)
	return false, time.Time{}
}

// escalatedCooldown computes the temp-disable duration using exponential
// backoff: baseCooldown * 2^(hits - threshold), capped at maxDisableCooldown.
//
//	5 hits → 10 min (base)
//	6 hits → 20 min
//	7+ hits → 30 min (cap)
func escalatedCooldown(hits int) time.Duration {
	exponent := hits - consecutiveDisableThreshold
	if exponent < 0 {
		exponent = 0
	}
	d := baseCooldown << exponent // baseCooldown * 2^exponent
	if d > maxDisableCooldown {
		return maxDisableCooldown
	}
	return d
}

func (k Key) hitCounterKey() string {
	if k.Model == "" {
		return fmt.Sprintf("%d", k.AccountID)
	}
	return fmt.Sprintf("%d:%s", k.AccountID, k.Model)
}

func normalizeKey(accountID int64, model string) Key {
	return Key{AccountID: accountID, Model: normalizeModel(model)}
}

func normalizeModel(model string) string {
	// Trim leading/trailing whitespace but preserve case so the key
	// matches what the gateway computes from the canonical model.
	if model == "" {
		return ""
	}
	out := model
	for len(out) > 0 && (out[0] == ' ' || out[0] == '\t') {
		out = out[1:]
	}
	for len(out) > 0 {
		last := out[len(out)-1]
		if last != ' ' && last != '\t' {
			break
		}
		out = out[:len(out)-1]
	}
	return out
}

// Size returns the number of tracked cooldown entries. Test/visibility only.
func (s *Service) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}
