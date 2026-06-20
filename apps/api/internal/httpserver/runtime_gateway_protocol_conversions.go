package httpserver

import (
	"context"
	"strings"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

const (
	gatewayProtocolRouteChatToResponses     = "chat_completions_to_responses"
	gatewayProtocolRouteChatToMessages      = "chat_completions_to_messages"
	gatewayProtocolRouteResponsesToChat     = "responses_to_chat_completions"
	gatewayProtocolRouteResponsesToMessages = "responses_to_messages"
	gatewayProtocolRouteMessagesToChat      = "messages_to_chat_completions"
	gatewayProtocolRouteMessagesToResponses = "messages_to_responses"
)

var defaultGatewayProtocolRouteSet = map[string]bool{
	gatewayProtocolRouteChatToResponses:     true,
	gatewayProtocolRouteChatToMessages:      true,
	gatewayProtocolRouteResponsesToChat:     true,
	gatewayProtocolRouteResponsesToMessages: true,
	gatewayProtocolRouteMessagesToChat:      true,
	gatewayProtocolRouteMessagesToResponses: true,
}

func defaultGatewayProtocolConversionRoutes() []string {
	return []string{
		gatewayProtocolRouteChatToResponses,
		gatewayProtocolRouteChatToMessages,
		gatewayProtocolRouteResponsesToChat,
		gatewayProtocolRouteResponsesToMessages,
		gatewayProtocolRouteMessagesToChat,
		gatewayProtocolRouteMessagesToResponses,
	}
}

func (rt *runtimeState) filterCandidatesByProtocolConversions(ctx context.Context, candidates []schedulercontract.Candidate) []schedulercontract.Candidate {
	if len(candidates) == 0 || rt.adminControl == nil {
		return candidates
	}
	settings, err := rt.adminControl.GetAdminSettings(ctx)
	if err != nil {
		if rt.logger != nil {
			rt.logger.Warn("protocol conversion filter skipped: admin settings unavailable", "error", err)
		}
		return candidates
	}
	enabledRoutes := gatewayProtocolConversionRouteSet(settings.Gateway)
	out := make([]schedulercontract.Candidate, len(candidates))
	for idx, candidate := range candidates {
		candidate.EffectiveCapabilities = filterEndpointCapabilitiesByProtocolConversions(candidate, enabledRoutes)
		out[idx] = candidate
	}
	return out
}

func gatewayProtocolConversionRouteSet(settings admincontrol.AdminSettingsGateway) map[string]bool {
	if settings.ProtocolConversionRoutes == nil {
		out := make(map[string]bool, len(defaultGatewayProtocolRouteSet))
		for route := range defaultGatewayProtocolRouteSet {
			out[route] = true
		}
		return out
	}
	out := make(map[string]bool, len(settings.ProtocolConversionRoutes))
	for _, route := range settings.ProtocolConversionRoutes {
		route = strings.ToLower(strings.TrimSpace(route))
		if defaultGatewayProtocolRouteSet[route] {
			out[route] = true
		}
	}
	return out
}

func filterEndpointCapabilitiesByProtocolConversions(candidate schedulercontract.Candidate, enabledRoutes map[string]bool) []capabilitiescontract.Descriptor {
	capabilities := candidate.EffectiveCapabilities
	if len(capabilities) == 0 {
		return capabilities
	}
	native := nativeConversationEndpointCapabilities(candidate)
	if len(native) == 0 {
		return capabilities
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(capabilities))
	for _, capability := range capabilities {
		key := strings.TrimSpace(capability.Key)
		switch key {
		case capabilitiescontract.KeyChatCompletions, capabilitiescontract.KeyResponses, capabilitiescontract.KeyMessages:
			if native[key] || conversionRouteEnabled(native, key, enabledRoutes) {
				out = append(out, capability)
			}
		default:
			out = append(out, capability)
		}
	}
	return out
}

func nativeConversationEndpointCapabilities(candidate schedulercontract.Candidate) map[string]bool {
	provider := candidate.Provider
	if strings.EqualFold(strings.TrimSpace(provider.AdapterType), "reverse-proxy-codex-cli") {
		return map[string]bool{capabilitiescontract.KeyResponses: true}
	}
	protocol := strings.ToLower(strings.TrimSpace(provider.Protocol))
	if protocol == "" {
		protocol = strings.ToLower(strings.TrimSpace(provider.AdapterType))
	}
	switch protocol {
	case string(gatewaycontract.ProtocolAnthropicCompatible):
		return map[string]bool{capabilitiescontract.KeyMessages: true}
	case string(gatewaycontract.ProtocolGeminiCompatible):
		return map[string]bool{}
	default:
		native := map[string]bool{capabilitiescontract.KeyChatCompletions: true}
		if openAIResponsesNativeCandidate(candidate) {
			native[capabilitiescontract.KeyResponses] = true
		}
		return native
	}
}

func openAIResponsesNativeCandidate(candidate schedulercontract.Candidate) bool {
	provider := candidate.Provider
	adapterType := strings.ToLower(strings.TrimSpace(provider.AdapterType))
	if adapterType == "native-openai" || adapterType == "native-grok" || adapterType == "xai-compatible" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(provider.Name)) {
	case "openai", "grok":
		return true
	}
	for _, values := range []map[string]any{candidate.Account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		if metadataBool(values, "native_responses") ||
			metadataBool(values, "responses_native") ||
			metadataBool(values, "responses_passthrough") ||
			metadataBool(values, "openai_responses_passthrough") {
			return true
		}
	}
	return false
}

func conversionRouteEnabled(native map[string]bool, targetCapability string, enabledRoutes map[string]bool) bool {
	for sourceCapability := range native {
		if enabledRoutes[protocolConversionRoute(sourceCapability, targetCapability)] {
			return true
		}
	}
	return false
}

func protocolConversionRoute(sourceCapability string, targetCapability string) string {
	switch sourceCapability {
	case capabilitiescontract.KeyChatCompletions:
		switch targetCapability {
		case capabilitiescontract.KeyResponses:
			return gatewayProtocolRouteChatToResponses
		case capabilitiescontract.KeyMessages:
			return gatewayProtocolRouteChatToMessages
		}
	case capabilitiescontract.KeyResponses:
		switch targetCapability {
		case capabilitiescontract.KeyChatCompletions:
			return gatewayProtocolRouteResponsesToChat
		case capabilitiescontract.KeyMessages:
			return gatewayProtocolRouteResponsesToMessages
		}
	case capabilitiescontract.KeyMessages:
		switch targetCapability {
		case capabilitiescontract.KeyChatCompletions:
			return gatewayProtocolRouteMessagesToChat
		case capabilitiescontract.KeyResponses:
			return gatewayProtocolRouteMessagesToResponses
		}
	}
	return ""
}
