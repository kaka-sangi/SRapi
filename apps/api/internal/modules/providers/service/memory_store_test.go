package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

type memoryStore struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.Provider
	byName map[string]int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		nextID: 1,
		byID:   map[int]contract.Provider{},
		byName: map[string]int{},
	}
}

func (s *memoryStore) Create(_ context.Context, input contract.CreateStoredProvider) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	provider := contract.Provider{
		ID:           s.nextID,
		Name:         input.Name,
		DisplayName:  input.DisplayName,
		AdapterType:  input.AdapterType,
		Protocol:     input.Protocol,
		Status:       input.Status,
		Capabilities: input.Capabilities,
		ConfigSchema: input.ConfigSchema,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.byID[provider.ID] = provider
	s.byName[strings.ToLower(input.Name)] = provider.ID
	s.nextID++
	return provider, nil
}

func (s *memoryStore) Update(_ context.Context, provider contract.Provider) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[provider.ID]; !ok {
		return contract.Provider{}, errors.New("provider not found")
	}
	s.byID[provider.ID] = provider
	return provider, nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.byID[id]
	if !ok {
		return contract.Provider{}, errors.New("provider not found")
	}
	return provider, nil
}

func (s *memoryStore) FindByName(_ context.Context, name string) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return contract.Provider{}, errors.New("provider not found")
	}
	provider := s.byID[id]
	return provider, nil
}

func (s *memoryStore) List(_ context.Context) ([]contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Provider, 0, len(s.byID))
	for _, provider := range s.byID {
		out = append(out, provider)
	}
	return out, nil
}

func (s *memoryStore) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.byID[id]
	if !ok {
		return errors.New("provider not found")
	}
	delete(s.byID, id)
	delete(s.byName, strings.ToLower(provider.Name))
	return nil
}
