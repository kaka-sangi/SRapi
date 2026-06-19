package httpserver

import (
	"fmt"
	"sort"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/metacoerce"
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
	// Preset baseline always merges in first: provider.Capabilities and account
	// metadata then act as overrides (true to enable, false to disable) rather
	// than full replacements. This mirrors sub2api's account-extra capability
	// shape and matches the ported behavior tested by
	// TestEffectiveCapabilitiesUsesPresetBaselineWithProviderOverrides.
	explicitlyDisabled := map[string]bool{}
	if presetCaps := presetCapabilitiesForAdapter(provider.AdapterType); len(presetCaps) > 0 {
		for key, value := range presetCaps {
			canonicalKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(key)
			if !ok {
				continue
			}
			if boolValue(value) {
				merged[canonicalKey] = capabilityRequirement(canonicalKey)
				providerScoped[canonicalKey] = capabilityRequirement(canonicalKey)
			}
		}
	}
	for key, value := range provider.Capabilities {
		capabilityKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(key)
		if !ok {
			continue
		}
		if capabilityKey == capabilitiescontract.KeyResponsesWebSocket {
			continue
		}
		if boolValue(value) {
			merged[capabilityKey] = capabilityRequirement(capabilityKey)
			providerScoped[capabilityKey] = capabilityRequirement(capabilityKey)
		} else {
			explicitlyDisabled[capabilityKey] = true
			delete(merged, capabilityKey)
			delete(providerScoped, capabilityKey)
		}
	}
	for key, value := range account.Metadata {
		if !strings.HasPrefix(key, "capability_") {
			continue
		}
		capabilityKey := strings.TrimPrefix(key, "capability_")
		canonicalKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(capabilityKey)
		if !ok {
			continue
		}
		if boolValue(value) {
			merged[canonicalKey] = capabilityRequirement(canonicalKey)
			providerScoped[canonicalKey] = capabilityRequirement(canonicalKey)
			delete(explicitlyDisabled, canonicalKey)
		} else {
			explicitlyDisabled[canonicalKey] = true
			delete(merged, canonicalKey)
			delete(providerScoped, canonicalKey)
		}
	}
	if strings.EqualFold(strings.TrimSpace(provider.AdapterType), "reverse-proxy-codex-cli") && accountCodexWebSocketEnabled(account.Metadata) {
		merged[capabilitiescontract.KeyResponsesWebSocket] = capabilityRequirement(capabilitiescontract.KeyResponsesWebSocket)
		providerScoped[capabilitiescontract.KeyResponsesWebSocket] = capabilityRequirement(capabilitiescontract.KeyResponsesWebSocket)
	} else {
		delete(merged, capabilitiescontract.KeyResponsesWebSocket)
		delete(providerScoped, capabilitiescontract.KeyResponsesWebSocket)
	}
	// Auto-include responses_compact whenever the merged set already declares
	// responses: compact is a strict non-streaming subset of /v1/responses, so
	// any account that can serve full responses can also serve compact. The
	// override is suppressed when the operator explicitly disabled compact via
	// provider.Capabilities[responses_compact]=false, account metadata
	// capability_responses_compact=false, or the
	// metadata.disable_responses_compact opt-out flag. Mirrors sub2api's
	// openai_gateway_service.openAICompactSupportTier which treats compact
	// support as a tier on top of /responses rather than an parallel
	// capability requirement.
	disableCompact := disableResponsesCompactOptedOut(provider, account)
	if disableCompact {
		delete(merged, capabilitiescontract.KeyResponsesCompact)
		delete(providerScoped, capabilitiescontract.KeyResponsesCompact)
	}
	if _, hasResponses := merged[capabilitiescontract.KeyResponses]; hasResponses &&
		!explicitlyDisabled[capabilitiescontract.KeyResponsesCompact] &&
		!disableCompact {
		if _, ok := merged[capabilitiescontract.KeyResponsesCompact]; !ok {
			merged[capabilitiescontract.KeyResponsesCompact] = capabilityRequirement(capabilitiescontract.KeyResponsesCompact)
			providerScoped[capabilitiescontract.KeyResponsesCompact] = capabilityRequirement(capabilitiescontract.KeyResponsesCompact)
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

// disableResponsesCompactOptedOut reports whether the operator explicitly
// suppressed the responses_compact auto-include via metadata. Provider or
// account metadata may set disable_responses_compact=true to force compact
// requests to fall back to cross-protocol synthesis on this account.
func disableResponsesCompactOptedOut(provider providercontract.Provider, account accountcontract.ProviderAccount) bool {
	if metadataBool(account.Metadata, "disable_responses_compact") {
		return true
	}
	for _, source := range []map[string]any{provider.Capabilities, provider.ConfigSchema} {
		if metadataBool(source, "disable_responses_compact") {
			return true
		}
	}
	return false
}

func capabilityRequirement(key string) capabilitiescontract.Descriptor {
	return capabilitiescontract.Descriptor{
		Key:     key,
		Level:   capabilitiescontract.DescriptorLevelRequired,
		Status:  capabilitiescontract.DescriptorStatusStable,
		Version: "v1",
	}
}

func presetCapabilitiesForAdapter(adapterType string) map[string]any {
	family := adapterTypeToPlatformFamily(adapterType)
	if family == "" {
		return nil
	}
	caps := providerpreset.Default().CapabilitiesForPlatformFamily(family)
	if len(caps) == 0 {
		return nil
	}
	out := make(map[string]any, len(caps))
	for k, v := range caps {
		out[k] = v
	}
	return out
}

func adapterTypeToPlatformFamily(adapterType string) providerpreset.PlatformFamily {
	switch adapterType {
	case "anthropic-compatible":
		return providerpreset.PlatformFamilyAnthropicCompatible
	case "reverse-proxy-antigravity":
		return providerpreset.PlatformFamilyReverseProxyAntigravity
	case "rerank-compatible":
		return providerpreset.PlatformFamilyRerankCompatible
	case "reverse-proxy-codex-cli":
		return providerpreset.PlatformFamilyCodexCLI
	case "openai-compatible", "reverse-proxy-chatgpt-web":
		return providerpreset.PlatformFamilyOpenAICompatible
	default:
		return ""
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
		capabilitiescontract.KeyResponsesWebSocket,
		capabilitiescontract.KeyResponsesCompact,
		capabilitiescontract.KeyTokenCounting,
		capabilitiescontract.KeyVisionInput,
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
	now := time.Now().UTC()
	quotaRemainingRatio := metadataOptionalFloat(metadata, "remaining_ratio", "quota_remaining_ratio")
	quotaExhausted := metadataBool(metadata, "quota_exhausted")
	if quotaMetadataWindowReset(metadata, now) {
		quotaRemainingRatio = nil
		quotaExhausted = false
	}
	if quotaRemainingRatio != nil && *quotaRemainingRatio <= 0 {
		quotaExhausted = true
	}
	return schedulercontract.RuntimeState{
		QuotaExhausted:      quotaExhausted,
		HealthScore:         metadataOptionalFloat(metadata, "health_score"),
		QuotaRemainingRatio: quotaRemainingRatio,
		LatencyP95MS:        metadataOptionalInt(metadata, "latency_p95_ms", "p95_latency_ms", "latency_p95"),
		CircuitOpen:         metadataBool(metadata, "circuit_open"),
		CooldownActive:      metadataBool(metadata, "cooldown_active") || metadataCooldownActive(metadata, now),
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
	return metacoerce.Bool(metadata, key)
}

func boolValue(value any) bool {
	return metacoerce.BoolValue(value)
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metacoerce.Value(metadata, keys...)
	if !ok {
		return 0
	}
	parsed, ok := metacoerce.Int(value)
	if !ok {
		return 0
	}
	return parsed
}

func metadataOptionalInt(metadata map[string]any, keys ...string) *int {
	return metacoerce.OptionalInt(metadata, keys...)
}

func metadataOptionalFloat(metadata map[string]any, keys ...string) *float64 {
	return metacoerce.OptionalFloat(metadata, keys...)
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

func quotaMetadataWindowReset(metadata map[string]any, now time.Time) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata["quota_reset_at"]
	if !ok || value == nil {
		return false
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" {
		return false
	}
	resetAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return false
	}
	return !now.Before(resetAt.UTC())
}
