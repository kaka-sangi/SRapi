package service

import (
	"context"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/domain"
)

type memoryStore struct {
	mu       sync.Mutex
	nextID   int
	byID     map[int]contract.APIKey
	byPrefix map[string]int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		nextID:   1,
		byID:     map[int]contract.APIKey{},
		byPrefix: map[string]int{},
	}
}

func (s *memoryStore) Create(_ context.Context, input contract.CreateStoredKey) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	key := contract.APIKey{
		ID:            s.nextID,
		UserID:        input.UserID,
		WorkspaceID:   cloneInt(input.WorkspaceID),
		Name:          input.Name,
		Prefix:        input.Prefix,
		Hash:          input.Hash,
		Status:        input.Status,
		Scopes:        append([]string(nil), input.Scopes...),
		AllowedModels: append([]string(nil), input.AllowedModels...),
		GroupIDs:      append([]int(nil), input.GroupIDs...),
		RPMLimit:      input.RPMLimit,
		TPMLimit:      input.TPMLimit,
		CostQuota:     cloneString(input.CostQuota),
		CostUsed:      "0.00000000",
		CostLimit5h:   cloneString(input.CostLimit5h),
		CostUsed5h:    "0.00000000",
		CostLimit1d:   cloneString(input.CostLimit1d),
		CostUsed1d:    "0.00000000",
		CostLimit7d:   cloneString(input.CostLimit7d),
		CostUsed7d:    "0.00000000",
		ExpiresAt:     input.ExpiresAt,
		CreatedAt:     now,
	}
	s.nextID++
	s.byID[key.ID] = key
	s.byPrefix[key.Prefix] = key.ID
	return key, nil
}

func (s *memoryStore) Update(_ context.Context, key contract.APIKey) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.byID[key.ID]
	if !ok {
		return contract.APIKey{}, contract.ErrKeyNotFound
	}
	key.UserID = stored.UserID
	key.WorkspaceID = cloneInt(stored.WorkspaceID)
	key.Prefix = stored.Prefix
	key.Hash = stored.Hash
	key.CreatedAt = stored.CreatedAt
	key.RPMLimit = stored.RPMLimit
	key.TPMLimit = stored.TPMLimit
	key.CostUsed = stored.CostUsed
	key.CostUsed5h = stored.CostUsed5h
	key.CostWindowStart5h = stored.CostWindowStart5h
	key.CostUsed1d = stored.CostUsed1d
	key.CostWindowStart1d = stored.CostWindowStart1d
	key.CostUsed7d = stored.CostUsed7d
	key.CostWindowStart7d = stored.CostWindowStart7d
	// ExpiresAt is editable via Update; keep whatever the service passes through.
	key.LastUsedAt = stored.LastUsedAt
	key.Scopes = append([]string(nil), key.Scopes...)
	key.AllowedModels = append([]string(nil), key.AllowedModels...)
	key.GroupIDs = append([]int(nil), key.GroupIDs...)
	s.byID[key.ID] = key
	return key, nil
}

func (s *memoryStore) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[id]
	if !ok {
		return contract.ErrKeyNotFound
	}
	delete(s.byPrefix, key.Prefix)
	delete(s.byID, id)
	return nil
}

func (s *memoryStore) FindByPrefix(_ context.Context, prefix string) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byPrefix[prefix]
	if !ok {
		return contract.APIKey{}, ErrKeyNotFound
	}
	return s.byID[id], nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[id]
	if !ok {
		return contract.APIKey{}, ErrKeyNotFound
	}
	return key, nil
}

func (s *memoryStore) ListByUser(_ context.Context, userID int) ([]contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]contract.APIKey, 0)
	for _, key := range s.byID {
		if key.UserID == userID {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *memoryStore) List(_ context.Context) ([]contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]contract.APIKey, 0, len(s.byID))
	for _, key := range s.byID {
		keys = append(keys, key)
	}
	return keys, nil
}

func (s *memoryStore) TouchLastUsed(_ context.Context, id int, usedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[id]
	if !ok {
		return ErrKeyNotFound
	}
	key.LastUsedAt = &usedAt
	s.byID[id] = key
	return nil
}

func (s *memoryStore) ApplyCostUsage(_ context.Context, input contract.CostUsageUpdate) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[input.KeyID]
	if !ok {
		return contract.APIKey{}, ErrKeyNotFound
	}
	key = domain.ApplyCostUsage(key, input)
	s.byID[key.ID] = key
	return key, nil
}

func (s *memoryStore) setStatus(prefix string, status contract.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.byPrefix[prefix]
	key := s.byID[id]
	key.Status = status
	s.byID[id] = key
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
