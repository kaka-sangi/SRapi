// Package memory is an in-process, TTL-bounded session→account affinity store.
//
// It is the fallback used when Redis is not configured. Bindings live only in
// this process, so stickiness is per-instance (good enough for single-node
// deployments and tests); multi-node deployments should use the Redis store so
// every node sees the same binding.
package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
)

type entry struct {
	accountID int
	expiresAt time.Time
}

// Store is an in-memory session→account affinity store.
type Store struct {
	mu              sync.Mutex
	bindings        map[string]entry
	accountSessions map[int]map[string]time.Time // accountID -> sessionID -> expiry
	now             func() time.Time
}

var _ contract.Store = (*Store)(nil)

// New returns an empty in-memory session affinity store.
func New() *Store {
	return &Store{
		bindings:        make(map[string]entry),
		accountSessions: make(map[int]map[string]time.Time),
		now:             func() time.Time { return time.Now().UTC() },
	}
}

// AddAccountSession records a conversation as active on an account.
func (s *Store) AddAccountSession(_ context.Context, accountID int, sessionID string, ttl time.Duration) error {
	sessionID = strings.TrimSpace(sessionID)
	if accountID <= 0 || sessionID == "" {
		return contract.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	expiresAt := now.Add(ttl)
	if ttl <= 0 {
		expiresAt = now.Add(time.Hour)
	}
	sessions := s.accountSessions[accountID]
	if sessions == nil {
		sessions = map[string]time.Time{}
		s.accountSessions[accountID] = sessions
	}
	for id, exp := range sessions {
		if now.After(exp) {
			delete(sessions, id)
		}
	}
	sessions[sessionID] = expiresAt
	return nil
}

// CountAccountSessionsExcluding counts distinct active sessions on an account
// other than sessionID.
func (s *Store) CountAccountSessionsExcluding(_ context.Context, accountID int, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	sessions := s.accountSessions[accountID]
	count := 0
	for id, exp := range sessions {
		if now.After(exp) {
			delete(sessions, id)
			continue
		}
		if id != sessionID {
			count++
		}
	}
	if len(sessions) == 0 {
		delete(s.accountSessions, accountID)
	}
	return count, nil
}

func storageKey(scope, key string) string {
	return scope + "\x00" + key
}

// Lookup resolves the longest-prefix binding for sessionKey, refreshing its TTL
// on a hit.
func (s *Store) Lookup(_ context.Context, scope, sessionKey string, ttl time.Duration) (contract.Binding, error) {
	candidates := contract.CandidateKeys(sessionKey)
	if len(candidates) == 0 {
		return contract.Binding{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, candidate := range candidates {
		storage := storageKey(scope, candidate)
		found, ok := s.bindings[storage]
		if !ok {
			continue
		}
		if !found.expiresAt.IsZero() && now.After(found.expiresAt) {
			delete(s.bindings, storage)
			continue
		}
		if ttl > 0 {
			found.expiresAt = now.Add(ttl)
			s.bindings[storage] = found
		}
		return contract.Binding{AccountID: found.accountID, MatchedKey: candidate}, nil
	}
	return contract.Binding{}, nil
}

// Bind stores sessionKey→accountID with the given TTL.
func (s *Store) Bind(_ context.Context, scope, sessionKey string, accountID int, ttl time.Duration) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || accountID <= 0 {
		return contract.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.evictExpiredLocked(now)
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}
	s.bindings[storageKey(scope, sessionKey)] = entry{accountID: accountID, expiresAt: expiresAt}
	return nil
}

// Release removes the binding for sessionKey.
func (s *Store) Release(_ context.Context, scope, sessionKey string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bindings, storageKey(scope, sessionKey))
	return nil
}

// StartGC launches a background goroutine that sweeps expired entries at the
// given interval. Ported from CLIProxyAPI's session cleanup goroutine that
// runs at ttl/2. Stops when ctx is canceled.
func (s *Store) StartGC(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.gc()
			}
		}
	}()
}

func (s *Store) gc() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, value := range s.bindings {
		if !value.expiresAt.IsZero() && now.After(value.expiresAt) {
			delete(s.bindings, key)
		}
	}
	for accountID, sessions := range s.accountSessions {
		for id, exp := range sessions {
			if now.After(exp) {
				delete(sessions, id)
			}
		}
		if len(sessions) == 0 {
			delete(s.accountSessions, accountID)
		}
	}
}

// evictExpiredLocked opportunistically drops expired entries so the map does
// not grow without bound. Callers must hold s.mu.
func (s *Store) evictExpiredLocked(now time.Time) {
	for key, value := range s.bindings {
		if !value.expiresAt.IsZero() && now.After(value.expiresAt) {
			delete(s.bindings, key)
		}
	}
}
