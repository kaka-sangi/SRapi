package memory

import (
	"context"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
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
		ID:            s.nextID,
		UserID:        input.UserID,
		Name:          input.Name,
		Prefix:        input.Prefix,
		Hash:          input.Hash,
		Status:        input.Status,
		Scopes:        append([]string(nil), input.Scopes...),
		AllowedModels: append([]string(nil), input.AllowedModels...),
		GroupIDs:      append([]int(nil), input.GroupIDs...),
		RPMLimit:      input.RPMLimit,
		TPMLimit:      input.TPMLimit,
		ExpiresAt:     input.ExpiresAt,
		CreatedAt:     now,
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
	key.Prefix = stored.Prefix
	key.Hash = stored.Hash
	key.CreatedAt = stored.CreatedAt
	key.RPMLimit = cloneIntPointer(stored.RPMLimit)
	key.TPMLimit = cloneIntPointer(stored.TPMLimit)
	key.ExpiresAt = cloneTimePointer(stored.ExpiresAt)
	key.LastUsedAt = cloneTimePointer(stored.LastUsedAt)
	key.Scopes = append([]string(nil), key.Scopes...)
	key.AllowedModels = append([]string(nil), key.AllowedModels...)
	key.GroupIDs = append([]int(nil), key.GroupIDs...)
	s.byID[key.ID] = key
	return key, nil
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

func cloneKey(key contract.APIKey) contract.APIKey {
	key.Scopes = append([]string(nil), key.Scopes...)
	key.AllowedModels = append([]string(nil), key.AllowedModels...)
	key.GroupIDs = append([]int(nil), key.GroupIDs...)
	key.RPMLimit = cloneIntPointer(key.RPMLimit)
	key.TPMLimit = cloneIntPointer(key.TPMLimit)
	key.ExpiresAt = cloneTimePointer(key.ExpiresAt)
	key.LastUsedAt = cloneTimePointer(key.LastUsedAt)
	return key
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
