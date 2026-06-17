// Codex image_generation tool wiring — verbatim port of
// CLIProxyAPI/internal/runtime/executor/codex_executor.go
// ensureImageGenerationTool (lines 1740-1773), plus a per-account
// semaphore around the Responses Do() so multiple in-flight image
// generations on one Codex auth do not flood the upstream.
//
// CLIProxyAPI keeps the inject unconditional (gated only by spark base
// model and free-plan auth + an operator-level DisableImageGeneration
// switch). srapi already has applyDisableImageGenerationToResponsesPayload
// to honour that switch, so this file only reproduces the spark/free-plan
// gates and the idempotent inject. The poll-state parser surfaces an
// in_progress image_generation_call so the caller can decide whether to
// poll (the existing event-loop in invokeReverseProxyCodexResponses
// drives the actual polling).
//
// Deviations from CLIProxyAPI (allowed by directive):
//   - map[string]any instead of tidwall/gjson+sjson, because srapi keeps
//     the Responses payload as a Go map until marshaling. The injected
//     tool object matches the imageGenToolJSON literal byte-for-byte
//     once marshaled.
//   - The slot limiter is the existing channel-based limiter shipped in
//     PR-3 (chatGPTWebImageSlotLimiter) — ctx-cancel safe, FIFO waiters.
//     CLIProxyAPI's per-account chan limiter is a Python condvar; we
//     keep parity on the happy path and add ctx safety.
//   - Bounded LRU is inherited from the underlying limiter, which only
//     ever tracks active accounts (released keys are deleted) — same
//     property CLIProxyAPI exhibits.
package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// codexImageGenJSONUnmarshal is a thin indirection so the parser can be
// swapped out in tests if needed; production uses encoding/json.
var codexImageGenJSONUnmarshal = json.Unmarshal

// codexImageGenToolDefault mirrors CLIProxyAPI's imageGenToolJSON literal
// (codex_executor.go:1740). The shape and field order must match so the
// downstream codexResponsesToolsContainType("image_generation") check
// continues to work without modification.
func codexImageGenToolDefault() map[string]any {
	return map[string]any{
		"type":          "image_generation",
		"output_format": "png",
	}
}

// ensureCodexImageGenerationTool mirrors CLIProxyAPI's
// ensureImageGenerationTool. It guarantees the payload's "tools" array
// contains a {"type":"image_generation"} entry, unless:
//   - the upstream base model ends with "spark" (the spark family does
//     not support the image_generation tool), or
//   - the codex auth is a free-plan account (free plans cannot use the
//     image_generation tool either; upstream rejects with a 400).
//
// Idempotent: a no-op when an image_generation tool is already present.
// Appends to an existing tools array; creates the array when absent.
func ensureCodexImageGenerationTool(payload map[string]any, baseModel string, account accountcontract.ProviderAccount) {
	if payload == nil {
		return
	}
	if strings.HasSuffix(strings.TrimSpace(baseModel), "spark") {
		return
	}
	if isCodexFreePlanAccount(account) {
		return
	}
	if codexResponsesToolsContainType(payload["tools"], "image_generation") {
		return
	}
	tool := codexImageGenToolDefault()
	switch tools := payload["tools"].(type) {
	case nil:
		payload["tools"] = []any{tool}
	case []any:
		payload["tools"] = append(tools, tool)
	case []map[string]any:
		out := make([]any, 0, len(tools)+1)
		for _, t := range tools {
			out = append(out, t)
		}
		out = append(out, tool)
		payload["tools"] = out
	default:
		// Unrecognised shape: replace with a fresh single-tool array so
		// the upstream still gets a well-formed payload. Matches
		// CLIProxyAPI's "tools doesn't exist or isn't an array" branch.
		payload["tools"] = []any{tool}
	}
}

// isCodexFreePlanAccount mirrors isCodexFreePlanAuth from CLIProxyAPI.
// In srapi the plan_type lives on the account metadata, populated by
// upstream quota responses (see codex_quota.go / codex.go:1809).
func isCodexFreePlanAccount(account accountcontract.ProviderAccount) bool {
	if account.Metadata == nil {
		return false
	}
	raw, ok := account.Metadata["plan_type"]
	if !ok || raw == nil {
		return false
	}
	value, ok := raw.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "free")
}

// codexPayloadHasImageGenerationTool reports whether the marshalable
// Responses payload already has an image_generation tool registered.
// Used by the slot-acquire wiring in invokeReverseProxyCodexResponses
// to decide whether to consume a slot.
func codexPayloadHasImageGenerationTool(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	return codexResponsesToolsContainType(payload["tools"], "image_generation")
}

// codexPayloadInputUsesImageGenerationCall reports whether the client
// has already emitted an image_generation_call item in the Responses
// input array. This is the "client used image_generation_call type"
// trigger from the wiring spec — when present, srapi auto-injects the
// matching image_generation tool so the upstream accepts the call.
func codexPayloadInputUsesImageGenerationCall(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	items, ok := codexAnySlice(payload["input"])
	if !ok {
		return false
	}
	for _, raw := range items {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(codexStringValue(entry["type"])), "image_generation_call") {
			return true
		}
	}
	return false
}

// DefaultCodexImageGenSlotCapacity matches CLIProxyAPI's default per-
// account image slot capacity (1; sub2api's chatgpt_web_image_slots
// default for the codex flow). Operators override via account metadata
// key codex_image_account_concurrency.
const DefaultCodexImageGenSlotCapacity = 1

// codexImageGenSlotCapacity reads the per-account override for the
// codex image-slot concurrency cap, defaulting to
// DefaultCodexImageGenSlotCapacity.
func codexImageGenSlotCapacity(account accountcontract.ProviderAccount) int {
	if account.Metadata == nil {
		return DefaultCodexImageGenSlotCapacity
	}
	for _, key := range []string{
		"codex_image_account_concurrency",
		"codex_image_slot_capacity",
	} {
		raw, ok := account.Metadata[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case int:
			if v > 0 {
				return v
			}
		case int64:
			if v > 0 {
				return int(v)
			}
		case float64:
			if v > 0 {
				return int(v)
			}
		case string:
			if n := parseIntOrZero(v); n > 0 {
				return n
			}
		}
	}
	return DefaultCodexImageGenSlotCapacity
}

// codexImageGenSlotKey builds a stable per-account key for the limiter.
// Mirrors the chatGPTWebImageSlotKey shape but uses a codex- prefix so
// the two flows never share slot state.
func codexImageGenSlotKey(accountID int) string {
	if accountID <= 0 {
		return ""
	}
	return "codex-" + strconv.Itoa(accountID)
}

// codexImageGenSlotAcquire reserves an image-generation slot for the
// given account. Returns a release function that must always be called
// (even on error paths) — release is a no-op when key is empty (i.e.
// the slot was never acquired). Mirrors the CLIProxyAPI per-account
// semaphore (one in-flight image gen per codex auth by default).
//
// The limiter shares the channel-based runtime from PR-3
// (chatGPTWebImageSlotLimiter), instantiated lazily as a process-global
// for the codex flow.
func codexImageGenSlotAcquire(ctx context.Context, account accountcontract.ProviderAccount) (func(), error) {
	key := codexImageGenSlotKey(account.ID)
	if key == "" {
		return func() {}, nil
	}
	capacity := codexImageGenSlotCapacity(account)
	if err := codexImageGenSlotLimiter().Acquire(ctx, key, capacity); err != nil {
		return func() {}, err
	}
	return func() {
		codexImageGenSlotLimiter().Release(key)
	}, nil
}

// CodexImageGenPollState describes the upstream's response when an
// image_generation_call item is mid-flight. The caller is expected to
// re-issue the original request (or poll the response_id endpoint,
// depending on the upstream protocol) until Polling is false.
type CodexImageGenPollState struct {
	// Polling is true when the response carries an image_generation_call
	// item in status "in_progress" (or "generating", which some Codex
	// backends emit interchangeably).
	Polling bool
	// PollID is the image_generation_call.id of the in-progress item
	// when one is found. Empty when no id is reported.
	PollID string
	// RetryAfter is the suggested wait in seconds parsed from the
	// upstream response's retry-after / poll_after fields. Zero means
	// the caller should choose its own backoff.
	RetryAfter int
}

// parseCodexImageGenPollState inspects an upstream Responses body and
// reports whether it contains an in-flight image_generation_call. The
// completion path is on the caller — the existing streaming loop in
// invokeReverseProxyCodexResponses already drives polling for other
// in-progress responses; surfacing the state here lets the caller add
// image-specific telemetry without re-parsing the body.
//
// Mirrors CLIProxyAPI's polling semantics: an image_generation_call
// with status "in_progress" means the upstream accepted the request
// but the raster output is not yet ready. The non-streaming Responses
// path returns this as a top-level response item; the streaming path
// emits response.image_generation_call.* events instead (handled
// elsewhere — this helper is only for the buffered path).
func parseCodexImageGenPollState(body []byte) CodexImageGenPollState {
	state := CodexImageGenPollState{}
	if len(body) == 0 {
		return state
	}
	// We intentionally use the same map-based parse as the rest of the
	// codex.go inbound path (parseCodexResponsesBody) rather than
	// re-introducing gjson — the upstream wraps the items inside
	// response.output for the non-stream JSON shape.
	var doc struct {
		Response struct {
			Output []struct {
				Type   string `json:"type"`
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"output"`
			RetryAfter int `json:"retry_after"`
		} `json:"response"`
		Output []struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"output"`
		RetryAfter int `json:"retry_after"`
	}
	if err := codexImageGenJSONUnmarshal(body, &doc); err != nil {
		return state
	}
	items := doc.Response.Output
	if len(items) == 0 {
		items = doc.Output
	}
	retryAfter := doc.Response.RetryAfter
	if retryAfter == 0 {
		retryAfter = doc.RetryAfter
	}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Type), "image_generation_call") {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status == "in_progress" || status == "generating" || status == "queued" {
			state.Polling = true
			state.PollID = strings.TrimSpace(item.ID)
			state.RetryAfter = retryAfter
			return state
		}
	}
	return state
}

// codexImageGenSlotLimiter returns the process-global limiter used by
// the codex flow. We piggy-back on the chatgpt_web_image_slots runtime
// because it is the exact channel-based, ctx-cancel-safe semaphore
// pattern PR-3 shipped — the keys are namespaced ("codex-N") so the
// two flows never collide on slot state.
func codexImageGenSlotLimiter() *ChatGPTWebImageSlotLimiter {
	return chatGPTWebImageSlotLimiter()
}
