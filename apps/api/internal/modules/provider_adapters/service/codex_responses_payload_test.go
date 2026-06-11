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

func TestCodexResponsesPayloadNormalizesKnownModelAliases(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
		want     string
	}{
		{name: "provider prefix and compact suffix", upstream: "openai/gpt5.4mini-openai-compact", want: "gpt-5.4-mini"},
		{name: "removed model fallback", upstream: "gpt-5.1", want: "gpt-5.4"},
		{name: "codex spark effort suffix", upstream: "gpt-5.3-codex-spark-high", want: "gpt-5.3-codex-spark"},
		{name: "codex mini alias", upstream: "codex-mini-latest", want: "gpt-5.3-codex"},
		{name: "custom model passes through", upstream: " custom-codex-model ", want: "custom-codex-model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, _, err := codexResponsesPayload(contract.ConversationRequest{
				SourceProtocol: "openai-compatible",
				SourceEndpoint: "/v1/responses",
				RawBody:        []byte(`{"model":"caller-model","input":"hello"}`),
				Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: tt.upstream},
			})
			if err != nil {
				t.Fatalf("build codex responses payload: %v", err)
			}
			if payload["model"] != tt.want {
				t.Fatalf("model = %v, want %s", payload["model"], tt.want)
			}
		})
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

func TestCodexResponsesPayloadNormalizesBuiltinToolAliases(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"codex-local",
			"input":"find latest model news",
			"tools":[
				{"type":"web_search_preview_2025_03_11"}
			],
			"tool_choice":{
				"type":"allowed_tools",
				"tools":[
					{"type":"web_search_preview"},
					{"type":"web_search_preview_2025_03_11"}
				]
			}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex responses payload: %v", err)
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool, got %+v", payload["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "web_search" {
		t.Fatalf("tools.0.type = %v, want web_search", tool["type"])
	}
	choice, ok := payload["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "allowed_tools" {
		t.Fatalf("expected allowed_tools tool_choice, got %+v", payload["tool_choice"])
	}
	choiceTools, ok := choice["tools"].([]any)
	if !ok || len(choiceTools) != 2 {
		t.Fatalf("expected two allowed tools, got %+v", choice["tools"])
	}
	for i, rawTool := range choiceTools {
		choiceTool, _ := rawTool.(map[string]any)
		if choiceTool["type"] != "web_search" {
			t.Fatalf("tool_choice.tools.%d.type = %v, want web_search", i, choiceTool["type"])
		}
	}
}

func TestCodexResponsesPayloadNormalizesTopLevelBuiltinToolChoiceAlias(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"codex-local",
			"input":"find latest model news",
			"tool_choice":{"type":"web_search_preview_2025_03_11"}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex responses payload: %v", err)
	}
	choice, ok := payload["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "web_search" {
		t.Fatalf("expected web_search tool_choice, got %+v", payload["tool_choice"])
	}
}

func TestCodexResponsesPayloadAddsCodexResponseRuntimeDefaults(t *testing.T) {
	payload, stream, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"codex-local",
			"input":"hello",
			"parallel_tool_calls":false,
			"include":["file_search_call.results"]
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex responses payload: %v", err)
	}
	if !stream {
		t.Fatal("codex responses payload should stream")
	}
	if payload["parallel_tool_calls"] != true {
		t.Fatalf("parallel_tool_calls = %v, want true", payload["parallel_tool_calls"])
	}
	if payload["store"] != false || payload["stream"] != true {
		t.Fatalf("expected stream=true and store=false, got %+v", payload)
	}
	include, ok := payload["include"].([]any)
	if !ok {
		t.Fatalf("include = %T(%v), want []any", payload["include"], payload["include"])
	}
	if !containsStringAny(include, "file_search_call.results") || !containsStringAny(include, codexResponsesEncryptedReasoningInclude) {
		t.Fatalf("include did not preserve existing values and add encrypted reasoning: %+v", include)
	}
}

func TestCodexResponsesCompactPayloadSkipsResponseRuntimeDefaults(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses/compact",
		RawBody: []byte(`{
			"model":"codex-local",
			"input":"hello"
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex compact payload: %v", err)
	}
	for _, field := range []string{"parallel_tool_calls", "include", "stream", "store"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("compact payload should not inject %s: %+v", field, payload)
		}
	}
}

func containsStringAny(values []any, want string) bool {
	for _, value := range values {
		if text, ok := value.(string); ok && text == want {
			return true
		}
	}
	return false
}
