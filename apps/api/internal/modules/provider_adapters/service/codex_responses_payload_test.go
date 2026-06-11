package service

import (
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestCodexResponsesPayloadStripsUnsupportedCompatibilityFields(t *testing.T) {
	payload, stream, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"codex-local",
			"input":"hello",
			"context_management":{"strategy":"auto"},
			"truncation":"auto",
			"max_output_tokens":64,
			"temperature":0.2,
			"stream":false
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex responses payload: %v", err)
	}
	if !stream {
		t.Fatal("codex responses payload should stream by default")
	}
	if payload["model"] != "codex-upstream" {
		t.Fatalf("model = %v, want codex-upstream", payload["model"])
	}
	for _, removed := range []string{"context_management", "truncation", "max_output_tokens", "temperature"} {
		if _, ok := payload[removed]; ok {
			t.Fatalf("expected %s to be removed, got %+v", removed, payload)
		}
	}
}

func TestCodexResponsesPayloadKeepsOnlyPriorityServiceTier(t *testing.T) {
	tests := []struct {
		name        string
		rawPayload  []byte
		accountMeta map[string]any
		wantTier    string
		wantPresent bool
	}{
		{
			name: "raw priority",
			rawPayload: []byte(`{
				"model":"codex-local",
				"input":"hello",
				"service_tier":"priority"
			}`),
			wantTier:    "priority",
			wantPresent: true,
		},
		{
			name: "raw fast alias",
			rawPayload: []byte(`{
				"model":"codex-local",
				"input":"hello",
				"service_tier":"fast"
			}`),
			wantTier:    "priority",
			wantPresent: true,
		},
		{
			name: "raw unsupported auto",
			rawPayload: []byte(`{
				"model":"codex-local",
				"input":"hello",
				"service_tier":"auto"
			}`),
			wantPresent: false,
		},
		{
			name: "configured unsupported default",
			rawPayload: []byte(`{
				"model":"codex-local",
				"input":"hello"
			}`),
			accountMeta: map[string]any{"codex_service_tier": "default"},
			wantPresent: false,
		},
		{
			name: "configured fast alias",
			rawPayload: []byte(`{
				"model":"codex-local",
				"input":"hello"
			}`),
			accountMeta: map[string]any{"codex_service_tier": "fast"},
			wantTier:    "priority",
			wantPresent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, _, err := codexResponsesPayload(contract.ConversationRequest{
				SourceProtocol: "openai-compatible",
				SourceEndpoint: "/v1/responses",
				RawBody:        tt.rawPayload,
				Account:        accountcontract.ProviderAccount{Metadata: tt.accountMeta},
				Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
			})
			if err != nil {
				t.Fatalf("build codex responses payload: %v", err)
			}
			gotTier, ok := payload["service_tier"]
			if ok != tt.wantPresent {
				t.Fatalf("service_tier presence = %v, want %v in %+v", ok, tt.wantPresent, payload)
			}
			if tt.wantPresent && gotTier != tt.wantTier {
				t.Fatalf("service_tier = %v, want %s", gotTier, tt.wantTier)
			}
		})
	}
}
