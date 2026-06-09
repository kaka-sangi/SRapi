package memory

import (
	"context"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/domain"
)

type Store struct {
	mu       sync.Mutex
	nextID   int
	byID     map[int]contract.APIKey
	byPrefix map[string]int
}

func New() *Store {
	return &Store{
		nextID:   1,
		byID:     map[int]contract.APIKey{},
		byPrefix: map[string]int{},
	}
}

func (s *Store) Create(_ context.Context, input contract.CreateStoredKey) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	key := contract.APIKey{
		ID:               s.nextID,
		UserID:           input.UserID,
		WorkspaceID:      cloneIntPointer(input.WorkspaceID),
		Name:             input.Name,
		Prefix:           input.Prefix,
		Hash:             input.Hash,
		Status:           input.Status,
		Scopes:           append([]string(nil), input.Scopes...),
		AllowedModels:    append([]string(nil), input.AllowedModels...),
		GroupIDs:         append([]int(nil), input.GroupIDs...),
		RPMLimit:         cloneIntPointer(input.RPMLimit),
		TPMLimit:         cloneIntPointer(input.TPMLimit),
		ConcurrencyLimit: cloneIntPointer(input.ConcurrencyLimit),
		RequestLimit5h:   cloneIntPointer(input.RequestLimit5h),
		RequestLimit1d:   cloneIntPointer(input.RequestLimit1d),
		RequestLimit7d:   cloneIntPointer(input.RequestLimit7d),
		CostQuota:        cloneStringPointer(input.CostQuota),
		CostUsed:         "0.00000000",
		CostLimit5h:      cloneStringPointer(input.CostLimit5h),
		CostUsed5h:       "0.00000000",
		CostLimit1d:      cloneStringPointer(input.CostLimit1d),
		CostUsed1d:       "0.00000000",
		CostLimit7d:      cloneStringPointer(input.CostLimit7d),
		CostUsed7d:       "0.00000000",
		AllowedIPs:       append([]string(nil), input.AllowedIPs...),
		DeniedIPs:        append([]string(nil), input.DeniedIPs...),
		ExpiresAt:        cloneTimePointer(input.ExpiresAt),
		CreatedAt:        now,
	}
	s.nextID++
	s.byID[key.ID] = key
	s.byPrefix[key.Prefix] = key.ID
	return key, nil
}

func (s *Store) Update(_ context.Context, key contract.APIKey) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.byID[key.ID]
	if !ok {
		return contract.APIKey{}, contract.ErrKeyNotFound
	}
	key.UserID = stored.UserID
	key.WorkspaceID = cloneIntPointer(stored.WorkspaceID)
	key.Prefix = stored.Prefix
	key.Hash = stored.Hash
	key.CreatedAt = stored.CreatedAt
	// ExpiresAt is editable via Update; the service carries the current value
	// through when unchanged, so persist whatever it hands us.
	key.ExpiresAt = cloneTimePointer(key.ExpiresAt)
	key.LastUsedAt = cloneTimePointer(stored.LastUsedAt)
	key.RPMLimit = cloneIntPointer(key.RPMLimit)
	key.TPMLimit = cloneIntPointer(key.TPMLimit)
	key.ConcurrencyLimit = cloneIntPointer(key.ConcurrencyLimit)
	key.RequestLimit5h = cloneIntPointer(key.RequestLimit5h)
	key.RequestLimit1d = cloneIntPointer(key.RequestLimit1d)
	key.RequestLimit7d = cloneIntPointer(key.RequestLimit7d)
	key.CostQuota = cloneStringPointer(key.CostQuota)
	key.CostUsed = stored.CostUsed
	key.CostLimit5h = cloneStringPointer(key.CostLimit5h)
	key.CostUsed5h = stored.CostUsed5h
	key.CostWindowStart5h = cloneTimePointer(stored.CostWindowStart5h)
	key.CostLimit1d = cloneStringPointer(key.CostLimit1d)
	key.CostUsed1d = stored.CostUsed1d
	key.CostWindowStart1d = cloneTimePointer(stored.CostWindowStart1d)
	key.CostLimit7d = cloneStringPointer(key.CostLimit7d)
	key.CostUsed7d = stored.CostUsed7d
	key.CostWindowStart7d = cloneTimePointer(stored.CostWindowStart7d)
	key.Scopes = append([]string(nil), key.Scopes...)
	key.AllowedModels = append([]string(nil), key.AllowedModels...)
	key.GroupIDs = append([]int(nil), key.GroupIDs...)
	key.AllowedIPs = append([]string(nil), key.AllowedIPs...)
	key.DeniedIPs = append([]string(nil), key.DeniedIPs...)
	s.byID[key.ID] = key
	return key, nil
}

func (s *Store) Delete(_ context.Context, id int) error {
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

func (s *Store) FindByPrefix(_ context.Context, prefix string) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byPrefix[prefix]
	if !ok {
		return contract.APIKey{}, contract.ErrKeyNotFound
	}
	return cloneKey(s.byID[id]), nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[id]
	if !ok {
		return contract.APIKey{}, contract.ErrKeyNotFound
	}
	return cloneKey(key), nil
}

func (s *Store) ListByUser(_ context.Context, userID int) ([]contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]contract.APIKey, 0)
	for _, key := range s.byID {
		if key.UserID == userID {
			keys = append(keys, cloneKey(key))
		}
	}
	return keys, nil
}

func (s *Store) List(_ context.Context) ([]contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]contract.APIKey, 0, len(s.byID))
	for _, key := range s.byID {
		keys = append(keys, cloneKey(key))
	}
	return keys, nil
}

func (s *Store) TouchLastUsed(_ context.Context, id int, usedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[id]
	if !ok {
		return contract.ErrKeyNotFound
	}
	key.LastUsedAt = &usedAt
	s.byID[id] = key
	return nil
}

func (s *Store) ApplyCostUsage(_ context.Context, input contract.CostUsageUpdate) (contract.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[input.KeyID]
	if !ok {
		return contract.APIKey{}, contract.ErrKeyNotFound
	}
	key = domain.ApplyCostUsage(key, input)
	s.byID[key.ID] = key
	return cloneKey(key), nil
}

func cloneKey(key contract.APIKey) contract.APIKey {
	key.Scopes = append([]string(nil), key.Scopes...)
	key.AllowedModels = append([]string(nil), key.AllowedModels...)
	key.GroupIDs = append([]int(nil), key.GroupIDs...)
	key.WorkspaceID = cloneIntPointer(key.WorkspaceID)
	key.RPMLimit = cloneIntPointer(key.RPMLimit)
	key.TPMLimit = cloneIntPointer(key.TPMLimit)
	key.ConcurrencyLimit = cloneIntPointer(key.ConcurrencyLimit)
	key.RequestLimit5h = cloneIntPointer(key.RequestLimit5h)
	key.RequestLimit1d = cloneIntPointer(key.RequestLimit1d)
	key.RequestLimit7d = cloneIntPointer(key.RequestLimit7d)
	key.CostQuota = cloneStringPointer(key.CostQuota)
	key.CostLimit5h = cloneStringPointer(key.CostLimit5h)
	key.CostWindowStart5h = cloneTimePointer(key.CostWindowStart5h)
	key.CostLimit1d = cloneStringPointer(key.CostLimit1d)
	key.CostWindowStart1d = cloneTimePointer(key.CostWindowStart1d)
	key.CostLimit7d = cloneStringPointer(key.CostLimit7d)
	key.CostWindowStart7d = cloneTimePointer(key.CostWindowStart7d)
	key.AllowedIPs = append([]string(nil), key.AllowedIPs...)
	key.DeniedIPs = append([]string(nil), key.DeniedIPs...)
	key.ExpiresAt = cloneTimePointer(key.ExpiresAt)
	key.LastUsedAt = cloneTimePointer(key.LastUsedAt)
	return key
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
