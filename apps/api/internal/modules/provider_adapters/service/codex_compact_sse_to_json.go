package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// codexBodyLooksLikeSSE reports whether body is a raw text/event-stream payload
// (i.e. starts with "data:" or contains "\ndata:"). Mirrors the gateway-side
// looksLikeSSE check but lives in the adapter so the codex compact path can
// rewrite Raw before it reaches the gateway raw-passthrough writer.
func codexBodyLooksLikeSSE(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	return bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:"))
}

// codexExtractTerminalResponseJSON scans an SSE body for the terminal
// response.completed / response.done event and returns its `response` field
// as JSON bytes. Ported from sub2api openai_gateway_service.go:5329
// extractCodexFinalResponse — used when the codex adapter requested a
// non-streaming /v1/responses/compact but the upstream returned SSE anyway
// (Codex backend ignores body stream=false on the compact endpoint). Without
// this rewrite, the gateway raw-passthrough path would emit raw SSE bytes
// with Content-Type: application/json, surfacing on Hermes (Codex CLI in
// Rust) as "Error running remote compact task: stream disconnected before
// completion: missing field `text` at line 1 column 203".
func codexExtractTerminalResponseJSON(body []byte) ([]byte, bool) {
	frames, err := parseSSEFrames(body)
	if err != nil || len(frames) == 0 {
		return nil, false
	}
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		eventType := strings.TrimSpace(payload.Type)
		if eventType == "" {
			eventType = strings.TrimSpace(frame.Event)
		}
		switch eventType {
		case "response.completed", "response.done":
			if response := bytes.TrimSpace(payload.Response); len(response) > 0 {
				return append([]byte(nil), response...), true
			}
		}
	}
	return nil, false
}

// codexAccumulateOutputTextFromSSE walks an SSE body and pieces together the
// final output text by joining every response.output_text.delta event's
// `delta` field in order, falling back to the last response.output_text.done
// event's `text` field if no deltas were present.
//
// Hermes' /v1/responses/compact parser requires a top-level `text` field on
// the rewritten body (the compact endpoint's whole purpose is to produce a
// summary string). The upstream Codex backend ships this string via SSE
// delta events for the model's reasoning/summary stream and DOES NOT
// always include it in the terminal response.completed event's `response`
// object — so extracting the terminal event alone leaves a body without
// the required `text` field, surfacing on Hermes as "missing field `text`
// at line 1 column 260" (live regression after the first compact fix in
// 427771fd that only handled the terminal-event case).
func codexAccumulateOutputTextFromSSE(body []byte) string {
	frames, err := parseSSEFrames(body)
	if err != nil || len(frames) == 0 {
		return ""
	}
	var deltas strings.Builder
	completed := ""
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Text  string `json:"text"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		eventType := strings.TrimSpace(payload.Type)
		if eventType == "" {
			eventType = strings.TrimSpace(frame.Event)
		}
		switch eventType {
		case "response.output_text.delta", "response.reasoning_summary_text.delta":
			deltas.WriteString(payload.Delta)
		case "response.output_text.done", "response.reasoning_summary_text.done":
			if strings.TrimSpace(payload.Text) != "" {
				completed = payload.Text
			}
		}
	}
	if completed != "" {
		return completed
	}
	return deltas.String()
}

// codexEnsureCompactResponseText guarantees the rewritten compact body has
// a top-level `text` field so Hermes' Rust parser ("missing field `text`")
// can decode it. Tries, in order:
//
//  1. The extracted terminal event's `response.text` if non-empty.
//  2. Output-text accumulated from response.output_text.delta events.
//  3. The parsed ConversationResponse's text projection (last-ditch — comes
//     from the SSE parser that already ran, so it covers cases where the
//     raw byte scanner missed something).
//
// The injection is a no-op when the body already has a `text` field with a
// non-empty value.
func codexEnsureCompactResponseText(body []byte, sseBody []byte, fallback contract.ConversationResponse) []byte {
	if len(bytes.TrimSpace(body)) == 0 {
		return body
	}
	var current map[string]any
	if err := json.Unmarshal(body, &current); err != nil {
		// Not a JSON object — leave the caller's body alone. The fallback
		// path in the caller will synthesize a minimal compact-shaped body
		// when no rewrite succeeded.
		return body
	}
	if textValue, ok := current["text"].(string); ok && strings.TrimSpace(textValue) != "" {
		return body
	}
	text := codexAccumulateOutputTextFromSSE(sseBody)
	if strings.TrimSpace(text) == "" {
		text = contentPartsText(fallback.Parts)
	}
	if strings.TrimSpace(text) == "" {
		// Nothing to inject. Leave the body alone — the caller's last-resort
		// path will synthesize a minimal compact body from the parsed
		// Usage/StopReason fields.
		return body
	}
	current["text"] = text
	rewritten, err := json.Marshal(current)
	if err != nil {
		return body
	}
	return rewritten
}

// codexSynthesizeCompactResponseFromParsed builds a minimal compact-shaped
// JSON body from the parsed ConversationResponse when SSE extraction
// failed AND the SSE body has no recoverable terminal event. The shape
// matches what Hermes' /compact parser expects:
//
//	{
//	  "id": "<from parsed>",
//	  "object": "response.compaction",
//	  "text": "<accumulated from parts>",
//	  "input_tokens": N,
//	  "output_tokens": M
//	}
//
// Used as a last-ditch fallback so the gateway never serves SSE bytes
// under Content-Type: application/json — that ALWAYS surfaces on Hermes
// as "missing field `text`" no matter which column number the parser hits.
func codexSynthesizeCompactResponseFromParsed(parsed contract.ConversationResponse) ([]byte, bool) {
	text := contentPartsText(parsed.Parts)
	if strings.TrimSpace(text) == "" {
		return nil, false
	}
	body := map[string]any{
		"object": "response.compaction",
		"text":   text,
	}
	if id := strings.TrimSpace(parsed.ID); id != "" {
		body["id"] = id
	}
	if parsed.Usage.InputTokens > 0 {
		body["input_tokens"] = parsed.Usage.InputTokens
	}
	if parsed.Usage.OutputTokens > 0 {
		body["output_tokens"] = parsed.Usage.OutputTokens
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// codexRewriteRawForNonStreamingCompact converts SSE bytes to a JSON body
// suitable for Hermes' /v1/responses/compact parser when the upstream
// returned SSE despite the request asking for stream=false. The rewrite is
// best-effort and layered:
//
//  1. Extract the terminal response.completed / response.done event's
//     `response` field — happy path; matches sub2api
//     handlePassthroughSSEToJSON (openai_gateway_service.go:4007).
//
//  2. Inject a top-level `text` field if step 1 produced a JSON body
//     without one (the upstream is allowed to ship the text via
//     response.output_text.delta events instead of inline on the
//     terminal event). Closes the live regression
//     "missing field `text` at line 1 column 260".
//
//  3. If steps 1+2 failed (no terminal event in SSE), synthesize a minimal
//     compact-shaped body from the parsed ConversationResponse — the SSE
//     parser already ran and we have parts/usage/id from it.
//
//  4. If even synthesis fails (no recoverable text anywhere), return the
//     original body and (false). The caller MUST decide whether to surface
//     a 502 in that case rather than serve unparseable bytes — but the
//     current caller path falls through to send the original Raw, which
//     was the original bug. A follow-up should add a defensive 502 here.
//
// Returns (rewritten, true) when any of steps 1-3 produced a parseable
// JSON body different from the input; otherwise returns (input, false).
func codexRewriteRawForNonStreamingCompact(body []byte, parsed contract.ConversationResponse) ([]byte, bool) {
	if !codexBodyLooksLikeSSE(body) {
		return body, false
	}
	extracted, ok := codexExtractTerminalResponseJSON(body)
	if ok {
		rewritten := codexEnsureCompactResponseText(extracted, body, parsed)
		return rewritten, true
	}
	if synthesized, ok := codexSynthesizeCompactResponseFromParsed(parsed); ok {
		return synthesized, true
	}
	return body, false
}
