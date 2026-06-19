package httpserver

import (
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

func TestEffectiveCapabilitiesUsesPresetBaselineWithProviderOverrides(t *testing.T) {
	model := modelcontract.Model{}
	mapping := modelcontract.ModelProviderMapping{}

	cases := []struct {
		name              string
		provider          providercontract.Provider
		account           accountcontract.ProviderAccount
		wantCompact       bool
		wantVision        bool
		wantTokenCounting bool
	}{
		{
			name: "partial provider capabilities keep preset compact",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
			wantCompact: true,
			wantVision:  true,
		},
		{
			name: "provider false disables preset capability",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponsesCompact: false},
			},
			wantVision: true,
		},
		{
			name: "account false disables provider scoped capability",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"capability_responses_compact": false},
			},
			wantVision: true,
		},
		{
			name: "account true enables explicit provider scoped capability",
			provider: providercontract.Provider{
				AdapterType:  "anthropic-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyTokenCounting: false},
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"capability_token_counting": true},
			},
			wantTokenCounting: true,
			wantVision:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveCapabilities(model, mapping, tc.provider, tc.account)
			if hasCapability(got, capabilitiescontract.KeyResponsesCompact) != tc.wantCompact {
				t.Fatalf("responses_compact presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyResponsesCompact), tc.wantCompact, got)
			}
			if hasCapability(got, capabilitiescontract.KeyVisionInput) != tc.wantVision {
				t.Fatalf("vision_input presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyVisionInput), tc.wantVision, got)
			}
			if hasCapability(got, capabilitiescontract.KeyTokenCounting) != tc.wantTokenCounting {
				t.Fatalf("token_counting presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyTokenCounting), tc.wantTokenCounting, got)
			}
		})
	}
}

func hasCapability(values []capabilitiescontract.Descriptor, key string) bool {
	for _, value := range values {
		if value.Key == key {
			return true
		}
	}
	return false
}

func TestEffectiveCapabilitiesResponsesCompactRequiresExplicitCapability(t *testing.T) {
	model := modelcontract.Model{}
	mapping := modelcontract.ModelProviderMapping{}

	cases := []struct {
		name        string
		provider    providercontract.Provider
		account     accountcontract.ProviderAccount
		wantCompact bool
	}{
		{
			name: "preset with explicit compact advertises compact",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			wantCompact: true,
		},
		{
			name: "ordinary responses capability does not imply compact",
			provider: providercontract.Provider{
				AdapterType:  "unknown-provider",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
		},
		{
			name: "provider explicit compact true enables compact",
			provider: providercontract.Provider{
				AdapterType:  "unknown-provider",
				Capabilities: map[string]any{capabilitiescontract.KeyResponsesCompact: true},
			},
			wantCompact: true,
		},
		{
			name: "provider explicit compact false suppresses preset compact",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponsesCompact: false},
			},
		},
		{
			name: "account explicit compact false suppresses preset compact",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"capability_responses_compact": false},
			},
		},
		{
			name: "account metadata disable_responses_compact opts out",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"disable_responses_compact": true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveCapabilities(model, mapping, tc.provider, tc.account)
			if hasCapability(got, capabilitiescontract.KeyResponsesCompact) != tc.wantCompact {
				t.Fatalf("responses_compact presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyResponsesCompact), tc.wantCompact, got)
			}
		})
	}
}

func TestEffectiveCapabilitiesResponsesInputItemsRequiresNativeSubresource(t *testing.T) {
	model := modelcontract.Model{
		Capabilities: []capabilitiescontract.Descriptor{
			capabilityRequirement(capabilitiescontract.KeyResponses),
			capabilityRequirement(capabilitiescontract.KeyResponsesInputItems),
		},
	}
	mapping := modelcontract.ModelProviderMapping{}

	cases := []struct {
		name      string
		provider  providercontract.Provider
		account   accountcontract.ProviderAccount
		wantInput bool
	}{
		{
			name: "model capability alone does not advertise input_items",
			provider: providercontract.Provider{
				AdapterType:  "unknown-provider",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
		},
		{
			name: "generic openai-compatible preset advertises native input_items",
			provider: providercontract.Provider{
				Name:        "openai-compatible",
				AdapterType: "openai-compatible",
			},
			wantInput: true,
		},
		{
			name: "concrete openai-compatible provider preset does not inherit generic input_items",
			provider: providercontract.Provider{
				Name:         "deepseek",
				AdapterType:  "openai-compatible",
				ConfigSchema: map[string]any{"provider_key": "deepseek"},
			},
		},
		{
			name: "codex reverse proxy advertises native input_items",
			provider: providercontract.Provider{
				Name:        "codex-cli",
				AdapterType: "reverse-proxy-codex-cli",
			},
			wantInput: true,
		},
		{
			name: "anthropic responses conversion does not imply input_items",
			provider: providercontract.Provider{
				AdapterType: "anthropic-compatible",
			},
		},
		{
			name: "gemini text conversion does not imply input_items",
			provider: providercontract.Provider{
				AdapterType: "gemini-compatible",
			},
		},
		{
			name: "provider override can enable input_items",
			provider: providercontract.Provider{
				AdapterType:  "unknown-provider",
				Capabilities: map[string]any{capabilitiescontract.KeyResponsesInputItems: true},
			},
			wantInput: true,
		},
		{
			name: "account false suppresses preset input_items",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"capability_responses_input_items": false},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveCapabilities(model, mapping, tc.provider, tc.account)
			if hasCapability(got, capabilitiescontract.KeyResponsesInputItems) != tc.wantInput {
				t.Fatalf("responses_input_items presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyResponsesInputItems), tc.wantInput, got)
			}
		})
	}
}

func TestEffectiveCapabilitiesResponsesWebSocketIsCodexAccountScoped(t *testing.T) {
	model := modelcontract.Model{}
	mapping := modelcontract.ModelProviderMapping{}

	cases := []struct {
		name     string
		provider providercontract.Provider
		account  accountcontract.ProviderAccount
		want     bool
	}{
		{
			name: "provider capability alone is ignored",
			provider: providercontract.Provider{
				AdapterType:  "reverse-proxy-codex-cli",
				Capabilities: map[string]any{capabilitiescontract.KeyResponsesWebSocket: true},
			},
		},
		{
			name: "codex account metadata enables capability",
			provider: providercontract.Provider{
				AdapterType: "reverse-proxy-codex-cli",
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"codex_responses_websocket": true},
			},
			want: true,
		},
		{
			name: "non codex account metadata does not enable capability",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"codex_responses_websocket": true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveCapabilities(model, mapping, tc.provider, tc.account)
			if hasCapability(got, capabilitiescontract.KeyResponsesWebSocket) != tc.want {
				t.Fatalf("responses_websocket presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyResponsesWebSocket), tc.want, got)
			}
		})
	}
}

func TestEffectiveCapabilitiesConversationEndpointCapabilitiesAreProviderScoped(t *testing.T) {
	model := modelcontract.Model{
		Capabilities: []capabilitiescontract.Descriptor{
			capabilityRequirement(capabilitiescontract.KeyChatCompletions),
			capabilityRequirement(capabilitiescontract.KeyResponses),
			capabilityRequirement(capabilitiescontract.KeyMessages),
		},
	}
	mapping := modelcontract.ModelProviderMapping{}

	cases := []struct {
		name         string
		provider     providercontract.Provider
		wantChat     bool
		wantResponse bool
		wantMessages bool
	}{
		{
			name: "model capabilities alone do not advertise endpoints",
			provider: providercontract.Provider{
				AdapterType:  "unknown-provider",
				Capabilities: map[string]any{},
			},
		},
		{
			name: "openai preset supplies openai endpoints",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
			},
			wantChat:     true,
			wantResponse: true,
			wantMessages: true,
		},
		{
			name: "anthropic preset supplies supported text endpoints",
			provider: providercontract.Provider{
				AdapterType: "anthropic-compatible",
			},
			wantChat:     true,
			wantResponse: true,
			wantMessages: true,
		},
		{
			name: "claude code reverse proxy supplies anthropic text endpoints",
			provider: providercontract.Provider{
				AdapterType: "reverse-proxy-claude-code-cli",
			},
			wantChat:     true,
			wantResponse: true,
			wantMessages: true,
		},
		{
			name: "gemini preset supplies supported text endpoints",
			provider: providercontract.Provider{
				AdapterType: "gemini-compatible",
			},
			wantChat:     true,
			wantMessages: true,
		},
		{
			name: "codex reverse proxy supplies text gateway endpoints",
			provider: providercontract.Provider{
				AdapterType: "reverse-proxy-codex-cli",
			},
			wantChat:     true,
			wantResponse: true,
			wantMessages: true,
		},
		{
			name: "provider override can enable one endpoint",
			provider: providercontract.Provider{
				AdapterType:  "unknown-provider",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
			wantResponse: true,
		},
		{
			name: "generic reverse proxy supplies implemented openai endpoints",
			provider: providercontract.Provider{
				AdapterType: "generic-reverse-proxy",
			},
			wantChat: true,
		},
		{
			name: "generic reverse proxy does not advertise unsupported text endpoints",
			provider: providercontract.Provider{
				AdapterType: "generic-reverse-proxy",
			},
			wantChat: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveCapabilities(model, mapping, tc.provider, accountcontract.ProviderAccount{})
			if hasCapability(got, capabilitiescontract.KeyChatCompletions) != tc.wantChat {
				t.Fatalf("chat_completions presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyChatCompletions), tc.wantChat, got)
			}
			if hasCapability(got, capabilitiescontract.KeyResponses) != tc.wantResponse {
				t.Fatalf("responses presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyResponses), tc.wantResponse, got)
			}
			if hasCapability(got, capabilitiescontract.KeyMessages) != tc.wantMessages {
				t.Fatalf("messages presence = %v, want %v; capabilities=%+v", hasCapability(got, capabilitiescontract.KeyMessages), tc.wantMessages, got)
			}
		})
	}
}

func TestEffectiveCapabilitiesGenericReverseProxyAdvertisesImplementedEmbeddings(t *testing.T) {
	model := modelcontract.Model{
		Capabilities: []capabilitiescontract.Descriptor{
			capabilityRequirement(capabilitiescontract.KeyEmbeddings),
		},
	}
	provider := providercontract.Provider{
		AdapterType: "generic-reverse-proxy",
	}

	got := effectiveCapabilities(model, modelcontract.ModelProviderMapping{}, provider, accountcontract.ProviderAccount{})
	if !hasCapability(got, capabilitiescontract.KeyEmbeddings) {
		t.Fatalf("expected generic reverse proxy to advertise implemented embeddings endpoint, got %+v", got)
	}
}
