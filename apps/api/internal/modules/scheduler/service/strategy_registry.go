package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

type StrategyRegistry struct {
	mu          sync.RWMutex
	descriptors map[strategyRegistryKey]contract.StrategyDescriptor
}

type strategyScopeKey struct {
	Type contract.StrategyScopeType
	ID   int
}

type strategyRegistryKey struct {
	Scope strategyScopeKey
	Name  contract.StrategyName
}

func NewStrategyRegistry() *StrategyRegistry {
	return &StrategyRegistry{descriptors: seededStrategyDescriptorMap()}
}

func seededStrategyDescriptorMap() map[strategyRegistryKey]contract.StrategyDescriptor {
	descriptors := map[strategyRegistryKey]contract.StrategyDescriptor{}
	for _, descriptor := range seededStrategyDescriptors() {
		descriptors[strategyRegistryKey{Scope: globalStrategyScope(), Name: descriptor.Name}] = descriptor
	}
	return descriptors
}

func seededStrategyDescriptors() []contract.StrategyDescriptor {
	return []contract.StrategyDescriptor{
		newStrategyDescriptor(contract.StrategyBalanced, "v1", "Balanced default scheduler strategy.", map[string]float64{
			"health":   0.30,
			"quota":    0.20,
			"latency":  0.15,
			"sticky":   0.10,
			"cache":    0.10,
			"cost":     0.10,
			"fairness": 0.05,
			"quality":  0.00,
		}),
		newStrategyDescriptor(contract.StrategyCostSaver, "v1", "Cost-saving scheduler strategy.", map[string]float64{
			"cost":     0.30,
			"quota":    0.20,
			"cache":    0.15,
			"health":   0.15,
			"fairness": 0.10,
			"latency":  0.05,
			"sticky":   0.05,
			"quality":  0.00,
		}),
		newStrategyDescriptor(contract.StrategyLatencyFirst, "v1", "Low-latency scheduler strategy.", map[string]float64{
			"latency":  0.35,
			"health":   0.25,
			"quota":    0.15,
			"sticky":   0.10,
			"cost":     0.05,
			"cache":    0.05,
			"fairness": 0.05,
			"quality":  0.00,
		}),
		newStrategyDescriptor(contract.StrategyQuotaProtect, "v1", "Quota-protection scheduler strategy.", map[string]float64{
			"quota":    0.35,
			"health":   0.25,
			"cost":     0.15,
			"latency":  0.10,
			"fairness": 0.05,
			"sticky":   0.05,
			"cache":    0.05,
			"quality":  0.00,
		}),
		newStrategyDescriptor(contract.StrategyStickyFirst, "v1", "Sticky-affinity scheduler strategy.", map[string]float64{
			"sticky":   0.35,
			"health":   0.25,
			"quota":    0.15,
			"latency":  0.10,
			"cost":     0.05,
			"cache":    0.05,
			"fairness": 0.05,
			"quality":  0.00,
		}),
		newStrategyDescriptor(contract.StrategyCacheAffinityFirst, "v1", "Cache-affinity scheduler strategy.", map[string]float64{
			"cache":    0.30,
			"cost":     0.20,
			"health":   0.20,
			"quota":    0.15,
			"latency":  0.05,
			"sticky":   0.05,
			"fairness": 0.05,
			"quality":  0.00,
		}),
		newStrategyDescriptor(contract.StrategyPremiumQuality, "v1", "Premium-quality scheduler strategy.", map[string]float64{
			"health":   0.35,
			"latency":  0.20,
			"quota":    0.15,
			"sticky":   0.10,
			"cost":     0.05,
			"cache":    0.05,
			"fairness": 0.05,
			"quality":  0.05,
		}),
	}
}

func (r *StrategyRegistry) Resolve(name contract.StrategyName, scopes ...strategyScopeKey) (contract.StrategyDescriptor, error) {
	if name == "" {
		name = contract.StrategyBalanced
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, scope := range scopes {
		descriptor, ok := r.descriptors[strategyRegistryKey{Scope: scope, Name: name}]
		if ok && descriptor.Status == contract.StrategyStatusActive {
			return cloneStrategyDescriptor(descriptor), nil
		}
	}
	descriptor, ok := r.descriptors[strategyRegistryKey{Scope: globalStrategyScope(), Name: name}]
	if !ok || descriptor.Status != contract.StrategyStatusActive {
		return contract.StrategyDescriptor{}, ErrInvalidInput
	}
	return cloneStrategyDescriptor(descriptor), nil
}

func (r *StrategyRegistry) List() []contract.StrategyDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]contract.StrategyDescriptor, 0, len(r.descriptors))
	for _, descriptor := range r.descriptors {
		out = append(out, cloneStrategyDescriptor(descriptor))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScopeType != out[j].ScopeType {
			return out[i].ScopeType < out[j].ScopeType
		}
		leftScopeID := 0
		rightScopeID := 0
		if out[i].ScopeID != nil {
			leftScopeID = *out[i].ScopeID
		}
		if out[j].ScopeID != nil {
			rightScopeID = *out[j].ScopeID
		}
		if leftScopeID != rightScopeID {
			return leftScopeID < rightScopeID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func newStrategyDescriptor(name contract.StrategyName, version string, description string, weights map[string]float64) contract.StrategyDescriptor {
	config := map[string]any{
		"weights":           weightsPayload(weights),
		"hard_rules":        []string{"account_disabled", "credential_invalid", "quota_exhausted", "rpm_limit_exceeded", "tpm_limit_exceeded", "concurrency_full", "circuit_open", "cooldown_active", "capability_mismatch"},
		"fallback_rules":    map[string]any{"max_attempts": 1},
		"randomization":     map[string]any{"top_n": 1, "seeded_tests": true},
		"risk_controls":     []string{"runtime_class", "account_status", "circuit_breaker"},
		"observability":     []string{"decision", "feedback", "usage"},
		"strategy_registry": "seed",
	}
	return contract.StrategyDescriptor{
		Name:        name,
		Version:     version,
		Status:      contract.StrategyStatusActive,
		ScopeType:   contract.StrategyScopeGlobal,
		Config:      config,
		ConfigHash:  configHash(config),
		Weights:     cloneWeights(weights),
		Description: description,
	}
}

func configHash(config map[string]any) string {
	raw, err := json.Marshal(config)
	if err != nil {
		return "sha256:invalid"
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneStrategyDescriptor(value contract.StrategyDescriptor) contract.StrategyDescriptor {
	value.Config = cloneMapAny(value.Config)
	value.Weights = cloneWeights(value.Weights)
	return value
}

func cloneWeights(values map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneMapAny(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func weightsPayload(weights map[string]float64) map[string]any {
	out := make(map[string]any, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}
