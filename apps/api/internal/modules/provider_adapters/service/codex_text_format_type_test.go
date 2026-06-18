package service

import (
	"testing"

	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestCodexResponsesPayloadEnsuresTextFormatTypeFromJSONSchemaWrapper
// pins the fix for the live production rejection on /v1/responses
// (req_c480b448... — provider 25, account 238, model gpt-5.5):
//
//	{"error":{"message":"Missing required parameter: 'text.format.type'.",
//	          "type":"invalid_request_error","param":"text.format.type",
//	          "code":"missing_required_parameter"}}
//
// Shape: caller forwards a chat-completions-style response_format
// wrapper as text.format = {json_schema:{name,strict,schema}}. The
// Codex Responses upstream requires inline `type:"json_schema"` plus
// the lifted name/strict/schema. CLIProxyAPI's chat-completions
// translator already does this lift; the same lift must happen on the
// /responses path so the existing input shape is accepted there too.
func TestCodexResponsesPayloadEnsuresTextFormatTypeFromJSONSchemaWrapper(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{
				"format":{
					"json_schema":{
						"name":"weather",
						"strict":true,
						"schema":{"type":"object","properties":{"city":{"type":"string"}}}
					}
				}
			}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text map, got %T(%v)", payload["text"], payload["text"])
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("expected text.format map, got %T(%v)", text["format"], text["format"])
	}
	if got := format["type"]; got != "json_schema" {
		t.Fatalf("expected text.format.type=json_schema, got %v", got)
	}
	if got := format["name"]; got != "weather" {
		t.Fatalf("expected text.format.name lifted from wrapper, got %v", got)
	}
	if got := format["strict"]; got != true {
		t.Fatalf("expected text.format.strict lifted from wrapper, got %v", got)
	}
	if _, ok := format["schema"].(map[string]any); !ok {
		t.Fatalf("expected text.format.schema lifted from wrapper, got %T(%v)", format["schema"], format["schema"])
	}
	if _, ok := format["json_schema"]; ok {
		t.Fatalf("expected text.format.json_schema wrapper removed after lift, got %+v", format)
	}
}

// TestCodexResponsesPayloadEnsuresTextFormatTypeFromBareSchema covers a
// related miscoded shape: text.format has a bare `schema` (no `type`,
// no `json_schema` wrapper). Common when a third-party client emits
// the new Responses-native format but forgets the `type` field.
// Codex still rejects with the same "Missing required parameter"
// error. Infer json_schema from the presence of a schema field.
func TestCodexResponsesPayloadEnsuresTextFormatTypeFromBareSchema(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"format":{"schema":{"type":"object"}}}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if got := format["type"]; got != "json_schema" {
		t.Fatalf("expected text.format.type=json_schema (inferred from schema), got %v", got)
	}
	if _, ok := format["schema"].(map[string]any); !ok {
		t.Fatalf("expected text.format.schema preserved, got %+v", format)
	}
}

// TestCodexResponsesPayloadEnsuresTextFormatTypeFallsBackToText covers
// the catch-all: text.format exists but neither json_schema wrapper
// nor a schema is present — only an unrelated field. Rather than
// let the request 400 at the upstream, default to "text" (the upstream
// default). Unknown fields are STRIPPED by the whitelist — keeping
// them would re-trigger the live "Unknown parameter" rejection
// (req_ae057a05... — text.format.verbosity). See
// codexStripUnknownTextFormatFields.
func TestCodexResponsesPayloadEnsuresTextFormatTypeFallsBackToText(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"format":{"custom_extension":"foo"}}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if got := format["type"]; got != "text" {
		t.Fatalf("expected text.format.type=text (fallback), got %v", got)
	}
	if _, leaked := format["custom_extension"]; leaked {
		t.Fatalf("unknown text.format fields must be stripped to avoid upstream 'Unknown parameter', got %+v", format)
	}
}

// TestCodexResponsesPayloadLeavesValidTextFormatAlone guards against
// the normalizer accidentally rewriting a payload that was already
// correctly shaped. A caller that sent text.format.type="json_schema"
// + name/strict/schema inline (the canonical Responses-native shape)
// must reach the upstream byte-for-byte unchanged.
func TestCodexResponsesPayloadLeavesValidTextFormatAlone(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{
				"format":{
					"type":"json_schema",
					"name":"weather",
					"strict":true,
					"schema":{"type":"object"}
				}
			}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if got := format["type"]; got != "json_schema" {
		t.Fatalf("expected text.format.type preserved, got %v", got)
	}
	if got := format["name"]; got != "weather" {
		t.Fatalf("expected text.format.name preserved, got %v", got)
	}
	if got := format["strict"]; got != true {
		t.Fatalf("expected text.format.strict preserved, got %v", got)
	}
	if _, ok := format["json_schema"]; ok {
		t.Fatalf("normalizer must not add a json_schema wrapper to an already-correct format, got %+v", format)
	}
}

// TestCodexResponsesPayloadHoistsVerbosityFromTextFormat pins the fix for
// the live production rejection req_ae057a05... (provider 25, account
// 271, model gpt-5.5):
//
//	{"error":{"message":"Unknown parameter: 'text.format.verbosity'.",
//	          "type":"invalid_request_error",
//	          "param":"text.format.verbosity",
//	          "code":"unknown_parameter"}}
//
// The upstream Codex /responses contract puts verbosity at `text.verbosity`,
// not inside `text.format`. A caller that placed it under format must
// have the field hoisted up before forwarding so the legitimate setting
// reaches the upstream at the correct path AND the unknown field at the
// wrong path doesn't 400 the request.
func TestCodexResponsesPayloadHoistsVerbosityFromTextFormat(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"format":{"type":"text","verbosity":"high"}}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if got := text["verbosity"]; got != "high" {
		t.Fatalf("expected text.verbosity=high after hoist, got %v", got)
	}
	if _, leaked := format["verbosity"]; leaked {
		t.Fatalf("text.format.verbosity must be deleted after hoist, got %+v", format)
	}
}

// TestCodexResponsesPayloadDoesNotOverwriteExistingTextVerbosity guards
// against the hoist clobbering a verbosity the caller already set
// correctly at the text level. The misplaced format-level verbosity
// must be dropped, but the legitimate top-level value wins.
func TestCodexResponsesPayloadDoesNotOverwriteExistingTextVerbosity(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"verbosity":"low","format":{"type":"text","verbosity":"high"}}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if got := text["verbosity"]; got != "low" {
		t.Fatalf("existing text.verbosity must win, got %v", got)
	}
	if _, leaked := format["verbosity"]; leaked {
		t.Fatalf("text.format.verbosity must be deleted, got %+v", format)
	}
}

// TestCodexResponsesPayloadStripsUnknownTextFormatFields covers any other
// stray key callers tuck under text.format. The upstream
// /responses contract whitelists only {type, name, strict, schema,
// description}; everything else is rejected with unknown_parameter.
// The normalizer must strip them after lifting/inference.
func TestCodexResponsesPayloadStripsUnknownTextFormatFields(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"format":{
				"type":"json_schema",
				"name":"weather",
				"strict":true,
				"schema":{"type":"object"},
				"description":"Weather schema.",
				"some_third_party_extension":"foo",
				"another_unknown":42
			}}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	// Allowed fields survived.
	for _, key := range []string{"type", "name", "strict", "schema", "description"} {
		if _, ok := format[key]; !ok {
			t.Fatalf("expected allow-listed field %q on text.format, got %+v", key, format)
		}
	}
	// Disallowed fields stripped.
	for _, key := range []string{"some_third_party_extension", "another_unknown"} {
		if _, ok := format[key]; ok {
			t.Fatalf("expected text.format.%s stripped, got %+v", key, format)
		}
	}
}

// TestCodexResponsesPayloadStripsJSONSchemaWrapperAfterLift makes sure
// the chat-completions response_format wrapper is fully unwrapped — not
// just the {name, strict, schema, description} subset, but also any
// other key the wrapper smuggled in. Defence in depth: even if a future
// caller adds a `xyz` key inside json_schema, the strict whitelist below
// catches it.
func TestCodexResponsesPayloadStripsJSONSchemaWrapperAfterLift(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"format":{
				"type":"json_schema",
				"json_schema":{
					"name":"weather",
					"strict":true,
					"schema":{"type":"object"},
					"description":"desc",
					"future_field":"x"
				}
			}}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if _, ok := format["json_schema"]; ok {
		t.Fatalf("json_schema wrapper must be removed after lift, got %+v", format)
	}
	if got := format["description"]; got != "desc" {
		t.Fatalf("expected description lifted from wrapper, got %v", got)
	}
	if _, ok := format["future_field"]; ok {
		t.Fatalf("expected unknown wrapper field stripped by whitelist, got %+v", format)
	}
}

// TestCodexResponsesPayloadDropsVerbosityOnGPT52 covers the sub2api
// SupportsVerbosity parity gate (openai_gateway_service.go:2631-2633).
// The `text.verbosity` field was introduced in gpt-5.3 and the upstream
// rejects it on older generations with
//
//	{"error":{"message":"Unknown parameter: 'text.verbosity'.", ...
//	          "param":"text.verbosity","code":"unknown_parameter"}}
//
// Without this gate, the additive `verbosity:"medium"` default we add
// in codexApplyResponsesPayloadDefaults would 400 every gpt-5.2 call.
func TestCodexResponsesPayloadDropsVerbosityOnGPT52(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody:        []byte(`{"model":"gpt-5.2","input":"hello","text":{"verbosity":"high"}}`),
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.2"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if text, ok := payload["text"].(map[string]any); ok {
		if _, leaked := text["verbosity"]; leaked {
			t.Fatalf("text.verbosity must be deleted for gpt-5.2 (does not support verbosity), got %+v", text)
		}
	}
}

// TestCodexResponsesPayloadKeepsVerbosityOnGPT55 sanity-checks the
// opposite direction: gpt-5.5 supports verbosity, so the additive
// default (or caller-supplied value) must survive normalization.
func TestCodexResponsesPayloadKeepsVerbosityOnGPT55(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody:        []byte(`{"model":"gpt-5.5","input":"hello","text":{"verbosity":"high"}}`),
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text map, got %T(%v)", payload["text"], payload["text"])
	}
	if got := text["verbosity"]; got != "high" {
		t.Fatalf("text.verbosity must survive on gpt-5.5, got %v", got)
	}
}

// TestCodexResponsesPayloadDropsTextEntirelyWhenOnlyVerbosityOnGPT52
// pins the "empty text object" cleanup: if verbosity was the only key
// under `text`, dropping it leaves the object empty — also forbidden
// by some older upstream contracts. Drop the whole `text` to play it
// safe.
func TestCodexResponsesPayloadDropsTextEntirelyWhenOnlyVerbosityOnGPT52(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody:        []byte(`{"model":"gpt-5.2","input":"hello"}`),
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.2"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if _, hasText := payload["text"]; hasText {
		t.Fatalf("text must be dropped entirely after verbosity strip on gpt-5.2 (no other keys), got %+v", payload["text"])
	}
}

// TestCodexResponsesPayloadLeavesTextWithoutFormatAlone makes sure the
// fix is scoped: when the caller sends only `text.verbosity` (no
// `format`), the helper must not synthesize a format object — the
// upstream accepts a text object that contains only verbosity.
func TestCodexResponsesPayloadLeavesTextWithoutFormatAlone(t *testing.T) {
	payload, _, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"gpt-5.5",
			"input":"hello",
			"text":{"verbosity":"high"}
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	text, _ := payload["text"].(map[string]any)
	if _, hasFormat := text["format"]; hasFormat {
		t.Fatalf("must not synthesize text.format when caller did not send one, got %+v", text)
	}
	if got := text["verbosity"]; got != "high" {
		t.Fatalf("expected verbosity preserved, got %v", got)
	}
}
