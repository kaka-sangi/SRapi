package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
)

// Store is an in-memory implementation of the account group rate limit store.
type Store struct {
	mu    sync.Mutex
	byID  map[int]contract.Limit
	seq   int
	clock func() time.Time
}

func New() *Store {
	return &Store{byID: make(map[int]contract.Limit), clock: time.Now}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) UpsertLimit(ctx context.Context, input contract.UpsertLimit) (contract.Limit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, limit := range s.byID {
		if limit.GroupID == input.GroupID {
			limit.RPMLimit = input.RPMLimit
			limit.TPMLimit = input.TPMLimit
			limit.MaxConcurrency = input.MaxConcurrency
			limit.Enabled = input.Enabled
			limit.UpdatedAt = now
			s.byID[id] = limit
			return limit, nil
		}
	}
	s.seq++
	limit := contract.Limit{
		ID:             s.seq,
		GroupID:        input.GroupID,
		RPMLimit:       input.RPMLimit,
		TPMLimit:       input.TPMLimit,
		MaxConcurrency: input.MaxConcurrency,
		Enabled:        input.Enabled,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.byID[limit.ID] = limit
	return limit, nil
}

func (s *Store) DeleteByGroup(ctx context.Context, groupID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, limit := range s.byID {
		if limit.GroupID == groupID {
			delete(s.byID, id)
			return nil
		}
	}
	return contract.ErrNotFound
}

func (s *Store) FindByGroup(ctx context.Context, groupID int) (contract.Limit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, limit := range s.byID {
		if limit.GroupID == groupID {
			return limit, nil
		}
	}
	return contract.Limit{}, contract.ErrNotFound
}

func (s *Store) ListLimits(ctx context.Context) ([]contract.Limit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Limit, 0, len(s.byID))
	for _, limit := range s.byID {
		out = append(out, limit)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GroupID < out[j].GroupID })
	return out, nil
}
