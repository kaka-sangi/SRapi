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
		// H7: Disabled or archived models must not be routable.
		if model.Status != contract.StatusActive {
			return contract.ModelResolution{}, ErrModelDisabled
		}
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
	// H7: Disabled or archived models must not be routable even via an active alias.
	if model.Status != contract.StatusActive {
		return contract.ModelResolution{}, ErrModelDisabled
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

func (s *Service) FindAliasByID(ctx context.Context, aliasID int) (contract.ModelAlias, error) {
	if aliasID <= 0 {
		return contract.ModelAlias{}, ErrInvalidInput
	}
	alias, err := s.store.FindAliasByID(ctx, aliasID)
	if err != nil {
		return contract.ModelAlias{}, ErrAliasNotFound
	}
	return alias, nil
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

func (s *Service) DeleteAlias(ctx context.Context, modelID int, aliasID int) error {
	if modelID <= 0 || aliasID <= 0 {
		return ErrInvalidInput
	}
	alias, err := s.store.FindAliasByID(ctx, aliasID)
	if err != nil || alias.ModelID != modelID {
		return ErrAliasNotFound
	}
	return s.store.DeleteAlias(ctx, aliasID)
}

func (s *Service) UpdateAlias(ctx context.Context, modelID int, aliasID int, req contract.UpdateAliasRequest) (contract.ModelAlias, error) {
	if modelID <= 0 || aliasID <= 0 {
		return contract.ModelAlias{}, ErrInvalidInput
	}
	existing, err := s.store.FindAliasByID(ctx, aliasID)
	if err != nil || existing.ModelID != modelID {
		return contract.ModelAlias{}, ErrAliasNotFound
	}
	// Start from current values, apply only the fields provided.
	aliasStr := existing.Alias
	if req.Alias != nil {
		aliasStr = strings.TrimSpace(*req.Alias)
		if aliasStr == "" {
			return contract.ModelAlias{}, ErrInvalidInput
		}
		// Uniqueness check: only needed when the alias string actually changes.
		if !strings.EqualFold(aliasStr, existing.Alias) {
			if _, err := s.store.FindByAlias(ctx, aliasStr); err == nil {
				return contract.ModelAlias{}, ErrAliasExists
			}
		}
	}
	strategyHint := existing.StrategyHint
	if req.StrategyHint != nil {
		strategyHint = cloneString(*req.StrategyHint)
	}
	fallbackModels := existing.FallbackModels
	if req.FallbackModels != nil {
		fallbackModels = cloneStrings(*req.FallbackModels)
	}
	status := existing.Status
	if req.Status != nil {
		status = *req.Status
	}
	return s.store.UpdateAlias(ctx, aliasID, contract.UpdateStoredAlias{
		Alias:          aliasStr,
		StrategyHint:   strategyHint,
		FallbackModels: fallbackModels,
		Status:         status,
	})
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

func (s *Service) FindMappingByID(ctx context.Context, mappingID int) (contract.ModelProviderMapping, error) {
	if mappingID <= 0 {
		return contract.ModelProviderMapping{}, ErrInvalidInput
	}
	mapping, err := s.store.FindMappingByID(ctx, mappingID)
	if err != nil {
		return contract.ModelProviderMapping{}, ErrMappingNotFound
	}
	return mapping, nil
}

func (s *Service) ListMappings(ctx context.Context) ([]contract.ModelProviderMapping, error) {
	mappings, err := s.store.ListMappings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ModelProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		out = append(out, mapping)
	}
	return out, nil
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

func (s *Service) DeleteMapping(ctx context.Context, modelID int, mappingID int) error {
	if modelID <= 0 || mappingID <= 0 {
		return ErrInvalidInput
	}
	mapping, err := s.store.FindMappingByID(ctx, mappingID)
	if err != nil || mapping.ModelID != modelID {
		return ErrMappingNotFound
	}
	return s.store.DeleteMapping(ctx, mappingID)
}

func (s *Service) UpdateMapping(ctx context.Context, modelID int, mappingID int, req contract.UpdateMappingRequest) (contract.ModelProviderMapping, error) {
	if modelID <= 0 || mappingID <= 0 {
		return contract.ModelProviderMapping{}, ErrInvalidInput
	}
	existing, err := s.store.FindMappingByID(ctx, mappingID)
	if err != nil || existing.ModelID != modelID {
		return contract.ModelProviderMapping{}, ErrMappingNotFound
	}
	// Start from current values, apply only the fields provided.
	upstreamModelName := existing.UpstreamModelName
	if req.UpstreamModelName != nil {
		upstreamModelName = strings.TrimSpace(*req.UpstreamModelName)
		if upstreamModelName == "" {
			return contract.ModelProviderMapping{}, ErrInvalidInput
		}
		// Uniqueness check: only needed when upstream_model_name actually changes.
		if upstreamModelName != existing.UpstreamModelName {
			if _, err := s.store.FindMapping(ctx, modelID, existing.ProviderID, upstreamModelName); err == nil {
				return contract.ModelProviderMapping{}, ErrMappingExists
			}
		}
	}
	status := existing.Status
	if req.Status != nil {
		status = *req.Status
	}
	capabilityOverride := existing.CapabilityOverride
	if req.CapabilityOverride != nil {
		normalized, err := normalizeDescriptors(*req.CapabilityOverride)
		if err != nil {
			return contract.ModelProviderMapping{}, ErrInvalidInput
		}
		capabilityOverride = normalized
	}
	pricingOverride := existing.PricingOverride
	if req.PricingOverride != nil {
		pricingOverride = cloneMap(*req.PricingOverride)
	}
	return s.store.UpdateMapping(ctx, mappingID, contract.UpdateStoredMapping{
		UpstreamModelName:  upstreamModelName,
		Status:             status,
		CapabilityOverride: capabilityOverride,
		PricingOverride:    pricingOverride,
	})
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
