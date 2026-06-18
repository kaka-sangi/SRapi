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

// codexRewriteRawForNonStreamingCompact ensures the compact response body
// has a top-level `text` field — the contract Hermes' Rust /compact
// parser enforces. Handles both shapes the upstream Codex backend
// actually returns:
//
//  1. SSE body (when upstream ignores the request's stream=false and ships
//     SSE anyway). Extracts the terminal response.completed/done event's
//     `response` object, then ensures `text` via delta accumulation or
//     parsed-fallback. Falls back to synthesis from parsed when no
//     terminal event is present.
//
//  2. JSON body (the path the sub2api transform pushes the upstream
//     toward via Accept: application/json — line 4184 of
//     openai_gateway_service.go). If the JSON already has a non-empty
//     `text` field at the top level, it's a no-op. If not, the
//     normalizer extracts the text from `output_text`, from
//     `output[].content[].text` (the nested compact shape), or from
//     parsed.Parts and injects it at the top level. This is the column-260
//     regression: upstream returns JSON without top-level text, the old
//     code passed it through, Hermes 400'd.
//
// Returns (rewritten, true) when any path produced a body different from
// the input; otherwise (input, false). The caller's warning emit logs the
// upstream body excerpt on every rewrite so the operator can correlate.
func codexRewriteRawForNonStreamingCompact(body []byte, parsed contract.ConversationResponse) ([]byte, bool) {
	if codexBodyLooksLikeSSE(body) {
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
	return codexEnsureCompactJSONBodyHasText(body, parsed)
}

// codexEnsureCompactJSONBodyHasText injects a top-level `text` field into
// the upstream's JSON compact response when one is missing. The text is
// pulled, in order:
//
//  1. The existing top-level `output_text` convenience field (some
//     upstream variants ship it).
//  2. The nested output[].content[].text — the canonical Responses shape
//     where structured output items carry text under content blocks.
//  3. The parsed ConversationResponse's Parts text projection (last
//     resort: the SRapi parser already accumulated it).
//
// No-op when the body already has a non-empty top-level `text`. Returns
// (input, false) when nothing recoverable was found — the caller may want
// to surface a 502 in that case, but the current path falls through. See
// codexCompactRewriteWarning for the body-excerpt log the caller emits
// on every rewrite (success or no-op).
func codexEnsureCompactJSONBodyHasText(body []byte, parsed contract.ConversationResponse) ([]byte, bool) {
	var current map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(body), &current); err != nil {
		return body, false
	}
	if textValue, ok := current["text"].(string); ok && strings.TrimSpace(textValue) != "" {
		return body, false
	}
	text := strings.TrimSpace(codexStringValue(current["output_text"]))
	if text == "" {
		text = codexExtractTextFromOutputArray(current["output"])
	}
	if strings.TrimSpace(text) == "" {
		text = contentPartsText(parsed.Parts)
	}
	if strings.TrimSpace(text) == "" {
		return body, false
	}
	current["text"] = text
	rewritten, err := json.Marshal(current)
	if err != nil {
		return body, false
	}
	return rewritten, true
}

// codexExtractTextFromOutputArray walks a Responses-shaped output array
// and joins every `text` field on output_text / refusal content blocks.
// The canonical shape from the upstream looks like
//
//	"output": [
//	    {"type":"reasoning","summary":[...], ...},
//	    {"type":"message","role":"assistant","content":[
//	        {"type":"output_text","text":"..."},
//	        ...
//	    ]}
//	]
//
// Mirrors sub2api's reconstructResponseOutputFromSSE intent (rebuild
// what's missing at the top level), only here we walk the JSON we already
// have instead of re-scanning SSE deltas.
func codexExtractTextFromOutputArray(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	var collected strings.Builder
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch codexStringValue(item["type"]) {
		case "message", "":
			content, ok := item["content"].([]any)
			if !ok {
				continue
			}
			for _, rawPart := range content {
				part, ok := rawPart.(map[string]any)
				if !ok {
					continue
				}
				partType := codexStringValue(part["type"])
				if partType != "output_text" && partType != "refusal" && partType != "input_text" && partType != "" {
					continue
				}
				if text, ok := part["text"].(string); ok && text != "" {
					collected.WriteString(text)
				} else if refusal, ok := part["refusal"].(string); ok && refusal != "" {
					collected.WriteString(refusal)
				}
			}
		case "output_text":
			if text, ok := item["text"].(string); ok && text != "" {
				collected.WriteString(text)
			}
		}
	}
	return collected.String()
}
