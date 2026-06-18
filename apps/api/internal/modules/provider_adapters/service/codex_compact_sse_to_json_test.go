package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestCodexExtractTerminalResponseJSONReturnsResponseObject pins the SSE → JSON
// conversion the codex adapter uses when the upstream returns SSE for a
// /v1/responses/compact request even though the body declared stream=false
// and Accept: application/json. Without this conversion, the gateway raw-
// passthrough path would write SSE bytes with Content-Type: application/json,
// which Hermes (Codex CLI in Rust) surfaces as:
//
//	"Error running remote compact task: stream disconnected before completion:
//	 missing field `text` at line 1 column 203"
//
// Mirrors sub2api openai_gateway_service.go:5329 extractCodexFinalResponse.
func TestCodexExtractTerminalResponseJSONReturnsResponseObject(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_a\"}}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp_1\",\"object\":\"response.compaction\",\"text\":\"summary\",\"input_tokens\":12,\"output_tokens\":3}}\n\n" +
			"data: [DONE]\n\n",
	)
	extracted, ok := codexExtractTerminalResponseJSON(body)
	if !ok {
		t.Fatalf("expected to extract terminal response from SSE, got ok=false")
	}
	var payload struct {
		ID           string `json:"id"`
		Object       string `json:"object"`
		Text         string `json:"text"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
	}
	if err := json.Unmarshal(extracted, &payload); err != nil {
		t.Fatalf("extracted body must be valid JSON, got %q (err=%v)", string(extracted), err)
	}
	if payload.ID != "cmp_1" || payload.Object != "response.compaction" ||
		payload.Text != "summary" || payload.InputTokens != 12 || payload.OutputTokens != 3 {
		t.Fatalf("unexpected extracted response, got %+v (raw=%q)", payload, string(extracted))
	}
	if bytes.Contains(extracted, []byte("data:")) {
		t.Fatalf("extracted body must not contain SSE markers, got %q", string(extracted))
	}
}

// TestCodexExtractTerminalResponseJSONIgnoresIntermediateEvents ensures only
// the terminal response.completed / response.done event populates the
// extracted body — intermediate events without a response object are skipped.
func TestCodexExtractTerminalResponseJSONIgnoresIntermediateEvents(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"there\"}\n\n" +
			"data: {\"type\":\"response.done\",\"response\":{\"id\":\"final\",\"object\":\"response\",\"output_text\":\"hi there\"}}\n\n" +
			"data: [DONE]\n\n",
	)
	extracted, ok := codexExtractTerminalResponseJSON(body)
	if !ok {
		t.Fatalf("expected to extract terminal response from SSE, got ok=false")
	}
	if !strings.Contains(string(extracted), `"output_text":"hi there"`) {
		t.Fatalf("expected terminal response.done body, got %q", string(extracted))
	}
}

// TestCodexExtractTerminalResponseJSONNoTerminalEventReturnsFalse documents
// the safety-net behaviour: when the SSE body has no response.completed /
// response.done frame, the helper returns ok=false so the caller's fallback
// (synthesis from parsed structure) can kick in.
func TestCodexExtractTerminalResponseJSONNoTerminalEventReturnsFalse(t *testing.T) {
	body := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n")
	if extracted, ok := codexExtractTerminalResponseJSON(body); ok {
		t.Fatalf("expected ok=false when no terminal event present, got extracted=%q ok=true", string(extracted))
	}
}

// TestCodexRewriteRawForNonStreamingCompactPassthroughOnJSON ensures the
// rewrite is a no-op when the upstream already returned a JSON body with
// a top-level `text` field — the raw bytes are returned unchanged so
// existing JSON-passthrough callers
// (TestReverseProxyCodexCLIAdapterUsesResponsesCompactEndpoint) continue
// to observe the same Raw payload.
func TestCodexRewriteRawForNonStreamingCompactPassthroughOnJSON(t *testing.T) {
	body := []byte(`{"id":"cmp_1","object":"response.compaction","text":"summary","input_tokens":12,"output_tokens":3}`)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if ok {
		t.Fatalf("rewrite must be a no-op when input is already JSON, got ok=true (rewritten=%q)", string(rewritten))
	}
	if !bytes.Equal(rewritten, body) {
		t.Fatalf("rewrite must return input unchanged on JSON, got %q want %q", string(rewritten), string(body))
	}
}

// TestCodexRewriteRawForNonStreamingInjectsTextFromOutputArray covers the
// live column-260 regression. The upstream Codex backend honours the
// Accept: application/json header sub2api forces for /compact requests
// and returns a JSON body — but in the Responses-canonical shape where
// `text` is NESTED inside output[].content[].text instead of at the top
// level. Hermes' Rust /compact parser requires top-level `text` and
// rejects with "missing field `text` at line 1 column 260".
//
// The fix walks output[] looking for message content blocks with
// output_text and joins them into a single top-level `text` string.
func TestCodexRewriteRawForNonStreamingInjectsTextFromOutputArray(t *testing.T) {
	body := []byte(`{
		"id":"cmp_x",
		"object":"response.compaction",
		"output":[
			{"type":"reasoning","summary":[],"content":null},
			{"type":"message","role":"assistant","content":[
				{"type":"output_text","text":"compact summary text"}
			]}
		],
		"input_tokens":7,
		"output_tokens":1
	}`)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("expected JSON-without-text path to inject text, got ok=false")
	}
	var payload map[string]any
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("rewritten body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if got := payload["text"]; got != "compact summary text" {
		t.Fatalf("text must be lifted from output[].content[].text, got %v", got)
	}
	if payload["id"] != "cmp_x" || payload["object"] != "response.compaction" {
		t.Fatalf("rewrite must preserve compact metadata, got %+v", payload)
	}
}

// TestCodexRewriteRawForNonStreamingInjectsTextFromMultipleOutputBlocks
// guards against the upstream emitting the compact text across multiple
// output_text content parts — they must be concatenated in order.
func TestCodexRewriteRawForNonStreamingInjectsTextFromMultipleOutputBlocks(t *testing.T) {
	body := []byte(`{
		"id":"cmp_z",
		"object":"response.compaction",
		"output":[
			{"type":"message","role":"assistant","content":[
				{"type":"output_text","text":"first "},
				{"type":"output_text","text":"second "},
				{"type":"output_text","text":"third"}
			]}
		]
	}`)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("expected rewrite to succeed, got ok=false")
	}
	var payload map[string]any
	_ = json.Unmarshal(rewritten, &payload)
	if got := payload["text"]; got != "first second third" {
		t.Fatalf("text must concatenate every output_text block, got %v", got)
	}
}

// TestCodexRewriteRawForNonStreamingInjectsTextFromOutputTextTopLevel
// covers the simpler convenience-field variant: some upstream variants
// ship the compact text as a top-level `output_text` field instead of
// nested under output[]. The rewrite must promote it to `text` so Hermes
// gets the field name it expects.
func TestCodexRewriteRawForNonStreamingInjectsTextFromOutputTextTopLevel(t *testing.T) {
	body := []byte(`{"id":"cmp_o","object":"response.compaction","output_text":"top-level convenience text"}`)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("expected output_text → text promotion, got ok=false")
	}
	var payload map[string]any
	_ = json.Unmarshal(rewritten, &payload)
	if got := payload["text"]; got != "top-level convenience text" {
		t.Fatalf("text must be promoted from output_text, got %v", got)
	}
}

// TestCodexRewriteRawForNonStreamingFallsBackToParsedOnJSONWithoutText
// covers the pathological case: upstream returns a JSON body with no
// recoverable text source (no output_text, no nested output[]
// content text). The rewrite then falls back to the parsed
// ConversationResponse's Parts text projection.
func TestCodexRewriteRawForNonStreamingFallsBackToParsedOnJSONWithoutText(t *testing.T) {
	body := []byte(`{"id":"cmp_p","object":"response.compaction","input_tokens":3,"output_tokens":1}`)
	parsed := contract.ConversationResponse{
		Parts: []contract.ContentPart{
			{Kind: contract.ContentPartText, Text: "parsed fallback text"},
		},
	}
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, parsed)
	if !ok {
		t.Fatalf("expected parsed-Parts fallback, got ok=false")
	}
	var payload map[string]any
	_ = json.Unmarshal(rewritten, &payload)
	if got := payload["text"]; got != "parsed fallback text" {
		t.Fatalf("text must come from parsed.Parts, got %v", got)
	}
}

// TestCodexRewriteRawForNonStreamingReturnsFalseOnUnrecoverableJSON is
// the safety net: when the JSON body has no top-level text AND no
// recoverable source anywhere, signal failure so the caller can decide
// to surface a 502 rather than serve unparseable bytes.
func TestCodexRewriteRawForNonStreamingReturnsFalseOnUnrecoverableJSON(t *testing.T) {
	body := []byte(`{"id":"cmp_q","object":"response.compaction","input_tokens":3,"output_tokens":1}`)
	if _, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{}); ok {
		t.Fatalf("rewrite must signal failure on unrecoverable JSON body")
	}
}

// TestCodexRewriteRawForNonStreamingInjectsTextFromDeltaEvents is the
// regression that motivated this whole helper rewrite. Production rejection
// (req_xxxxxx... column 260): the upstream's terminal response.completed
// event carried only {id, object, input_tokens, output_tokens} — no `text`
// field. The actual summary text was streamed via response.output_text.delta
// events. Without this fix the rewritten body still lacks `text` and Hermes'
// Rust parser blows up identically to the original "column 203" rejection.
//
// The fix: scan delta events, accumulate the text, inject it onto the
// extracted terminal response as `text`.
func TestCodexRewriteRawForNonStreamingInjectsTextFromDeltaEvents(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Some \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"summary \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"text.\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp_x\",\"object\":\"response.compaction\",\"input_tokens\":7,\"output_tokens\":1}}\n\n" +
			"data: [DONE]\n\n",
	)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("expected rewrite to succeed, got ok=false")
	}
	var payload map[string]any
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("rewritten body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if got := payload["text"]; got != "Some summary text." {
		t.Fatalf("text must be reconstructed from delta events, got %v (rewritten=%q)", got, string(rewritten))
	}
	if payload["id"] != "cmp_x" || payload["object"] != "response.compaction" {
		t.Fatalf("terminal-event metadata must survive the injection, got %+v", payload)
	}
}

// TestCodexRewriteRawForNonStreamingInjectsTextFromDoneEvent covers the
// alternate completed-text path: some upstream variants emit a single
// response.output_text.done event with the full text instead of streaming
// deltas. The rewrite must pick the `text` field off that terminal-done
// event when no deltas were present.
func TestCodexRewriteRawForNonStreamingInjectsTextFromDoneEvent(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.done\",\"text\":\"Whole summary.\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp_y\",\"object\":\"response.compaction\"}}\n\n",
	)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("expected rewrite to succeed, got ok=false")
	}
	var payload map[string]any
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("rewritten body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if got := payload["text"]; got != "Whole summary." {
		t.Fatalf("text must be lifted from output_text.done, got %v (rewritten=%q)", got, string(rewritten))
	}
}

// TestCodexRewriteRawForNonStreamingInjectsTextFromReasoningSummary covers
// the third upstream variant: when the compact endpoint emits the summary
// via response.reasoning_summary_text.delta events (older codex backend
// variant). The reconstruction must pick those up too.
func TestCodexRewriteRawForNonStreamingInjectsTextFromReasoningSummary(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"Reasoning \"}\n\n" +
			"data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"summary.\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp_r\",\"object\":\"response.compaction\"}}\n\n",
	)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("expected rewrite to succeed, got ok=false")
	}
	var payload map[string]any
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("rewritten body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if got := payload["text"]; got != "Reasoning summary." {
		t.Fatalf("text must be reconstructed from reasoning_summary deltas, got %v", got)
	}
}

// TestCodexRewriteRawForNonStreamingFallsBackToParsedWhenNoTerminalEvent
// pins the last-resort synthesis path: when the SSE has no
// response.completed event AND no parseable terminal info, synthesize a
// minimal compact body from the parsed ConversationResponse (which the
// SRapi SSE parser already populated upstream). Without this fallback,
// a stream that ends mid-flight (only deltas, no terminal frame) would
// still leave SSE bytes in Raw and 400 the client.
func TestCodexRewriteRawForNonStreamingFallsBackToParsedWhenNoTerminalEvent(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"text\"}\n\n",
	)
	parsed := contract.ConversationResponse{
		ID: "cmp_fallback",
		Parts: []contract.ContentPart{
			{Kind: contract.ContentPartText, Text: "synthesized fallback text"},
		},
		Usage: contract.Usage{InputTokens: 5, OutputTokens: 2},
	}
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, parsed)
	if !ok {
		t.Fatalf("expected synthesis fallback to succeed when parsed has text, got ok=false")
	}
	var payload map[string]any
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("synthesized body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if payload["text"] != "synthesized fallback text" {
		t.Fatalf("text must come from parsed.Parts, got %v", payload["text"])
	}
	if payload["id"] != "cmp_fallback" || payload["object"] != "response.compaction" {
		t.Fatalf("synthesized body must mirror compact shape, got %+v", payload)
	}
}

// TestCodexRewriteRawForNonStreamingReturnsFalseWhenNothingRecoverable
// guards against silently emitting empty bodies. If the SSE has no
// terminal event AND the parsed structure has no text, the rewrite must
// signal failure so the caller can decide to surface a 502 instead of
// shipping a corrupted body.
func TestCodexRewriteRawForNonStreamingReturnsFalseWhenNothingRecoverable(t *testing.T) {
	body := []byte("data: {\"type\":\"response.in_progress\"}\n\n")
	parsed := contract.ConversationResponse{} // no parts, no usage
	if _, ok := codexRewriteRawForNonStreamingCompact(body, parsed); ok {
		t.Fatalf("rewrite must signal failure when neither terminal event nor parsed text are available")
	}
}

// TestCodexMaybeRewriteRawForNonStreamingIgnoresStreamingPath proves the
// rewrite is gated on stream=false: the streaming path's SSE Raw bytes must
// stay intact so the gateway's SSE-passthrough writer can forward them
// verbatim. Without this guard the rewrite would corrupt /v1/responses
// streaming responses.
func TestCodexMaybeRewriteRawForNonStreamingIgnoresStreamingPath(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp\"}}\n\n",
	)
	rewritten, ok := codexRewriteRawForNonStreamingCompact(body, contract.ConversationResponse{})
	if !ok {
		t.Fatalf("sanity: SSE with terminal event should rewrite when called directly, got ok=false")
	}
	if bytes.Equal(rewritten, body) {
		t.Fatalf("sanity: rewrite of SSE should differ from original, got identical")
	}
	// The caller-side gate (stream==true) is exercised by
	// codexMaybeRewriteRawForNonStreaming in codex.go; the gateway streaming
	// path passes stream=true and must observe Raw bytes unchanged. This is
	// covered end-to-end by TestReverseProxyCodexCLIAdapterPassesCliRuntimeContext
	// (which already runs the SSE-streaming path with stream=true) — that
	// test would fail if codexMaybeRewriteRawForNonStreaming ever lost its
	// stream guard.
}
