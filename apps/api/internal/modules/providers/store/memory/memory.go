package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.Provider
	byName map[string]int
}

func New() *Store {
	return &Store{
		nextID: 1,
		byID:   map[int]contract.Provider{},
		byName: map[string]int{},
	}
}

func (s *Store) Create(_ context.Context, input contract.CreateStoredProvider) (contract.Provider, error) {
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
		Capabilities: cloneMap(input.Capabilities),
		ConfigSchema: cloneMap(input.ConfigSchema),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.byID[provider.ID] = provider
	s.byName[strings.ToLower(provider.Name)] = provider.ID
	s.nextID++
	return provider, nil
}

func (s *Store) Update(_ context.Context, provider contract.Provider) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[provider.ID]; !ok {
		return contract.Provider{}, errors.New("provider not found")
	}
	stored := provider
	stored.Capabilities = cloneMap(provider.Capabilities)
	stored.ConfigSchema = cloneMap(provider.ConfigSchema)
	s.byID[stored.ID] = stored
	return stored, nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.byID[id]
	if !ok || provider.DeletedAt != nil {
		return contract.Provider{}, errors.New("provider not found")
	}
	return provider, nil
}

func (s *Store) FindByName(_ context.Context, name string) (contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return contract.Provider{}, errors.New("provider not found")
	}
	provider := s.byID[id]
	if provider.DeletedAt != nil {
		return contract.Provider{}, errors.New("provider not found")
	}
	return provider, nil
}

func (s *Store) List(_ context.Context) ([]contract.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Provider, 0, len(s.byID))
	for _, provider := range s.byID {
		if provider.DeletedAt != nil {
			continue
		}
		out = append(out, provider)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) SoftDelete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.byID[id]
	if !ok || provider.DeletedAt != nil {
		return errors.New("provider not found")
	}
	now := time.Now().UTC().Unix()
	provider.DeletedAt = &now
	provider.Status = contract.StatusArchived
	provider.UpdatedAt = time.Now().UTC()
	s.byID[id] = provider
	delete(s.byName, strings.ToLower(provider.Name))
	return nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
	}
	return cloned
}
