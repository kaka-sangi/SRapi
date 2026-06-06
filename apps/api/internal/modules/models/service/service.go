package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/models/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store contract.Store
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, req contract.CreateRequest) (contract.Model, error) {
	canonicalName := strings.TrimSpace(req.CanonicalName)
	displayName := strings.TrimSpace(req.DisplayName)
	if canonicalName == "" || displayName == "" {
		return contract.Model{}, ErrInvalidInput
	}
	if _, err := s.store.FindByCanonicalName(ctx, canonicalName); err == nil {
		return contract.Model{}, ErrModelExists
	}
	status := contract.StatusActive
	if req.Status != nil {
		status = *req.Status
	}
	capabilities, err := normalizeDescriptors(req.Capabilities)
	if err != nil {
		return contract.Model{}, ErrInvalidInput
	}
	stored, err := s.store.Create(ctx, contract.CreateStoredModel{
		CanonicalName:   canonicalName,
		DisplayName:     displayName,
		Family:          cloneString(req.Family),
		ContextWindow:   cloneInt(req.ContextWindow),
		MaxOutputTokens: cloneInt(req.MaxOutputTokens),
		QualityTier:     cloneString(req.QualityTier),
		Status:          status,
		Capabilities:    capabilities,
	})
	if err != nil {
		return contract.Model{}, err
	}
	return stored, nil
}

func (s *Service) List(ctx context.Context) ([]contract.Model, error) {
	models, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Model, 0, len(models))
	for _, model := range models {
		out = append(out, model)
	}
	return out, nil
}

func (s *Service) FindByID(ctx context.Context, id int) (contract.Model, error) {
	if id <= 0 {
		return contract.Model{}, ErrInvalidInput
	}
	return s.store.FindByID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, id); err != nil {
		return ErrModelNotFound
	}
	return s.store.Delete(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int, req contract.UpdateRequest) (contract.Model, error) {
	if id <= 0 {
		return contract.Model{}, ErrInvalidInput
	}
	model, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.Model{}, ErrModelNotFound
	}
	if req.DisplayName != nil {
		displayName := strings.TrimSpace(*req.DisplayName)
		if displayName == "" {
			return contract.Model{}, ErrInvalidInput
		}
		model.DisplayName = displayName
	}
	if req.Family != nil {
		model.Family = cloneString(*req.Family)
	}
	if req.ContextWindow != nil {
		if *req.ContextWindow != nil && **req.ContextWindow <= 0 {
			return contract.Model{}, ErrInvalidInput
		}
		model.ContextWindow = cloneInt(*req.ContextWindow)
	}
	if req.MaxOutputTokens != nil {
		if *req.MaxOutputTokens != nil && **req.MaxOutputTokens <= 0 {
			return contract.Model{}, ErrInvalidInput
		}
		model.MaxOutputTokens = cloneInt(*req.MaxOutputTokens)
	}
	if req.QualityTier != nil {
		model.QualityTier = cloneString(*req.QualityTier)
	}
	if req.Status != nil {
		model.Status = *req.Status
	}
	if req.Capabilities != nil {
		capabilities, err := normalizeDescriptors(*req.Capabilities)
		if err != nil {
			return contract.Model{}, ErrInvalidInput
		}
		model.Capabilities = capabilities
	}
	model.UpdatedAt = s.clock.Now()
	return s.store.Update(ctx, model)
}

func (s *Service) FindByCanonicalName(ctx context.Context, canonicalName string) (contract.Model, error) {
	canonicalName = strings.TrimSpace(canonicalName)
	if canonicalName == "" {
		return contract.Model{}, ErrInvalidInput
	}
	return s.store.FindByCanonicalName(ctx, canonicalName)
}

func (s *Service) ResolveModel(ctx context.Context, name string) (contract.Model, error) {
	resolution, err := s.ResolveModelReference(ctx, name)
	if err != nil {
		return contract.Model{}, err
	}
	return resolution.Model, nil
}

func (s *Service) ResolveModelReference(ctx context.Context, name string) (contract.ModelResolution, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return contract.ModelResolution{}, ErrInvalidInput
	}
	if model, err := s.store.FindByCanonicalName(ctx, name); err == nil {
		return contract.ModelResolution{Model: model}, nil
	}
	alias, err := s.store.FindByAlias(ctx, name)
	if err != nil {
		return contract.ModelResolution{}, ErrModelNotFound
	}
	if alias.Status != contract.StatusActive {
		return contract.ModelResolution{}, ErrModelNotFound
	}
	model, err := s.store.FindByID(ctx, alias.ModelID)
	if err != nil {
		return contract.ModelResolution{}, err
	}
	return contract.ModelResolution{Model: model, Alias: &alias}, nil
}

func (s *Service) CreateAlias(ctx context.Context, modelID int, req contract.CreateAliasRequest) (contract.ModelAlias, error) {
	alias := strings.TrimSpace(req.Alias)
	if modelID <= 0 || alias == "" {
		return contract.ModelAlias{}, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, modelID); err != nil {
		return contract.ModelAlias{}, ErrModelNotFound
	}
	if _, err := s.store.FindByAlias(ctx, alias); err == nil {
		return contract.ModelAlias{}, ErrAliasExists
	}
	status := contract.StatusActive
	if req.Status != nil {
		status = *req.Status
	}
	stored, err := s.store.CreateAlias(ctx, contract.CreateStoredAlias{
		Alias:          alias,
		ModelID:        modelID,
		StrategyHint:   cloneString(req.StrategyHint),
		FallbackModels: cloneStrings(req.FallbackModels),
		Status:         status,
	})
	if err != nil {
		return contract.ModelAlias{}, err
	}
	return stored, nil
}

func (s *Service) ListAliasesByModel(ctx context.Context, modelID int) ([]contract.ModelAlias, error) {
	if modelID <= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, modelID); err != nil {
		return nil, ErrModelNotFound
	}
	return s.store.ListAliasesByModel(ctx, modelID)
}

func (s *Service) CreateMapping(ctx context.Context, modelID int, req contract.CreateMappingRequest) (contract.ModelProviderMapping, error) {
	upstreamModelName := strings.TrimSpace(req.UpstreamModelName)
	if modelID <= 0 || req.ProviderID <= 0 || upstreamModelName == "" {
		return contract.ModelProviderMapping{}, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, modelID); err != nil {
		return contract.ModelProviderMapping{}, ErrModelNotFound
	}
	if _, err := s.store.FindMapping(ctx, modelID, req.ProviderID, upstreamModelName); err == nil {
		return contract.ModelProviderMapping{}, ErrMappingExists
	}
	status := contract.StatusActive
	if req.Status != nil {
		status = *req.Status
	}
	capabilityOverride, err := normalizeDescriptors(req.CapabilityOverride)
	if err != nil {
		return contract.ModelProviderMapping{}, ErrInvalidInput
	}
	stored, err := s.store.CreateMapping(ctx, contract.CreateStoredMapping{
		ModelID:            modelID,
		ProviderID:         req.ProviderID,
		UpstreamModelName:  upstreamModelName,
		Status:             status,
		CapabilityOverride: capabilityOverride,
		PricingOverride:    cloneMap(req.PricingOverride),
	})
	if err != nil {
		return contract.ModelProviderMapping{}, err
	}
	return stored, nil
}

func (s *Service) ListMappingsByModel(ctx context.Context, modelID int) ([]contract.ModelProviderMapping, error) {
	if modelID <= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, modelID); err != nil {
		return nil, ErrModelNotFound
	}
	return s.store.ListMappingsByModel(ctx, modelID)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	cloned := make([]capabilitiescontract.Descriptor, len(values))
	copy(cloned, values)
	return cloned
}

func normalizeDescriptors(values []capabilitiescontract.Descriptor) ([]capabilitiescontract.Descriptor, error) {
	normalized, err := capabilitiescontract.NormalizeDescriptors(values)
	if err != nil {
		return nil, err
	}
	return cloneDescriptors(normalized), nil
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
