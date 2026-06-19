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

// TestEffectiveCapabilitiesAutoIncludesResponsesCompactFromResponses proves
// /v1/responses/compact is treated as a strict non-streaming subset of
// /v1/responses: any account that declares the responses capability
// automatically advertises responses_compact too, unless the operator
// explicitly opted out via metadata.disable_responses_compact or by setting
// capability_responses_compact=false on the provider or account.
func TestEffectiveCapabilitiesAutoIncludesResponsesCompactFromResponses(t *testing.T) {
	model := modelcontract.Model{}
	mapping := modelcontract.ModelProviderMapping{}

	cases := []struct {
		name        string
		provider    providercontract.Provider
		account     accountcontract.ProviderAccount
		wantCompact bool
	}{
		{
			name: "responses on model only auto-includes compact",
			provider: providercontract.Provider{
				AdapterType: "openai-compatible",
				// Provider declares no caps map at all (preset baseline applies):
				// the openai-compatible preset already advertises responses +
				// responses_compact, so the auto-include is a no-op here.
			},
			wantCompact: true,
		},
		{
			name: "preset provider with no overrides auto-includes compact",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
			wantCompact: true,
		},
		{
			name: "provider explicit responses_compact=false suppresses auto-include",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponsesCompact: false},
			},
			wantCompact: false,
		},
		{
			name: "account capability_responses_compact=false suppresses auto-include",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"capability_responses_compact": false},
			},
			wantCompact: false,
		},
		{
			name: "account metadata disable_responses_compact opts out",
			provider: providercontract.Provider{
				AdapterType:  "openai-compatible",
				Capabilities: map[string]any{capabilitiescontract.KeyResponses: true},
			},
			account: accountcontract.ProviderAccount{
				Metadata: map[string]any{"disable_responses_compact": true},
			},
			wantCompact: false,
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
