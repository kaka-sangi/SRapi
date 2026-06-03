package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
)

// Store is an in-memory contract.Store used by the memory storage backend and
// httpserver tests. Keyed by the (UserID, Platform) unique pair.
type Store struct {
	mu    sync.Mutex
	byID  map[int]contract.Quota
	seq   int
	clock func() time.Time
}

func New() *Store {
	return &Store{byID: make(map[int]contract.Quota), clock: time.Now}
}

func (s *Store) now() time.Time {
	return s.clock().UTC()
}

func (s *Store) UpsertQuota(ctx context.Context, input contract.UpsertQuota) (contract.Quota, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, quota := range s.byID {
		if quota.UserID == input.UserID && quota.Platform == input.Platform {
			quota.DailyLimit = input.DailyLimit
			quota.WeeklyLimit = input.WeeklyLimit
			quota.MonthlyLimit = input.MonthlyLimit
			quota.Currency = input.Currency
			quota.Enabled = input.Enabled
			quota.UpdatedAt = now
			s.byID[id] = quota
			return quota, nil
		}
	}
	s.seq++
	quota := contract.Quota{
		ID:           s.seq,
		UserID:       input.UserID,
		Platform:     input.Platform,
		DailyLimit:   input.DailyLimit,
		WeeklyLimit:  input.WeeklyLimit,
		MonthlyLimit: input.MonthlyLimit,
		Currency:     input.Currency,
		Enabled:      input.Enabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.byID[quota.ID] = quota
	return quota, nil
}

func (s *Store) DeleteByUserPlatform(ctx context.Context, userID int, platform string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, quota := range s.byID {
		if quota.UserID == userID && quota.Platform == platform {
			delete(s.byID, id)
			return nil
		}
	}
	return contract.ErrNotFound
}

func (s *Store) FindByUserPlatform(ctx context.Context, userID int, platform string) (contract.Quota, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, quota := range s.byID {
		if quota.UserID == userID && quota.Platform == platform {
			return quota, nil
		}
	}
	return contract.Quota{}, contract.ErrNotFound
}

func (s *Store) ListByUser(ctx context.Context, userID int) ([]contract.Quota, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Quota, 0, len(s.byID))
	for _, quota := range s.byID {
		if quota.UserID == userID {
			out = append(out, quota)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Platform < out[j].Platform })
	return out, nil
}
