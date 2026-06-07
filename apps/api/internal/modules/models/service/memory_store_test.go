package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/models/contract"
)

type memoryStore struct {
	mu                sync.Mutex
	nextID            int
	nextAliasID       int
	nextMappingID     int
	byID              map[int]contract.Model
	byCanonicalName   map[string]int
	aliasesByID       map[int]contract.ModelAlias
	aliasByName       map[string]int
	aliasIDsByModel   map[int][]int
	mappingsByID      map[int]contract.ModelProviderMapping
	mappingByKey      map[string]int
	mappingIDsByModel map[int][]int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		nextID:            1,
		nextAliasID:       1,
		nextMappingID:     1,
		byID:              map[int]contract.Model{},
		byCanonicalName:   map[string]int{},
		aliasesByID:       map[int]contract.ModelAlias{},
		aliasByName:       map[string]int{},
		aliasIDsByModel:   map[int][]int{},
		mappingsByID:      map[int]contract.ModelProviderMapping{},
		mappingByKey:      map[string]int{},
		mappingIDsByModel: map[int][]int{},
	}
}

func (s *memoryStore) Create(_ context.Context, input contract.CreateStoredModel) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	model := contract.Model{
		ID:              s.nextID,
		CanonicalName:   input.CanonicalName,
		DisplayName:     input.DisplayName,
		Family:          input.Family,
		ContextWindow:   input.ContextWindow,
		MaxOutputTokens: input.MaxOutputTokens,
		QualityTier:     input.QualityTier,
		Status:          input.Status,
		Capabilities:    input.Capabilities,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.byID[model.ID] = model
	s.byCanonicalName[strings.ToLower(model.CanonicalName)] = model.ID
	s.nextID++
	return model, nil
}

func (s *memoryStore) Update(_ context.Context, model contract.Model) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[model.ID]; !ok {
		return contract.Model{}, errors.New("model not found")
	}
	s.byID[model.ID] = model
	return model, nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	model, ok := s.byID[id]
	if !ok {
		return contract.Model{}, errors.New("model not found")
	}
	return model, nil
}

func (s *memoryStore) FindByCanonicalName(_ context.Context, canonicalName string) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byCanonicalName[strings.ToLower(strings.TrimSpace(canonicalName))]
	if !ok {
		return contract.Model{}, errors.New("model not found")
	}
	return s.byID[id], nil
}

func (s *memoryStore) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	model, ok := s.byID[id]
	if !ok {
		return errors.New("model not found")
	}
	for _, aliasID := range s.aliasIDsByModel[id] {
		if alias, ok := s.aliasesByID[aliasID]; ok {
			delete(s.aliasByName, strings.ToLower(strings.TrimSpace(alias.Alias)))
			delete(s.aliasesByID, aliasID)
		}
	}
	delete(s.aliasIDsByModel, id)
	for _, mappingID := range s.mappingIDsByModel[id] {
		if mapping, ok := s.mappingsByID[mappingID]; ok {
			delete(s.mappingByKey, mappingKey(mapping.ModelID, mapping.ProviderID, mapping.UpstreamModelName))
			delete(s.mappingsByID, mappingID)
		}
	}
	delete(s.mappingIDsByModel, id)
	delete(s.byCanonicalName, strings.ToLower(strings.TrimSpace(model.CanonicalName)))
	delete(s.byID, id)
	return nil
}

func (s *memoryStore) FindByAlias(_ context.Context, alias string) (contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.aliasByName[strings.ToLower(strings.TrimSpace(alias))]
	if !ok {
		return contract.ModelAlias{}, errors.New("model alias not found")
	}
	return s.aliasesByID[id], nil
}

func (s *memoryStore) CreateAlias(_ context.Context, input contract.CreateStoredAlias) (contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[input.ModelID]; !ok {
		return contract.ModelAlias{}, errors.New("model not found")
	}
	key := strings.ToLower(strings.TrimSpace(input.Alias))
	if _, ok := s.aliasByName[key]; ok {
		return contract.ModelAlias{}, errors.New("model alias already exists")
	}
	now := time.Now().UTC()
	alias := contract.ModelAlias{
		ID:             s.nextAliasID,
		Alias:          strings.TrimSpace(input.Alias),
		ModelID:        input.ModelID,
		StrategyHint:   input.StrategyHint,
		FallbackModels: input.FallbackModels,
		Status:         input.Status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.aliasesByID[alias.ID] = alias
	s.aliasByName[key] = alias.ID
	s.aliasIDsByModel[alias.ModelID] = append(s.aliasIDsByModel[alias.ModelID], alias.ID)
	s.nextAliasID++
	return alias, nil
}

func (s *memoryStore) ListAliasesByModel(_ context.Context, modelID int) ([]contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.aliasIDsByModel[modelID]
	out := make([]contract.ModelAlias, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.aliasesByID[id])
	}
	return out, nil
}

func (s *memoryStore) FindAliasByID(_ context.Context, id int) (contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	alias, ok := s.aliasesByID[id]
	if !ok {
		return contract.ModelAlias{}, errors.New("model alias not found")
	}
	return alias, nil
}

func (s *memoryStore) DeleteAlias(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	alias, ok := s.aliasesByID[id]
	if !ok {
		return errors.New("model alias not found")
	}
	delete(s.aliasByName, strings.ToLower(strings.TrimSpace(alias.Alias)))
	delete(s.aliasesByID, id)
	remaining := make([]int, 0, len(s.aliasIDsByModel[alias.ModelID]))
	for _, existing := range s.aliasIDsByModel[alias.ModelID] {
		if existing != id {
			remaining = append(remaining, existing)
		}
	}
	s.aliasIDsByModel[alias.ModelID] = remaining
	return nil
}

func (s *memoryStore) FindMapping(_ context.Context, modelID int, providerID int, upstreamModelName string) (contract.ModelProviderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.mappingByKey[mappingKey(modelID, providerID, upstreamModelName)]
	if !ok {
		return contract.ModelProviderMapping{}, errors.New("model provider mapping not found")
	}
	return s.mappingsByID[id], nil
}

func (s *memoryStore) CreateMapping(_ context.Context, input contract.CreateStoredMapping) (contract.ModelProviderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[input.ModelID]; !ok {
		return contract.ModelProviderMapping{}, errors.New("model not found")
	}
	key := mappingKey(input.ModelID, input.ProviderID, input.UpstreamModelName)
	if _, ok := s.mappingByKey[key]; ok {
		return contract.ModelProviderMapping{}, errors.New("model provider mapping already exists")
	}
	now := time.Now().UTC()
	mapping := contract.ModelProviderMapping{
		ID:                 s.nextMappingID,
		ModelID:            input.ModelID,
		ProviderID:         input.ProviderID,
		UpstreamModelName:  strings.TrimSpace(input.UpstreamModelName),
		Status:             input.Status,
		CapabilityOverride: input.CapabilityOverride,
		PricingOverride:    input.PricingOverride,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.mappingsByID[mapping.ID] = mapping
	s.mappingByKey[key] = mapping.ID
	s.mappingIDsByModel[mapping.ModelID] = append(s.mappingIDsByModel[mapping.ModelID], mapping.ID)
	s.nextMappingID++
	return mapping, nil
}

func (s *memoryStore) ListMappingsByModel(_ context.Context, modelID int) ([]contract.ModelProviderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.mappingIDsByModel[modelID]
	out := make([]contract.ModelProviderMapping, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.mappingsByID[id])
	}
	return out, nil
}

func (s *memoryStore) FindMappingByID(_ context.Context, id int) (contract.ModelProviderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapping, ok := s.mappingsByID[id]
	if !ok {
		return contract.ModelProviderMapping{}, errors.New("model provider mapping not found")
	}
	return mapping, nil
}

func (s *memoryStore) DeleteMapping(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapping, ok := s.mappingsByID[id]
	if !ok {
		return errors.New("model provider mapping not found")
	}
	delete(s.mappingByKey, mappingKey(mapping.ModelID, mapping.ProviderID, mapping.UpstreamModelName))
	delete(s.mappingsByID, id)
	remaining := make([]int, 0, len(s.mappingIDsByModel[mapping.ModelID]))
	for _, existing := range s.mappingIDsByModel[mapping.ModelID] {
		if existing != id {
			remaining = append(remaining, existing)
		}
	}
	s.mappingIDsByModel[mapping.ModelID] = remaining
	return nil
}

func (s *memoryStore) List(_ context.Context) ([]contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Model, 0, len(s.byID))
	for _, model := range s.byID {
		out = append(out, model)
	}
	return out, nil
}

func mappingKey(modelID int, providerID int, upstreamModelName string) string {
	return fmt.Sprintf("%d:%d:%s", modelID, providerID, strings.TrimSpace(upstreamModelName))
}
