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
// nor a schema is present — only some unrelated field. Rather than
// let the request 400 at the upstream, default to "text" (the upstream
// default). Preserves whatever extra fields the caller sent.
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
	if got := format["custom_extension"]; got != "foo" {
		t.Fatalf("expected unknown fields preserved, got %+v", format)
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
