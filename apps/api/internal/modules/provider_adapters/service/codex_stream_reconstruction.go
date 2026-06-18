package service

// codex_stream_reconstruction.go ports the streaming-output reconstruction and
// terminal-error classification helpers from
// CLIProxyAPI/internal/runtime/executor/codex_executor.go (see
// collectCodexOutputItemDone / patchCodexCompletedOutput /
// codexTerminalStreamErr / codexTerminalErrorIsContextLength).
//
// srapi's existing parseCodexResponsesStream already reconstructs streamed
// output items into `indexedItems`/`fallbackItems` (semantically equivalent
// to CLIProxyAPI's outputItemsByIndex/outputItemsFallback). This file adds
// the CLIProxyAPI-shaped functions for two reasons:
//   1. They are usable as a standalone post-stream patch when an upstream
//      cuts the response.completed event early — exactly the bug
//      CLIProxyAPI's patchCodexCompletedOutput was added to fix.
//   2. They expose a stable classifier (ClassifyCodexStreamTerminalError)
//      that other parts of the gateway can call without dragging the entire
//      provider-adapter response object around.
//
// All ported logic is in this file; behaviour mirrors CLIProxyAPI verbatim
// modulo Go-idiomatic adaptations (no `bytes.Buffer.Grow` micro-optimization
// when JSON marshaling is sufficient; ctx is propagated where the caller has
// one). Tests in codex_stream_reconstruction_test.go pin the same assertions
// as CLIProxyAPI's codex_executor_retry_test.go / codex_executor_stream_output_test.go.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SSEEvent is the minimal frame shape this module consumes. It is decoupled
// from contract.ConversationStreamEvent so callers that already parse SSE
// (the gateway and ad-hoc retry paths) can pass raw frames in without first
// going through the full ConversationResponse translation pipeline.
type SSEEvent struct {
	// Type is the SSE `event:` line (e.g. "response.output_item.done",
	// "response.completed", "error", "response.failed").
	Type string
	// Data is the raw JSON payload of the `data:` line. May be empty for
	// terminal frames that carry the payload elsewhere.
	Data []byte
}

// ReconstructedCodexOutput is the shape of an output-array element after
// reconstruction. Codex's Responses API emits arbitrary `output_item.done`
// shapes (text, tool calls, reasoning, images, etc.) — we keep them as raw
// JSON objects so the caller doesn't need to maintain a typed mirror of the
// upstream schema.
type ReconstructedCodexOutput = map[string]any

// ReconstructCodexCompletedOutput walks an SSE event stream and rebuilds the
// `response.output` array that should accompany the terminal
// `response.completed` event. This mirrors CLIProxyAPI's
// collectCodexOutputItemDone + patchCodexCompletedOutput pair: if the
// terminal event arrives with an empty or missing `output`, we patch it
// using the buffered `response.output_item.done` items, ordered by
// `output_index` (falling back to arrival order when no index is provided).
//
// The second return reports whether the stream ended in a
// "context_length_exceeded" terminal error — callers should NOT retry on a
// different account in that case (it is a request-level fault, not a
// provider fault).
func ReconstructCodexCompletedOutput(events []SSEEvent) ([]ReconstructedCodexOutput, bool, error) {
	indexed := map[int64][]byte{}
	var fallback [][]byte
	var contextLengthExceeded bool
	var terminalCompleted bool
	var rebuiltFromCompleted []ReconstructedCodexOutput

	for _, evt := range events {
		if len(evt.Data) == 0 {
			continue
		}
		switch evt.Type {
		case "response.output_item.done":
			collectOutputItemDone(evt.Data, indexed, &fallback)
		case "response.completed":
			terminalCompleted = true
			if rebuilt, ok, err := tryUnmarshalCompletedOutput(evt.Data); err != nil {
				return nil, false, err
			} else if ok {
				rebuiltFromCompleted = rebuilt
			}
		case "response.failed", "error":
			if isContextLengthError(evt.Data) {
				contextLengthExceeded = true
			}
		}
	}

	out := rebuiltFromCompleted
	if (len(out) == 0) && (len(indexed) > 0 || len(fallback) > 0) {
		out = collectIndexedAndFallback(indexed, fallback)
	}

	// A stream that produced no terminal completed event and no items is
	// effectively interrupted — the caller may want to retry. Surface as an
	// error so the caller decides.
	if !terminalCompleted && !contextLengthExceeded && len(out) == 0 {
		return nil, false, errCodexStreamIncomplete
	}
	return out, contextLengthExceeded, nil
}

var errCodexStreamIncomplete = errors.New("codex stream incomplete: no terminal event or items")

func collectOutputItemDone(data []byte, indexed map[int64][]byte, fallback *[][]byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	itemRaw, ok := raw["item"]
	if !ok || len(itemRaw) == 0 || itemRaw[0] != '{' {
		return
	}
	// Defensive copy so a downstream mutation cannot reach back into the
	// caller-owned event buffer.
	itemCopy := append([]byte(nil), itemRaw...)
	if oi, ok := raw["output_index"]; ok && len(oi) > 0 {
		var idx int64
		if err := json.Unmarshal(oi, &idx); err == nil {
			indexed[idx] = itemCopy
			return
		}
	}
	*fallback = append(*fallback, itemCopy)
}

func collectIndexedAndFallback(indexed map[int64][]byte, fallback [][]byte) []ReconstructedCodexOutput {
	indexes := make([]int64, 0, len(indexed))
	for idx := range indexed {
		indexes = append(indexes, idx)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })

	out := make([]ReconstructedCodexOutput, 0, len(indexed)+len(fallback))
	for _, idx := range indexes {
		obj, ok := unmarshalObject(indexed[idx])
		if !ok {
			continue
		}
		out = append(out, obj)
	}
	for _, raw := range fallback {
		obj, ok := unmarshalObject(raw)
		if !ok {
			continue
		}
		out = append(out, obj)
	}
	return out
}

func unmarshalObject(raw []byte) (map[string]any, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

// tryUnmarshalCompletedOutput extracts response.output from a
// response.completed event payload when present. Returns (nil, false, nil)
// when the field is absent — that's the case CLIProxyAPI patches.
func tryUnmarshalCompletedOutput(data []byte) ([]ReconstructedCodexOutput, bool, error) {
	var envelope struct {
		Response struct {
			Output []ReconstructedCodexOutput `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		// A malformed completed envelope is treated as "no rebuilt output"
		// — let the indexed/fallback path try.
		return nil, false, nil
	}
	if len(envelope.Response.Output) == 0 {
		return nil, false, nil
	}
	return envelope.Response.Output, true, nil
}

// isContextLengthError mirrors codexTerminalErrorIsContextLength: any of the
// canonical codes / message substrings counts as a context-length fault.
func isContextLengthError(data []byte) bool {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	// Walk both `error.{code,message}` and the top-level message — Codex
	// emits both shapes for `response.failed` vs. `error`.
	if errObj, ok := raw["error"].(map[string]any); ok {
		if matchContextLengthFields(errObj) {
			return true
		}
	}
	if respObj, ok := raw["response"].(map[string]any); ok {
		if errObj, ok := respObj["error"].(map[string]any); ok {
			if matchContextLengthFields(errObj) {
				return true
			}
		}
	}
	return matchContextLengthFields(raw)
}

func matchContextLengthFields(obj map[string]any) bool {
	code := strings.ToLower(strings.TrimSpace(asString(obj["code"])))
	msg := strings.ToLower(strings.TrimSpace(asString(obj["message"])))
	if code == "context_length_exceeded" || code == "context_too_large" {
		return true
	}
	return strings.Contains(msg, "context window") ||
		strings.Contains(msg, "context length") ||
		strings.Contains(msg, "too many tokens")
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

// CodexStreamErrorClass enumerates the buckets ClassifyCodexStreamTerminalError
// returns. Stable identifiers so callers can switch on them without
// re-implementing the classification.
type CodexStreamErrorClass string

const (
	CodexErrClassNone                  CodexStreamErrorClass = ""
	CodexErrClassContextLengthExceeded CodexStreamErrorClass = "context_length_exceeded"
	CodexErrClassRateLimit             CodexStreamErrorClass = "rate_limit_exceeded"
	CodexErrClassUpstream5xx           CodexStreamErrorClass = "upstream_5xx"
	CodexErrClassTransientNetwork      CodexStreamErrorClass = "transient_network"
	CodexErrClassUnknown               CodexStreamErrorClass = "unknown"
)

// CodexStreamErrorVerdict carries the classifier result plus retry advice.
// Mirrors UpstreamFailoverDecision in shape so wiring sites can pass it
// straight through to the failover loop without an adapter layer.
type CodexStreamErrorVerdict struct {
	Class      CodexStreamErrorClass
	Retryable  bool
	RetryAfter time.Duration
}

// ClassifyCodexStreamTerminalError maps the terminal-stream error events to
// the classifier's verdict.
//
// Policy:
//   - context_length_exceeded → terminal, no retry.
//   - rate_limit_exceeded     → retryable with RetryAfter (parsed from
//     error.retry_after or 0 when absent).
//   - 5xx-shaped upstream     → retryable, no specific RetryAfter.
//   - transient network/EOF   → retryable.
//   - anything else           → unknown, no retry.
//
// nil err returns CodexErrClassNone, retryable=false (caller path likely
// shouldn't have invoked the classifier).
func ClassifyCodexStreamTerminalError(err error) CodexStreamErrorVerdict {
	if err == nil {
		return CodexStreamErrorVerdict{Class: CodexErrClassNone}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context_length_exceeded") ||
		strings.Contains(msg, "context_too_large") ||
		strings.Contains(msg, "context window") ||
		strings.Contains(msg, "context length") ||
		strings.Contains(msg, "too many tokens"):
		return CodexStreamErrorVerdict{Class: CodexErrClassContextLengthExceeded, Retryable: false}
	case strings.Contains(msg, "rate_limit_exceeded") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "usage_limit_reached"):
		return CodexStreamErrorVerdict{Class: CodexErrClassRateLimit, Retryable: true, RetryAfter: parseRetryAfterFromMessage(msg)}
	case strings.Contains(msg, "internal server error") ||
		strings.Contains(msg, "bad gateway") ||
		strings.Contains(msg, "service unavailable") ||
		strings.Contains(msg, "gateway timeout") ||
		strings.Contains(msg, "upstream_5xx") ||
		strings.Contains(msg, "provider_5xx"):
		return CodexStreamErrorVerdict{Class: CodexErrClassUpstream5xx, Retryable: true}
	case strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "stream_interrupted"):
		return CodexStreamErrorVerdict{Class: CodexErrClassTransientNetwork, Retryable: true}
	}
	return CodexStreamErrorVerdict{Class: CodexErrClassUnknown, Retryable: false}
}

// codexStreamHasContextLengthError walks the parsed SSE frames of a
// streaming Codex response and reports true when a terminal error event
// indicates context_length_exceeded. Used at the top of
// parseCodexResponsesStream to short-circuit out as invalid_request 400
// (mirroring CLIProxyAPI's codexTerminalStreamContextLengthErr fast-path).
//
// The helper accepts sseFrame slices directly so the wiring site doesn't
// have to re-marshal frames into SSEEvents for the public API.
func codexStreamHasContextLengthError(frames []sseFrame) bool {
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		eventType := frame.EventType("")
		// Many providers send the event-line and embed `type` in the JSON;
		// honor both to match CLIProxyAPI's codexTerminalStreamErr.
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(data), &probe); err != nil {
			continue
		}
		t := strings.TrimSpace(probe.Type)
		if t == "" {
			t = eventType
		}
		if t != "error" && t != "response.failed" {
			continue
		}
		if isContextLengthError([]byte(data)) {
			return true
		}
	}
	return false
}

// parseRetryAfterFromMessage scrapes a "retry after Ns" style suffix from an
// error message. Best-effort — returns 0 when not present.
func parseRetryAfterFromMessage(msg string) time.Duration {
	idx := strings.Index(msg, "retry after")
	if idx == -1 {
		return 0
	}
	rest := msg[idx+len("retry after"):]
	rest = strings.TrimLeft(rest, " :=")
	var n int
	var unit string
	if _, err := fmt.Sscanf(rest, "%d%s", &n, &unit); err != nil || n <= 0 {
		return 0
	}
	unit = strings.TrimSpace(strings.ToLower(unit))
	switch {
	case strings.HasPrefix(unit, "ms"), strings.HasPrefix(unit, "milli"):
		return time.Duration(n) * time.Millisecond
	case strings.HasPrefix(unit, "m"), strings.HasPrefix(unit, "min"):
		return time.Duration(n) * time.Minute
	case strings.HasPrefix(unit, "h"):
		return time.Duration(n) * time.Hour
	default:
		return time.Duration(n) * time.Second
	}
}
