package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
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

func (s *Service) Create(ctx context.Context, req contract.CreateRequest) (contract.Provider, error) {
	name := strings.TrimSpace(req.Name)
	displayName := strings.TrimSpace(req.DisplayName)
	adapterType := strings.TrimSpace(req.AdapterType)
	protocol := strings.TrimSpace(req.Protocol)
	if name == "" || displayName == "" || adapterType == "" || protocol == "" {
		return contract.Provider{}, ErrInvalidInput
	}
	if _, err := s.store.FindByName(ctx, name); err == nil {
		return contract.Provider{}, ErrProviderExists
	}

	status := contract.StatusActive
	if req.Status != nil {
		status = *req.Status
	}
	capabilities, err := normalizeCapabilityMap(req.Capabilities)
	if err != nil {
		return contract.Provider{}, ErrInvalidInput
	}

	stored, err := s.store.Create(ctx, contract.CreateStoredProvider{
		Name:         name,
		DisplayName:  displayName,
		AdapterType:  adapterType,
		Protocol:     protocol,
		Status:       status,
		Capabilities: capabilities,
		ConfigSchema: cloneMap(req.ConfigSchema),
	})
	if err != nil {
		return contract.Provider{}, err
	}
	return stored, nil
}

func (s *Service) List(ctx context.Context) ([]contract.Provider, error) {
	providers, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Provider, 0, len(providers))
	for _, provider := range providers {
		out = append(out, provider)
	}
	return out, nil
}

func (s *Service) Update(ctx context.Context, id int, req contract.UpdateRequest) (contract.Provider, error) {
	if id <= 0 {
		return contract.Provider{}, ErrInvalidInput
	}
	provider, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.Provider{}, err
	}
	if req.DisplayName != nil {
		displayName := strings.TrimSpace(*req.DisplayName)
		if displayName == "" {
			return contract.Provider{}, ErrInvalidInput
		}
		provider.DisplayName = displayName
	}
	if req.AdapterType != nil {
		adapterType := strings.TrimSpace(*req.AdapterType)
		if adapterType == "" {
			return contract.Provider{}, ErrInvalidInput
		}
		provider.AdapterType = adapterType
	}
	if req.Protocol != nil {
		protocol := strings.TrimSpace(*req.Protocol)
		if protocol == "" {
			return contract.Provider{}, ErrInvalidInput
		}
		provider.Protocol = protocol
	}
	if req.Status != nil {
		provider.Status = *req.Status
	}
	if req.Capabilities != nil {
		capabilities, err := normalizeCapabilityMap(*req.Capabilities)
		if err != nil {
			return contract.Provider{}, ErrInvalidInput
		}
		provider.Capabilities = capabilities
	}
	if req.ConfigSchema != nil {
		provider.ConfigSchema = cloneMap(*req.ConfigSchema)
	}
	provider.UpdatedAt = s.clock.Now()
	return s.store.Update(ctx, provider)
}

func (s *Service) FindByID(ctx context.Context, id int) (contract.Provider, error) {
	if id <= 0 {
		return contract.Provider{}, ErrInvalidInput
	}
	return s.store.FindByID(ctx, id)
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

func normalizeCapabilityMap(values map[string]any) (map[string]any, error) {
	if values == nil {
		return nil, nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		canonical, ok := capabilitiescontract.CanonicalKeyFromConvenience(key)
		if !ok {
			return nil, fmt.Errorf("unknown provider capability key %q", key)
		}
		out[canonical] = value
	}
	return cloneMap(out), nil
}
