package service

import (
	"bytes"
	"encoding/json"
	"strings"
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

// codexRewriteRawForNonStreamingCompact converts SSE bytes to the terminal
// `response` JSON object when the upstream returned SSE for a request the
// adapter declared as non-streaming (i.e. /v1/responses/compact). Returns
// the rewritten body and true when a rewrite happened; otherwise returns
// the input unchanged and false. The rewrite is a no-op when the body is
// already JSON or when the SSE has no terminal event — callers should keep
// the original body in both cases so failure modes surface unchanged.
func codexRewriteRawForNonStreamingCompact(body []byte) ([]byte, bool) {
	if !codexBodyLooksLikeSSE(body) {
		return body, false
	}
	extracted, ok := codexExtractTerminalResponseJSON(body)
	if !ok {
		return body, false
	}
	return extracted, true
}
