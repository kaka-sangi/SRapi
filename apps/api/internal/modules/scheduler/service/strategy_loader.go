package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func (s *Service) RefreshStrategies(ctx context.Context) error {
	descriptors, err := s.store.ListActiveStrategies(ctx)
	if err != nil {
		return err
	}
	return s.registry.ReplaceActive(descriptors)
}

func (r *StrategyRegistry) ReplaceActive(descriptors []contract.StrategyDescriptor) error {
	next := seededStrategyDescriptorMap()
	for _, descriptor := range descriptors {
		normalized, err := normalizeLoadedStrategyDescriptor(descriptor)
		if err != nil {
			return err
		}
		next[normalized.Name] = normalized
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.descriptors = next
	return nil
}

func normalizeLoadedStrategyDescriptor(descriptor contract.StrategyDescriptor) (contract.StrategyDescriptor, error) {
	name := contract.StrategyName(strings.TrimSpace(string(descriptor.Name)))
	if !knownStrategyName(name) {
		return contract.StrategyDescriptor{}, fmt.Errorf("%w: unknown strategy %q", ErrInvalidInput, descriptor.Name)
	}
	status := strings.TrimSpace(descriptor.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" {
		return contract.StrategyDescriptor{}, fmt.Errorf("%w: strategy %s is not active", ErrInvalidInput, name)
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
	version := strings.TrimSpace(descriptor.Version)
	if version == "" {
		version = "v1"
	}
	return contract.StrategyDescriptor{
		ID:          descriptor.ID,
		Name:        name,
		Version:     version,
		Status:      "active",
		Config:      config,
		ConfigHash:  configHash(config),
		Weights:     weights,
		Description: strings.TrimSpace(descriptor.Description),
	}, nil
}

func normalizedStrategyWeights(descriptor contract.StrategyDescriptor, config map[string]any) (map[string]float64, error) {
	weights := cloneWeights(descriptor.Weights)
	if len(weights) == 0 {
		loaded, err := weightsFromConfig(config)
		if err != nil {
			return nil, err
		}
		weights = loaded
	}
	out := make(map[string]float64, len(weights))
	total := 0.0
	for key, value := range weights {
		normalizedKey, ok := normalizeStrategyWeightKey(key)
		if !ok {
			return nil, fmt.Errorf("%w: unknown strategy weight %q", ErrInvalidInput, key)
		}
		if value < 0 || value > 1 {
			return nil, fmt.Errorf("%w: strategy weight %q out of range", ErrInvalidInput, key)
		}
		out[normalizedKey] = value
		total += value
	}
	if total <= 0 {
		return nil, fmt.Errorf("%w: strategy weights must contain a positive value", ErrInvalidInput)
	}
	return out, nil
}

func weightsFromConfig(config map[string]any) (map[string]float64, error) {
	raw, ok := config["weights"]
	if !ok {
		return nil, fmt.Errorf("%w: strategy config missing weights", ErrInvalidInput)
	}
	rawWeights, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: strategy weights must be an object", ErrInvalidInput)
	}
	weights := make(map[string]float64, len(rawWeights))
	for key, value := range rawWeights {
		parsed, ok := floatValue(value)
		if !ok {
			return nil, fmt.Errorf("%w: strategy weight %q must be numeric", ErrInvalidInput, key)
		}
		weights[key] = parsed
	}
	return weights, nil
}

func knownStrategyName(name contract.StrategyName) bool {
	switch name {
	case contract.StrategyBalanced,
		contract.StrategyCostSaver,
		contract.StrategyLatencyFirst,
		contract.StrategyQuotaProtect,
		contract.StrategyStickyFirst,
		contract.StrategyCacheAffinityFirst,
		contract.StrategyPremiumQuality:
		return true
	default:
		return false
	}
}

func normalizeStrategyWeightKey(key string) (string, bool) {
	switch strings.TrimSpace(key) {
	case "health", "health_weight":
		return "health", true
	case "quota", "quota_weight":
		return "quota", true
	case "latency", "latency_weight":
		return "latency", true
	case "sticky", "sticky_weight":
		return "sticky", true
	case "cache", "cache_weight":
		return "cache", true
	case "cost", "cost_weight":
		return "cost", true
	case "fairness", "fairness_weight":
		return "fairness", true
	case "priority", "priority_weight", "quality", "quality_weight", "premium_quality_weight", "quality_preference_weight":
		return "priority", true
	default:
		return "", false
	}
}
