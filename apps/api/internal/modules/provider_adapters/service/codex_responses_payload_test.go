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

func TestCodexResponsesCompactPayloadMatchesSub2API(t *testing.T) {
	// Verbatim port of sub2api applyCodexOAuthTransform
	// (openai_codex_transform.go:131-165): compact is DELETE-ONLY. The
	// transform strips store, stream, and the openAICodexOAuthUnsupportedFields
	// list (user, metadata, prompt_cache_retention, safety_identifier,
	// stream_options, max_output_tokens, max_completion_tokens, temperature,
	// top_p, frequency_penalty, presence_penalty). It explicitly DOES NOT add
	// include, parallel_tool_calls, or default model instructions for compact —
	// the comment at openai_codex_transform.go:161-162 reads "compact 端点形态
	// 不同，单独处理，此处跳过 ensureCodexReasoningInclude".
	//
	// Earlier srapi tried to verbatim-port CLIProxyAPI's translator (which
	// does add those fields), but Codex /v1/responses/compact actually
	// rejects them with {"error":{"code":"unknown_parameter","param":"include"}}.
	// Diagnosed live against srapi.senran.net production traffic. This test
	// pins the sub2api-aligned compact shape so future regressions are
	// caught immediately.
	payload, stream, err := codexResponsesPayload(contract.ConversationRequest{
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
	// Compact MUST NOT carry any of these — sub2api strips/skips them all.
	for _, field := range []string{
		"store",
		"stream",
		"include",
		"parallel_tool_calls",
		"client_metadata",
	} {
		if _, ok := payload[field]; ok {
			t.Fatalf("compact must NOT carry %q (sub2api parity, Codex /compact rejects), got payload=%+v", field, payload)
		}
	}
	// Live Codex /compact rejects an absent instructions field with
	// {"detail":"Instructions are required"}. Keep the field explicit but
	// empty; never inject the non-compact model base prompt.
	if payload["instructions"] != "" {
		t.Fatalf("compact must carry explicit empty instructions, got %+v", payload)
	}
	// stream return MUST be false for compact even though payload["stream"]
	// was deleted. The historical codexResponsesPayloadStream returns true
	// when the field is absent — without the explicit compact override in
	// codexResponsesPayload, the codex adapter would set Accept:
	// text/event-stream + ExpectStream:true on the upstream request,
	// surfacing on the client as "stream disconnected before completion:
	// missing field 'text' at line 1 column 203". Pin the override so a
	// future refactor of codexResponsesPayloadStream cannot reintroduce
	// the streaming-route regression.
	if stream {
		t.Fatalf("compact must return stream=false even when payload[\"stream\"] is absent, got stream=true")
	}
}

func TestCodexResponsesCompactPayloadStripsClientMetadataEvenWhenRequestSettingsSetIt(t *testing.T) {
	// Regression: codexApplyClientMetadataSettings populates client_metadata
	// from request settings (x-codex-installation-id, x-codex-turn-metadata,
	// x-codex-window-id, beta-features). For /compact the upstream rejects
	// the field outright (live srapi.senran.net diagnosis returned
	// {"code":"unknown_parameter","param":"client_metadata"}), so the
	// final payload normalizer must drop it after settings populated it.
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses/compact",
		RawBody:        []byte(`{"model":"codex-local","input":"hello"}`),
		RequestSettings: map[string]any{
			"codex_installation_id": "install-xyz",
			"codex_window_id":       "window-xyz",
			"codex_turn_metadata":   `{"prompt_cache_key":"pck-xyz"}`,
		},
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex compact payload with client_metadata settings: %v", err)
	}
	if _, ok := payload["client_metadata"]; ok {
		t.Fatalf("client_metadata must be stripped for compact even when settings populated it, got %+v", payload)
	}
}

func TestCodexResponsesPayloadStrandedSystemRoleBecomesDeveloper(t *testing.T) {
	// Two-layer defence against the Codex upstream rejecting role="system"
	// in the input array (CLIProxyAPI translator
	// codex_openai-responses_request.go:65-86 convertSystemRoleToDeveloper):
	//   1. The common case — system items with textual content — is handled
	//      by codexLiftInstructionInputItems which lifts the text into the
	//      top-level "instructions" field and removes the system item from
	//      input entirely (see TestCodexResponsesPayload* / sub2api parity).
	//   2. The edge case here — a system item whose content the lifter
	//      cannot extract into plain text (e.g. only image attachments) —
	//      is still removed from input by the same lift in srapi today,
	//      but the input-item normalizer also rewrites role=system to
	//      role=developer as a defence in depth, mirroring CLIProxyAPI.
	//      Exercise it directly so future lifter changes don't silently
	//      reintroduce a role=system message into the outbound payload.
	for _, endpoint := range []string{"/v1/responses", "/v1/responses/compact"} {
		t.Run(endpoint, func(t *testing.T) {
			normalized := codexNormalizeResponsesInputItem(map[string]any{
				"role":    "system",
				"content": []any{map[string]any{"type": "input_text", "text": "you are helpful"}},
			})
			object, ok := normalized.(map[string]any)
			if !ok {
				t.Fatalf("expected map, got %T(%v)", normalized, normalized)
			}
			if got := codexStringValue(object["role"]); got != "developer" {
				t.Fatalf("[%s] expected role=developer, got %q", endpoint, got)
			}
		})
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

func TestCodexBaseInstructionsForModel(t *testing.T) {
	// The embedded Codex CLI base prompts must load — an empty `instructions`
	// field is rejected by the upstream Codex backend.
	if codexBaseInstructions == "" || codexInstructionsGPT51 == "" || codexInstructionsGPT52 == "" {
		t.Fatal("embedded codex base instructions must not be empty")
	}

	cases := []struct {
		model string
		want  string
	}{
		{"gpt-5.3-codex", codexBaseInstructions},
		{"codex-upstream", codexBaseInstructions},
		{"gpt-5-codex", codexBaseInstructions},
		{"gpt-5.2-codex", codexBaseInstructions}, // codex match takes precedence
		{"gpt-5.5", codexInstructionsGPT51},      // the model that was being rejected
		{"GPT-5.5", codexInstructionsGPT51},      // case-insensitive
		{"gpt-5", codexInstructionsGPT51},
		{"gpt-5.1", codexInstructionsGPT51},
		{"gpt-5.2", codexInstructionsGPT52},
		{"", codexBaseInstructions},
	}
	for _, tc := range cases {
		if got := codexBaseInstructionsForModel(tc.model); got != tc.want {
			t.Errorf("codexBaseInstructionsForModel(%q) returned the wrong base prompt", tc.model)
		}
	}
}
