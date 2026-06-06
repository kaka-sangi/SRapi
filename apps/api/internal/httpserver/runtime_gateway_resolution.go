package httpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// This file holds the pure helpers that derive scheduler runtime state and
// effective capabilities from model + account metadata. Split out of
// runtime_gateway_core.go to keep that route-family file under the
// architecture partition size limit; no behavior change.

func effectiveCapabilities(model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, account accountcontract.ProviderAccount) []capabilitiescontract.Descriptor {
	merged := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range model.Capabilities {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			merged[normalized.Key] = normalized
		}
	}
	for _, descriptor := range mapping.CapabilityOverride {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			merged[normalized.Key] = normalized
		}
	}
	providerScoped := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range mapping.CapabilityOverride {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			providerScoped[normalized.Key] = normalized
		}
	}
	for key, value := range provider.Capabilities {
		capabilityKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(key)
		if ok && boolValue(value) {
			merged[capabilityKey] = capabilityRequirement(capabilityKey)
			providerScoped[capabilityKey] = capabilityRequirement(capabilityKey)
		}
	}
	for key, value := range account.Metadata {
		if strings.HasPrefix(key, "capability_") && boolValue(value) {
			capabilityKey := strings.TrimPrefix(key, "capability_")
			if canonicalKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(capabilityKey); ok {
				merged[canonicalKey] = capabilityRequirement(canonicalKey)
				providerScoped[canonicalKey] = capabilityRequirement(canonicalKey)
			}
		}
	}
	for _, key := range providerScopedCapabilityKeys() {
		if _, ok := providerScoped[key]; !ok {
			delete(merged, key)
		}
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(merged))
	for _, descriptor := range merged {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func capabilityRequirement(key string) capabilitiescontract.Descriptor {
	return capabilitiescontract.Descriptor{
		Key:     key,
		Level:   capabilitiescontract.DescriptorLevelRequired,
		Status:  capabilitiescontract.DescriptorStatusStable,
		Version: "v1",
	}
}

func providerScopedCapabilityKeys() []string {
	return []string{
		capabilitiescontract.KeyEmbeddings,
		capabilitiescontract.KeyImages,
		capabilitiescontract.KeyAudioTranscriptions,
		capabilitiescontract.KeyAudioSpeech,
		capabilitiescontract.KeyModerations,
		capabilitiescontract.KeyRerank,
		capabilitiescontract.KeyRealtimeWebSocket,
		capabilitiescontract.KeyResponsesCompact,
		capabilitiescontract.KeyTokenCounting,
	}
}

func dedupeCapabilityDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]capabilitiescontract.Descriptor{}
	for _, value := range values {
		seen[value.Key] = value
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func schedulerRuntimeState(metadata map[string]any) schedulercontract.RuntimeState {
	quotaRemainingRatio := metadataOptionalFloat(metadata, "remaining_ratio", "quota_remaining_ratio")
	quotaExhausted := metadataBool(metadata, "quota_exhausted")
	if quotaRemainingRatio != nil && *quotaRemainingRatio <= 0 {
		quotaExhausted = true
	}
	return schedulercontract.RuntimeState{
		QuotaExhausted:      quotaExhausted,
		HealthScore:         metadataOptionalFloat(metadata, "health_score"),
		QuotaRemainingRatio: quotaRemainingRatio,
		LatencyP95MS:        metadataOptionalInt(metadata, "latency_p95_ms", "p95_latency_ms", "latency_p95"),
		CircuitOpen:         metadataBool(metadata, "circuit_open"),
		CooldownActive:      metadataBool(metadata, "cooldown_active") || metadataCooldownActive(metadata, time.Now().UTC()),
		CurrentConcurrency:  metadataInt(metadata, "current_concurrency"),
		RPMUsed:             metadataInt(metadata, "rpm_used"),
		TPMUsed:             metadataInt(metadata, "tpm_used"),
		CostWindowUsed:      metadataFloatValue(metadata, "cost_window_used"),
	}
}

// metadataFloatValue reads a float metadata value, returning 0 when absent.
func metadataFloatValue(metadata map[string]any, keys ...string) float64 {
	if value := metadataOptionalFloat(metadata, keys...); value != nil {
		return *value
	}
	return 0
}

func schedulerRuntimeLimits(metadata map[string]any) schedulercontract.RuntimeLimits {
	return schedulercontract.RuntimeLimits{
		MaxConcurrency:  metadataOptionalInt(metadata, "max_concurrency"),
		RPMLimit:        metadataOptionalInt(metadata, "rpm_limit"),
		TPMLimit:        metadataOptionalInt(metadata, "tpm_limit"),
		CostWindowLimit: metadataOptionalFloat(metadata, "cost_window_limit"),
	}
}

func metadataBool(metadata map[string]any, key string) bool {
	return boolValue(metadata[key])
}

func boolValue(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	default:
		return false
	}
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	switch value := value.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return int(parsed)
		}
		floatValue, err := value.Float64()
		if err == nil {
			return int(floatValue)
		}
	case string:
		raw := strings.TrimSpace(value)
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			return parsed
		}
		floatValue, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return int(floatValue)
		}
	}
	return 0
}

func metadataOptionalInt(metadata map[string]any, keys ...string) *int {
	if _, ok := metadataValue(metadata, keys...); !ok {
		return nil
	}
	value := metadataInt(metadata, keys...)
	return &value
}

func metadataOptionalFloat(metadata map[string]any, keys ...string) *float64 {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	switch value := value.(type) {
	case int:
		out := float64(value)
		return &out
	case int8:
		out := float64(value)
		return &out
	case int16:
		out := float64(value)
		return &out
	case int32:
		out := float64(value)
		return &out
	case int64:
		out := float64(value)
		return &out
	case uint:
		out := float64(value)
		return &out
	case uint8:
		out := float64(value)
		return &out
	case uint16:
		out := float64(value)
		return &out
	case uint32:
		out := float64(value)
		return &out
	case uint64:
		out := float64(value)
		return &out
	case float32:
		out := float64(value)
		return &out
	case float64:
		return &value
	case json.Number:
		parsed, err := value.Float64()
		if err == nil {
			return &parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func metadataValue(metadata map[string]any, keys ...string) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func metadataCooldownActive(metadata map[string]any, now time.Time) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata["cooldown_until"]
	if !ok {
		return false
	}
	var raw string
	switch value := value.(type) {
	case string:
		raw = value
	default:
		raw = fmt.Sprint(value)
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	return err == nil && now.Before(until)
}
