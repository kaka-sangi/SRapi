package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func (s *Service) CreateStrategy(ctx context.Context, input contract.StrategyMutation) (contract.StrategyDescriptor, error) {
	descriptor, err := strategyDescriptorFromMutation(input, nil)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	if descriptor.Status == "" {
		descriptor.Status = contract.StrategyStatusActive
	}
	if descriptor.Status == contract.StrategyStatusActive {
		now := s.clock.Now()
		descriptor.ActivatedAt = &now
	}
	created, err := s.store.CreateStrategy(ctx, descriptor)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	s.invalidateStrategyCache()
	if err := s.RefreshStrategies(ctx); err != nil {
		return contract.StrategyDescriptor{}, err
	}
	return created, nil
}

func (s *Service) UpdateStrategy(ctx context.Context, id int, input contract.StrategyMutation) (contract.StrategyDescriptor, error) {
	if id <= 0 {
		return contract.StrategyDescriptor{}, ErrInvalidInput
	}
	current, err := s.store.GetStrategy(ctx, id)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	descriptor, err := strategyDescriptorFromMutation(input, &current)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	updated, err := s.store.UpdateStrategy(ctx, id, descriptor)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	s.invalidateStrategyCache()
	if err := s.RefreshStrategies(ctx); err != nil {
		return contract.StrategyDescriptor{}, err
	}
	return updated, nil
}

func (s *Service) ActivateStrategy(ctx context.Context, id int) (contract.StrategyDescriptor, error) {
	if id <= 0 {
		return contract.StrategyDescriptor{}, ErrInvalidInput
	}
	now := s.clock.Now()
	updated, err := s.store.UpdateStrategy(ctx, id, contract.StrategyDescriptor{
		Status:       contract.StrategyStatusActive,
		ActivatedAt:  &now,
		DeprecatedAt: nil,
	})
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	s.invalidateStrategyCache()
	if err := s.RefreshStrategies(ctx); err != nil {
		return contract.StrategyDescriptor{}, err
	}
	return updated, nil
}

func (s *Service) DeprecateStrategy(ctx context.Context, id int) (contract.StrategyDescriptor, error) {
	if id <= 0 {
		return contract.StrategyDescriptor{}, ErrInvalidInput
	}
	now := s.clock.Now()
	updated, err := s.store.UpdateStrategy(ctx, id, contract.StrategyDescriptor{
		Status:       contract.StrategyStatusDeprecated,
		DeprecatedAt: &now,
	})
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	s.invalidateStrategyCache()
	if err := s.RefreshStrategies(ctx); err != nil {
		return contract.StrategyDescriptor{}, err
	}
	return updated, nil
}

func strategyDescriptorFromMutation(input contract.StrategyMutation, current *contract.StrategyDescriptor) (contract.StrategyDescriptor, error) {
	descriptor := contract.StrategyDescriptor{}
	if current != nil {
		descriptor = cloneStrategyDescriptor(*current)
	}
	if input.Name != "" {
		descriptor.Name = input.Name
	}
	if input.Version != "" {
		descriptor.Version = strings.TrimSpace(input.Version)
	}
	if input.Status != "" {
		descriptor.Status = input.Status
	}
	if input.ScopeType != "" {
		descriptor.ScopeType = input.ScopeType
		descriptor.ScopeID = cloneIntPtr(input.ScopeID)
	}
	if input.Config != nil {
		descriptor.Config = cloneMapAny(input.Config)
	}
	if input.Weights != nil {
		descriptor.Weights = cloneWeights(input.Weights)
	}
	descriptor.Description = strings.TrimSpace(input.Description)
	if input.CreatedBy != nil {
		descriptor.CreatedBy = cloneIntPtr(input.CreatedBy)
	}
	if descriptor.Version == "" {
		descriptor.Version = "v1"
	}
	if descriptor.Status == "" {
		descriptor.Status = contract.StrategyStatusActive
	}
	if err := validateStrategyStatus(descriptor.Status); err != nil {
		return contract.StrategyDescriptor{}, err
	}
	normalized, err := normalizeMutableStrategyDescriptor(descriptor)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	return normalized, nil
}

func validateStrategyStatus(status contract.StrategyStatus) error {
	switch status {
	case contract.StrategyStatusDraft, contract.StrategyStatusActive, contract.StrategyStatusDeprecated:
		return nil
	default:
		return fmt.Errorf("%w: invalid strategy status %q", ErrInvalidInput, status)
	}
}

func normalizeMutableStrategyDescriptor(descriptor contract.StrategyDescriptor) (contract.StrategyDescriptor, error) {
	name := contract.StrategyName(strings.TrimSpace(string(descriptor.Name)))
	if !knownStrategyName(name) {
		return contract.StrategyDescriptor{}, fmt.Errorf("%w: unknown strategy %q", ErrInvalidInput, descriptor.Name)
	}
	scope, scopeID, err := normalizeStrategyScope(descriptor.ScopeType, descriptor.ScopeID)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	config := cloneMapAny(descriptor.Config)
	if config == nil {
		config = map[string]any{}
	}
	weights, err := normalizedStrategyWeights(descriptor, config)
	if err != nil {
		return contract.StrategyDescriptor{}, err
	}
	config["weights"] = weightsPayload(weights)
	return contract.StrategyDescriptor{
		ID:           descriptor.ID,
		Name:         name,
		Version:      strings.TrimSpace(descriptor.Version),
		Status:       descriptor.Status,
		ScopeType:    scope,
		ScopeID:      scopeID,
		ConfigHash:   configHash(config),
		Config:       config,
		Weights:      weights,
		Description:  strings.TrimSpace(descriptor.Description),
		CreatedBy:    cloneIntPtr(descriptor.CreatedBy),
		CreatedAt:    descriptor.CreatedAt,
		ActivatedAt:  cloneTime(descriptor.ActivatedAt),
		DeprecatedAt: cloneTime(descriptor.DeprecatedAt),
	}, nil
}
