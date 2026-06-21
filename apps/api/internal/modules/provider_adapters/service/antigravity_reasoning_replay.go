// Antigravity reasoning replay wiring — pre-request splice, post-response
// capture, signature-failure cache clear. Operates on the raw JSON bytes of
// the antigravityRequest envelope ({"request":{"contents":[...]}}).
//
// Compared to the Codex equivalent (codex_wiring.go), this path is wire-shape
// specific: cache items are keyed by (contentIndex, partIndex) against
// request.contents[*].parts[*] instead of the linear Responses input array.
// We therefore work on the marshaled bytes rather than rebuilding the typed
// envelope.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// antigravityReasoningReplayScope identifies a (model, session) continuity
// boundary. Independent from the selected upstream credential so auth
// failover preserves replay.
type antigravityReasoningReplayScope struct {
	modelName  string
	sessionKey string
}

func (s antigravityReasoningReplayScope) valid() bool {
	return strings.TrimSpace(s.modelName) != "" && strings.TrimSpace(s.sessionKey) != ""
}

// antigravityUsesReasoningReplayCache gates the replay path by upstream
// model family. Claude-routed Antigravity has its own signature wiring; the
// replay cache is only for the Gemini-shaped path (Gemini / Flash /
// Agent-* model names). Matches CLIProxyAPI verbatim.
func antigravityUsesReasoningReplayCache(modelName string) bool {
	modelName = strings.ToLower(modelName)
	if strings.Contains(modelName, "claude") {
		return false
	}
	return strings.Contains(modelName, "gemini") ||
		strings.Contains(modelName, "flash") ||
		strings.Contains(modelName, "agent")
}

// antigravityReasoningReplayScopeFromRequest derives the scope from the
// request structure. Prefers the marshaled body's `sessionId` (this is what
// the upstream sees), falls back to setting / metadata / hashed text — the
// same priority sub2api uses for session affinity, kept verbatim.
func antigravityReasoningReplayScopeFromRequest(req contract.ConversationRequest, payload []byte) antigravityReasoningReplayScope {
	if !antigravityUsesReasoningReplayCache(req.Mapping.UpstreamModelName) {
		return antigravityReasoningReplayScope{}
	}
	model := strings.TrimSpace(req.Mapping.UpstreamModelName)
	if id := antigravityReplaySessionIDFromPayload(payload); id != "" {
		return antigravityReasoningReplayScope{modelName: model, sessionKey: "session:" + id}
	}
	// The adapter's antigravitySessionID() is already a deterministic
	// session-key producer for the live request — reuse it so the cache
	// key tracks the same continuity boundary the upstream sees.
	if id := antigravitySessionID(req); id != "" {
		return antigravityReasoningReplayScope{modelName: model, sessionKey: "session:" + strings.TrimPrefix(id, "-")}
	}
	if value := requestSetting(req, "antigravity_execution_session", "execution_session"); value != "" {
		return antigravityReasoningReplayScope{modelName: model, sessionKey: "execution:" + value}
	}
	return antigravityReasoningReplayScope{}
}

// antigravityReplaySessionIDFromPayload reads the canonical sessionId fields
// from the outbound antigravity request body.
func antigravityReplaySessionIDFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ""
	}
	if id := strings.TrimSpace(stringValueOrEmpty(obj["sessionId"])); id != "" {
		return id
	}
	if id := strings.TrimSpace(stringValueOrEmpty(obj["session_id"])); id != "" {
		return id
	}
	if inner, ok := obj["request"].(map[string]any); ok {
		if id := strings.TrimSpace(stringValueOrEmpty(inner["sessionId"])); id != "" {
			return id
		}
		if id := strings.TrimSpace(stringValueOrEmpty(inner["session_id"])); id != "" {
			return id
		}
	}
	return ""
}

// applyAntigravityReasoningReplayPayload splices cached items into the
// outbound payload's request.contents. Returns the (possibly new) payload
// bytes. On any parse failure the original bytes are returned untouched —
// replay is best-effort.
func applyAntigravityReasoningReplayPayload(ctx context.Context, cache *AntigravityReasoningReplayCache, scope antigravityReasoningReplayScope, payload []byte) []byte {
	if cache == nil || !scope.valid() || len(payload) == 0 {
		return payload
	}
	items, ok, err := cache.GetItemsCtx(ctx, scope.modelName, scope.sessionKey)
	if err != nil || !ok || len(items) == 0 {
		return payload
	}
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return payload
	}
	inner, ok := envelope["request"].(map[string]any)
	if !ok {
		return payload
	}
	contents, ok := inner["contents"].([]any)
	if !ok {
		return payload
	}
	contents, changed := mergeAntigravityReasoningReplayItems(contents, items)
	if !changed {
		return payload
	}
	inner["contents"] = contents
	updated, err := json.Marshal(envelope)
	if err != nil {
		return payload
	}
	return updated
}

// mergeAntigravityReasoningReplayItems applies cached replay items to the
// live `contents` array. Returns the (possibly new) contents and whether
// anything changed. Pure function — exported only for tests.
func mergeAntigravityReasoningReplayItems(contents []any, items [][]byte) ([]any, bool) {
	if len(items) == 0 {
		return contents, false
	}
	existing := antigravityExistingFunctionCallKeys(contents)
	changed := false
	for _, item := range items {
		var obj map[string]any
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		switch strings.TrimSpace(stringValueOrEmpty(obj["type"])) {
		case "thought_signature":
			if mergeAntigravityThoughtSignatureItem(contents, obj) {
				changed = true
			}
		case "function_call_part":
			updated, ok := mergeAntigravityFunctionCallItem(contents, obj, existing)
			if ok {
				contents = updated
				changed = true
			}
		}
	}
	return contents, changed
}

// mergeAntigravityThoughtSignatureItem writes the cached signature into the
// matching parts entry if (a) the location exists, (b) no signature is
// already there. Returns whether anything was written.
func mergeAntigravityThoughtSignatureItem(contents []any, obj map[string]any) bool {
	sig := strings.TrimSpace(stringValueOrEmpty(obj["thoughtSignature"]))
	if sig == "" {
		return false
	}
	ci, ciOK := antigravityReadInt(obj["contentIndex"])
	pi, piOK := antigravityReadInt(obj["partIndex"])
	if !ciOK || !piOK {
		return false
	}
	if ci < 0 || ci >= len(contents) {
		return false
	}
	content, ok := contents[ci].(map[string]any)
	if !ok {
		return false
	}
	parts, ok := content["parts"].([]any)
	if !ok || pi < 0 || pi >= len(parts) {
		return false
	}
	part, ok := parts[pi].(map[string]any)
	if !ok {
		return false
	}
	if strings.TrimSpace(stringValueOrEmpty(part["thoughtSignature"])) != "" {
		return false
	}
	part["thoughtSignature"] = sig
	parts[pi] = part
	content["parts"] = parts
	contents[ci] = content
	return true
}

// mergeAntigravityFunctionCallItem either fills in a missing signature on an
// already-present functionCall part, or inserts a fresh `role:"model"`
// content containing the cached functionCall right before the matching
// functionResponse. Returns the (possibly new) contents and whether
// anything changed.
func mergeAntigravityFunctionCallItem(contents []any, obj map[string]any, existing map[string]bool) ([]any, bool) {
	name := strings.TrimSpace(stringValueOrEmpty(obj["name"]))
	args, hasArgs := obj["args"]
	callID := strings.TrimSpace(stringValueOrEmpty(obj["call_id"]))
	sig := strings.TrimSpace(stringValueOrEmpty(obj["thoughtSignature"]))
	if name == "" || !hasArgs {
		return contents, false
	}

	if callID != "" {
		if ci, pi, found := antigravityFunctionCallPartLocation(contents, callID); found {
			if sig == "" {
				// Already present, no signature to add → no-op so we
				// don't churn the request.
				return contents, false
			}
			content := contents[ci].(map[string]any)
			parts := content["parts"].([]any)
			part := parts[pi].(map[string]any)
			if strings.TrimSpace(stringValueOrEmpty(part["thoughtSignature"])) != "" {
				return contents, false
			}
			part["thoughtSignature"] = sig
			parts[pi] = part
			content["parts"] = parts
			contents[ci] = content
			return contents, true
		}
		if frIndex, ok := antigravityFunctionResponseContentIndex(contents, callID); ok {
			return insertAntigravityModelFunctionCallBeforeContent(contents, frIndex, name, callID, sig, args), true
		}
	}

	// Suppress duplicate functionCalls when the same (name, args, id) is
	// already in the request. This catches the "callID missing on both
	// sides" case where we'd otherwise re-add the same call.
	if existing[antigravityFunctionCallKey(name, args, callID)] {
		return contents, false
	}
	return contents, false
}

func insertAntigravityModelFunctionCallBeforeContent(contents []any, beforeIndex int, name, callID, sig string, args any) []any {
	fc := map[string]any{"name": name}
	if callID != "" {
		fc["id"] = callID
	}
	fc["args"] = args
	part := map[string]any{"functionCall": fc}
	if sig != "" {
		part["thoughtSignature"] = sig
	}
	model := map[string]any{
		"role":  "model",
		"parts": []any{part},
	}
	if beforeIndex < 0 {
		beforeIndex = 0
	}
	if beforeIndex > len(contents) {
		beforeIndex = len(contents)
	}
	out := make([]any, 0, len(contents)+1)
	out = append(out, contents[:beforeIndex]...)
	out = append(out, model)
	out = append(out, contents[beforeIndex:]...)
	return out
}

func antigravityExistingFunctionCallKeys(contents []any) map[string]bool {
	keys := make(map[string]bool)
	for _, c := range contents {
		content, ok := c.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			fc, ok := part["functionCall"].(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(stringValueOrEmpty(fc["name"]))
			if name == "" {
				continue
			}
			id := strings.TrimSpace(stringValueOrEmpty(fc["id"]))
			keys[antigravityFunctionCallKey(name, fc["args"], id)] = true
		}
	}
	return keys
}

func antigravityFunctionCallPartLocation(contents []any, callID string) (int, int, bool) {
	for ci, c := range contents {
		content, ok := c.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for pi, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			fc, ok := part["functionCall"].(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(stringValueOrEmpty(fc["id"])) == callID {
				return ci, pi, true
			}
		}
	}
	return -1, -1, false
}

func antigravityFunctionResponseContentIndex(contents []any, callID string) (int, bool) {
	for ci, c := range contents {
		content, ok := c.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			fr, ok := part["functionResponse"].(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(stringValueOrEmpty(fr["id"])) == callID {
				return ci, true
			}
		}
	}
	return -1, false
}

func antigravityFunctionCallKey(name string, args any, callID string) string {
	rawArgs, _ := json.Marshal(args)
	h := sha256.Sum256([]byte(strings.Join([]string{name, string(rawArgs), callID}, "\x00")))
	return "fc:" + fmt.Sprintf("%x", h[:8])
}

// captureAntigravityReasoningReplayFromResponse walks the unwrapped
// response candidates' parts and pushes thoughtSignature + functionCall
// items into the cache. Best-effort: on any parse error the cache is
// left untouched.
func captureAntigravityReasoningReplayFromResponse(ctx context.Context, cache *AntigravityReasoningReplayCache, scope antigravityReasoningReplayScope, body []byte) {
	if cache == nil || !scope.valid() || len(body) == 0 {
		return
	}
	items := antigravityResponseReplayItems(body)
	if len(items) == 0 {
		return
	}
	cache.PutItemsCtx(ctx, scope.modelName, scope.sessionKey, items)
}

// captureAntigravityReasoningReplayFromSSEFrames runs the same capture for
// the streamed shape: each SSE frame is a chunk; we iterate over their
// payloads.
func captureAntigravityReasoningReplayFromSSEFrames(ctx context.Context, cache *AntigravityReasoningReplayCache, scope antigravityReasoningReplayScope, frames []sseFrame) {
	if cache == nil || !scope.valid() || len(frames) == 0 {
		return
	}
	all := make([][]byte, 0)
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		all = append(all, antigravityResponseReplayItems([]byte(data))...)
	}
	if len(all) == 0 {
		return
	}
	cache.PutItemsCtx(ctx, scope.modelName, scope.sessionKey, all)
}

func antigravityResponseReplayItems(body []byte) [][]byte {
	// Antigravity wraps in {"response": <gemini>, "traceId": "..."} but the
	// streaming path emits the inner gemini envelope directly. Probe both.
	parts, ok := antigravityResponsePartsFromBody(body)
	if !ok || len(parts) == 0 {
		return nil
	}
	items := make([][]byte, 0, len(parts))
	for pi, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		sig := strings.TrimSpace(stringValueOrEmpty(part["thoughtSignature"]))
		if sig == "" {
			sig = strings.TrimSpace(stringValueOrEmpty(part["thought_signature"]))
		}
		if fc, ok := part["functionCall"].(map[string]any); ok {
			item := buildAntigravityFunctionCallReplayItem(0, pi, fc, sig)
			if len(item) > 0 {
				items = append(items, item)
			}
			continue
		}
		if sig != "" {
			items = append(items, buildAntigravityThoughtSignatureReplayItem(0, pi, sig))
		}
	}
	return items
}

func antigravityResponsePartsFromBody(body []byte) ([]any, bool) {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false
	}
	resp, ok := envelope["response"].(map[string]any)
	if !ok {
		resp = envelope
	}
	candidates, ok := resp["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return nil, false
	}
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return nil, false
	}
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return nil, false
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return nil, false
	}
	return parts, true
}

func buildAntigravityThoughtSignatureReplayItem(contentIndex, partIndex int, signature string) []byte {
	encoded, err := encodePresentKeysInOrder(map[string]any{
		"type":             "thought_signature",
		"thoughtSignature": signature,
		"contentIndex":     contentIndex,
		"partIndex":        partIndex,
	}, []string{"type", "thoughtSignature", "contentIndex", "partIndex"})
	if err != nil {
		return nil
	}
	return encoded
}

func buildAntigravityFunctionCallReplayItem(contentIndex, partIndex int, fc map[string]any, signature string) []byte {
	name := strings.TrimSpace(stringValueOrEmpty(fc["name"]))
	args, hasArgs := fc["args"]
	if name == "" || !hasArgs {
		return nil
	}
	out := map[string]any{
		"type":         "function_call_part",
		"name":         name,
		"args":         args,
		"contentIndex": contentIndex,
		"partIndex":    partIndex,
	}
	if id := strings.TrimSpace(stringValueOrEmpty(fc["id"])); id != "" {
		out["call_id"] = id
	}
	if signature != "" {
		out["thoughtSignature"] = signature
	}
	encoded, err := encodePresentKeysInOrder(out, []string{"type", "call_id", "name", "args", "thoughtSignature", "contentIndex", "partIndex"})
	if err != nil {
		return nil
	}
	return encoded
}

// clearAntigravityReasoningReplayOnSignatureFailure inspects the upstream
// error response and, when the body looks like a thoughtSignature rejection,
// drops the cached entry so the next turn starts from scratch. This is the
// inverse of the silent gemini_signature_retry downgrade — we'd rather invalidate
// the replay state than serve a poisoned signature forever.
func clearAntigravityReasoningReplayOnSignatureFailure(cache *AntigravityReasoningReplayCache, scope antigravityReasoningReplayScope, statusCode int, body []byte) {
	if cache == nil || !scope.valid() {
		return
	}
	if statusCode != http.StatusBadRequest {
		return
	}
	bodyText := strings.ToLower(string(body))
	if !strings.Contains(bodyText, "thoughtsignature") &&
		!strings.Contains(bodyText, "thought_signature") &&
		!strings.Contains(bodyText, "thought signature") &&
		!strings.Contains(bodyText, "signature") {
		return
	}
	cache.Delete(scope.modelName, scope.sessionKey)
}

// antigravityStableSessionIDFromText is a deterministic per-conversation
// hash used as the last-ditch session key when the request carries no
// sessionId / setting. Mirrors antigravitySessionID's text-hash branch so
// the cache key matches what the upstream sees.
func antigravityStableSessionIDFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	n := int64(binary.BigEndian.Uint64(sum[:8])) & 0x7FFFFFFFFFFFFFFF
	return strconv.FormatInt(n, 10)
}
