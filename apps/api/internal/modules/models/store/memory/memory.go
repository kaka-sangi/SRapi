package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/models/contract"
)

type Store struct {
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

func New() *Store {
	return &Store{
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

func (s *Store) Create(_ context.Context, input contract.CreateStoredModel) (contract.Model, error) {
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
		Capabilities:    cloneDescriptors(input.Capabilities),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.byID[model.ID] = model
	s.byCanonicalName[strings.ToLower(model.CanonicalName)] = model.ID
	s.nextID++
	return model, nil
}

func (s *Store) Update(_ context.Context, model contract.Model) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[model.ID]; !ok {
		return contract.Model{}, errors.New("model not found")
	}
	stored := model
	stored.Capabilities = cloneDescriptors(model.Capabilities)
	s.byID[stored.ID] = stored
	return cloneModel(stored), nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	model, ok := s.byID[id]
	if !ok {
		return contract.Model{}, errors.New("model not found")
	}
	return cloneModel(model), nil
}

func (s *Store) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	model, ok := s.byID[id]
	if !ok {
		return errors.New("model not found")
	}
	for _, aliasID := range s.aliasIDsByModel[id] {
		if alias, ok := s.aliasesByID[aliasID]; ok {
			delete(s.aliasByName, aliasKey(alias.Alias))
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

func (s *Store) FindByCanonicalName(_ context.Context, canonicalName string) (contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byCanonicalName[strings.ToLower(strings.TrimSpace(canonicalName))]
	if !ok {
		return contract.Model{}, errors.New("model not found")
	}
	return cloneModel(s.byID[id]), nil
}

func (s *Store) FindByAlias(_ context.Context, alias string) (contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.aliasByName[aliasKey(alias)]
	if !ok {
		return contract.ModelAlias{}, errors.New("model alias not found")
	}
	return cloneAlias(s.aliasesByID[id]), nil
}

func (s *Store) CreateAlias(_ context.Context, input contract.CreateStoredAlias) (contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[input.ModelID]; !ok {
		return contract.ModelAlias{}, errors.New("model not found")
	}
	key := aliasKey(input.Alias)
	if _, ok := s.aliasByName[key]; ok {
		return contract.ModelAlias{}, errors.New("model alias already exists")
	}
	now := time.Now().UTC()
	alias := contract.ModelAlias{
		ID:             s.nextAliasID,
		Alias:          strings.TrimSpace(input.Alias),
		ModelID:        input.ModelID,
		StrategyHint:   input.StrategyHint,
		FallbackModels: cloneStrings(input.FallbackModels),
		Status:         input.Status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.aliasesByID[alias.ID] = alias
	s.aliasByName[key] = alias.ID
	s.aliasIDsByModel[alias.ModelID] = append(s.aliasIDsByModel[alias.ModelID], alias.ID)
	s.nextAliasID++
	return cloneAlias(alias), nil
}

func (s *Store) ListAliasesByModel(_ context.Context, modelID int) ([]contract.ModelAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.aliasIDsByModel[modelID]
	out := make([]contract.ModelAlias, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneAlias(s.aliasesByID[id]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) FindMapping(_ context.Context, modelID int, providerID int, upstreamModelName string) (contract.ModelProviderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.mappingByKey[mappingKey(modelID, providerID, upstreamModelName)]
	if !ok {
		return contract.ModelProviderMapping{}, errors.New("model provider mapping not found")
	}
	return cloneMapping(s.mappingsByID[id]), nil
}

func (s *Store) CreateMapping(_ context.Context, input contract.CreateStoredMapping) (contract.ModelProviderMapping, error) {
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
		CapabilityOverride: cloneDescriptors(input.CapabilityOverride),
		PricingOverride:    cloneMap(input.PricingOverride),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.mappingsByID[mapping.ID] = mapping
	s.mappingByKey[key] = mapping.ID
	s.mappingIDsByModel[mapping.ModelID] = append(s.mappingIDsByModel[mapping.ModelID], mapping.ID)
	s.nextMappingID++
	return cloneMapping(mapping), nil
}

func (s *Store) ListMappingsByModel(_ context.Context, modelID int) ([]contract.ModelProviderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.mappingIDsByModel[modelID]
	out := make([]contract.ModelProviderMapping, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneMapping(s.mappingsByID[id]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) List(_ context.Context) ([]contract.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Model, 0, len(s.byID))
	for _, model := range s.byID {
		out = append(out, cloneModel(model))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func aliasKey(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func mappingKey(modelID int, providerID int, upstreamModelName string) string {
	return fmt.Sprintf("%d:%d:%s", modelID, providerID, strings.TrimSpace(upstreamModelName))
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	cloned := make([]capabilitiescontract.Descriptor, len(values))
	copy(cloned, values)
	return cloned
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneAlias(value contract.ModelAlias) contract.ModelAlias {
	value.FallbackModels = cloneStrings(value.FallbackModels)
	return value
}

func cloneMapping(value contract.ModelProviderMapping) contract.ModelProviderMapping {
	value.CapabilityOverride = cloneDescriptors(value.CapabilityOverride)
	value.PricingOverride = cloneMap(value.PricingOverride)
	return value
}

func cloneModel(value contract.Model) contract.Model {
	value.Capabilities = cloneDescriptors(value.Capabilities)
	return value
}
