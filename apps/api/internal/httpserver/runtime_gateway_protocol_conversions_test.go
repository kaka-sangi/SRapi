package httpserver

import (
	"slices"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func TestFilterEndpointCapabilitiesByProtocolConversionsOpenAICompatible(t *testing.T) {
	candidate := schedulercontract.Candidate{
		Provider: providercontract.Provider{
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		EffectiveCapabilities: endpointCapabilities(
			capabilitiescontract.KeyChatCompletions,
			capabilitiescontract.KeyResponses,
			capabilitiescontract.KeyMessages,
		),
	}
	got := filterEndpointCapabilitiesByProtocolConversions(candidate, map[string]bool{
		gatewayProtocolRouteChatToResponses: true,
	})

	if !hasCapability(got, capabilitiescontract.KeyChatCompletions) {
		t.Fatalf("native chat_completions should remain: %+v", got)
	}
	if !hasCapability(got, capabilitiescontract.KeyResponses) {
		t.Fatalf("enabled chat_completions_to_responses should keep responses: %+v", got)
	}
	if hasCapability(got, capabilitiescontract.KeyMessages) {
		t.Fatalf("disabled chat_completions_to_messages should remove messages: %+v", got)
	}
}

func TestFilterEndpointCapabilitiesByProtocolConversionsCodexCLI(t *testing.T) {
	candidate := schedulercontract.Candidate{
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		EffectiveCapabilities: endpointCapabilities(
			capabilitiescontract.KeyChatCompletions,
			capabilitiescontract.KeyResponses,
			capabilitiescontract.KeyMessages,
		),
	}
	got := filterEndpointCapabilitiesByProtocolConversions(candidate, map[string]bool{
		gatewayProtocolRouteResponsesToMessages: true,
	})

	if hasCapability(got, capabilitiescontract.KeyChatCompletions) {
		t.Fatalf("disabled responses_to_chat_completions should remove chat_completions: %+v", got)
	}
	if !hasCapability(got, capabilitiescontract.KeyResponses) {
		t.Fatalf("native codex responses should remain: %+v", got)
	}
	if !hasCapability(got, capabilitiescontract.KeyMessages) {
		t.Fatalf("enabled responses_to_messages should keep messages: %+v", got)
	}
}

func TestFilterEndpointCapabilitiesByProtocolConversionsNativeResponsesOptIn(t *testing.T) {
	candidate := schedulercontract.Candidate{
		Provider: providercontract.Provider{
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			Metadata: map[string]any{"native_responses": true},
		},
		EffectiveCapabilities: endpointCapabilities(
			capabilitiescontract.KeyChatCompletions,
			capabilitiescontract.KeyResponses,
			capabilitiescontract.KeyMessages,
		),
	}
	got := filterEndpointCapabilitiesByProtocolConversions(candidate, map[string]bool{})

	if !hasCapability(got, capabilitiescontract.KeyChatCompletions) || !hasCapability(got, capabilitiescontract.KeyResponses) {
		t.Fatalf("native chat and account-native responses should remain: %+v", got)
	}
	if hasCapability(got, capabilitiescontract.KeyMessages) {
		t.Fatalf("disabled text conversions should remove synthetic messages: %+v", got)
	}
}

func TestFilterEndpointCapabilitiesByProtocolConversionsLeavesGeminiTextCapabilities(t *testing.T) {
	candidate := schedulercontract.Candidate{
		Provider: providercontract.Provider{
			AdapterType: "gemini-compatible",
			Protocol:    "gemini-compatible",
		},
		EffectiveCapabilities: endpointCapabilities(
			capabilitiescontract.KeyChatCompletions,
			capabilitiescontract.KeyMessages,
			capabilitiescontract.KeyGeminiGenerateContent,
		),
	}
	got := filterEndpointCapabilitiesByProtocolConversions(candidate, map[string]bool{})

	if !hasCapability(got, capabilitiescontract.KeyChatCompletions) ||
		!hasCapability(got, capabilitiescontract.KeyMessages) ||
		!hasCapability(got, capabilitiescontract.KeyGeminiGenerateContent) {
		t.Fatalf("gemini gateway text capabilities should remain outside chat/responses/messages route toggles: %+v", got)
	}
}

func TestToAPIAdminSettingsProtocolConversionsDefaultNil(t *testing.T) {
	settings := admincontrol.AdminSettings{
		Gateway: admincontrol.AdminSettingsGateway{
			ProtocolConversionRoutes: nil,
		},
	}

	got := toAPIAdminSettings(settings)
	if got.Gateway.ProtocolConversionRoutes == nil {
		t.Fatal("expected default protocol conversion routes, got nil")
	}
	want := []string{
		gatewayProtocolRouteChatToResponses,
		gatewayProtocolRouteChatToMessages,
		gatewayProtocolRouteResponsesToChat,
		gatewayProtocolRouteResponsesToMessages,
		gatewayProtocolRouteMessagesToChat,
		gatewayProtocolRouteMessagesToResponses,
	}
	var routes []string
	for _, route := range *got.Gateway.ProtocolConversionRoutes {
		routes = append(routes, string(route))
	}
	if !slices.Equal(routes, want) {
		t.Fatalf("routes = %v, want %v", routes, want)
	}
}

func TestToAPIAdminSettingsProtocolConversionsPreservesEmpty(t *testing.T) {
	settings := admincontrol.AdminSettings{
		Gateway: admincontrol.AdminSettingsGateway{
			ProtocolConversionRoutes: []string{},
		},
	}

	got := toAPIAdminSettings(settings)
	if got.Gateway.ProtocolConversionRoutes == nil {
		t.Fatal("expected explicit empty protocol conversion routes, got nil")
	}
	if len(*got.Gateway.ProtocolConversionRoutes) != 0 {
		t.Fatalf("expected explicit empty protocol conversion routes, got %v", *got.Gateway.ProtocolConversionRoutes)
	}
}

func endpointCapabilities(keys ...string) []capabilitiescontract.Descriptor {
	out := make([]capabilitiescontract.Descriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, capabilityRequirement(key))
	}
	return out
}
