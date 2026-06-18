// Wiring glue that activates the codex_reasoning_replay_cache + codex_identity_
// confuse modules in the live Codex request hot path. PR-1 ported the
// algorithms from CLIProxyAPI verbatim, but the request flow was never wired
// to call them, leaving the modules as dead code (the verifier flagged this
// gap explicitly in its gotchas). This file is the activation layer.
//
// Design:
//   - One process-global CodexReasoningReplayCache (10k LRU, 1h sliding TTL)
//     matching the CLIProxyAPI defaults; sized at init so we don't need an
//     App-struct field or newHandler signature change.
//   - codexIdentityConfuseConfigForAccount: always-on (SessionAffinity=true)
//     for any account whose runtime class implies multiplexing (OAuth-class)
//     or any account whose ID is non-zero. The CLIProxyAPI flag matrix
//     gates on routing strategy; in srapi we default-on because the
//     scheduler routinely multiplexes one account across many users — the
//     prompt_cache_key + installation_id leakage prevention is always
//     desirable in production.
//   - codexApplyOutboundWiring: at the point the outbound body is built,
//     rewrite identifiers via ApplyCodexIdentityConfuseBody and propagate
//     the rewrite into request headers via ApplyCodexIdentityConfuseHeaders.
//     Returns the (possibly new) body and the state that the response side
//     must consume.
//   - codexApplyReasoningReplay: before identity confusion rewrites the request,
//     inject cached reasoning / tool-call items for the same source session.
//     This mirrors CLIProxyAPI's applyCodexReasoningReplayCacheRequired and
//     keeps stateless Anthropic/Claude callers from losing encrypted reasoning
//     continuity when routed through Codex.
//   - codexCaptureInboundWiring: after a successful upstream response, (1)
//     reverse-map any echoed identifiers in the raw response so the client
//     sees its original ids, (2) walk the response's JSON output array or SSE
//     response.output_item.done frames and persist completed reasoning / tool-
//     call items keyed on prompt_cache_key.
package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const (
	// CLIProxyAPI defaults — preserved verbatim so wire behaviour is
	// identical to the reference implementation under matching load.
	codexReasoningReplayCacheMaxEntries = 10_240
	codexReasoningReplayCacheEvictBatch = 128
	codexReasoningReplayCacheTTL        = time.Hour
)

var (
	codexReasoningReplayCacheOnce sync.Once
	codexReasoningReplayCacheInst *CodexReasoningReplayCache
	codexReplayClaudeSessionRE    = regexp.MustCompile(`_session_([a-fA-F0-9-]+)$`)
)

// codexReasoningReplayCache returns the package-global cache instance,
// initialised lazily on first use so tests can swap it out by re-assigning
// the package var if they need to.
func codexReasoningReplayCache() *CodexReasoningReplayCache {
	codexReasoningReplayCacheOnce.Do(func() {
		codexReasoningReplayCacheInst = NewCodexReasoningReplayCache(
			codexReasoningReplayCacheMaxEntries,
			codexReasoningReplayCacheEvictBatch,
			codexReasoningReplayCacheTTL,
			time.Now,
		)
	})
	return codexReasoningReplayCacheInst
}

// codexIdentityConfuseConfigForAccount produces the runtime confuse config
// for one Codex request. Default ON (opt-out): identity confusion runs
// for every Codex account unless the account metadata explicitly sets
// "codex_identity_confuse" to a falsy value. This mirrors CLIProxyAPI's
// default-on routing-strategy gate but doesn't make operators discover
// the flag — when one upstream account is multiplexed across many
// callers, OpenAI's prompt cache and identity heuristics should not be
// able to fingerprint the multiplexer. Operators with single-tenant
// setups or integration tests that pin specific identifiers opt out
// via metadata.
//
// CLIProxyAPI gates this internally on (SessionAffinity || routing
// strategy == fill-first). srapi is more permissive: when on we always
// treat the account as session-affinity to keep the rewrite
// deterministic across turns of the same prompt_cache_key.
func codexIdentityConfuseConfigForAccount(account accountcontract.ProviderAccount) CodexIdentityConfuseConfig {
	if metadataFalsy(account.Metadata, "codex_identity_confuse") {
		return CodexIdentityConfuseConfig{}
	}
	return CodexIdentityConfuseConfig{
		Enabled:         true,
		SessionAffinity: true,
		RoutingStrategy: "fill-first",
	}
}

// metadataFalsy reads a metadata key and treats explicit `false` / "false"
// / "0" / "no" / "off" as falsy. Absent / empty / anything else is NOT
// falsy — used by codexIdentityConfuseConfigForAccount where the default
// is ON and operators opt out only by setting one of these explicit
// values. Pairs with metadataTruthy for the inverse semantics.
func metadataFalsy(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := v.(type) {
	case bool:
		return !typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "false", "0", "no", "off":
			return true
		}
		return false
	default:
		return false
	}
}

// metadataTruthy reads a metadata key and treats true / "true" / "1" /
// "yes" / "on" as truthy. Everything else (absent, empty, false, 0,
// other types) is false. Mirrors the convention used by metadataBool in
// the accounts contract.
func metadataTruthy(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true
		}
		return false
	default:
		return false
	}
}

// codexIdentityAuthIDForAccount derives a stable per-account auth id used
// as the namespace seed for UUIDv5 in identity_confuse. Using the integer
// account ID stringified keeps the per-account rewrite deterministic and
// stable across restarts; the UUIDv5 derivation gives downstream OpenAI a
// stable-per-account identifier without ever exposing the integer.
func codexIdentityAuthIDForAccount(account accountcontract.ProviderAccount) string {
	if account.ID <= 0 {
		return ""
	}
	return "srapi-acct-" + strconv.Itoa(account.ID)
}

// codexApplyOutboundWiring is the integration point on the outbound side:
// rewrite the body's identifiers (prompt_cache_key, installation_id, turn
// metadata) to per-account UUIDv5 derivatives so multiplexing this account
// across many users can't cross-contaminate OpenAI's prompt cache.
//
// Returns the (possibly new) body and the rewrite state. The state lets
// codexCaptureInboundWiring reverse-map any echoed identifier in the
// response. Safe to call with a zero account (returns the body unchanged
// + a zero state).
func codexApplyOutboundWiring(account accountcontract.ProviderAccount, headers http.Header, rawBody []byte) ([]byte, CodexIdentityConfuseState) {
	cfg := codexIdentityConfuseConfigForAccount(account)
	authID := codexIdentityAuthIDForAccount(account)
	// userPayload == rawBody here because srapi doesn't do a separate
	// "user" build step; the body the client effectively sees as the
	// outbound payload is the one already marshaled. ApplyCodexIdentityConfuseBody
	// reads originals out of userPayload before rewriting raw, which works
	// fine when they're the same buffer (it parses twice).
	out, state := ApplyCodexIdentityConfuseBody(cfg, authID, rawBody, rawBody)
	ApplyCodexIdentityConfuseHeaders(headers, &state)
	return out, state
}

// codexApplyReasoningReplay injects cached reasoning / tool-call items before
// the next live input item. It is deliberately limited to Anthropic-origin
// traffic, matching CLIProxyAPI's replay source gate; native OpenAI Responses
// requests already carry their own state.
func codexApplyReasoningReplay(req contract.ConversationRequest, payload map[string]any) codexReasoningReplayScope {
	scope := codexReasoningReplayScopeFromRequest(req, payload)
	if !scope.valid() {
		return scope
	}
	items, ok := codexReasoningReplayCache().GetItems(scope.modelName, scope.sessionKey)
	if !ok {
		return scope
	}
	items = filterCodexReasoningReplayItemsForPayload(payload, items)
	if len(items) == 0 {
		return scope
	}
	insertCodexReasoningReplayItems(payload, items)
	return scope
}

type codexReasoningReplayScope struct {
	modelName  string
	sessionKey string
}

func (s codexReasoningReplayScope) valid() bool {
	return strings.TrimSpace(s.modelName) != "" && strings.TrimSpace(s.sessionKey) != ""
}

func codexReasoningReplayScopeFromRequest(req contract.ConversationRequest, payload map[string]any) codexReasoningReplayScope {
	if payload == nil || !codexReasoningReplayEnabledForRequest(req) {
		return codexReasoningReplayScope{}
	}
	return codexReasoningReplayScope{
		modelName:  codexReplayModel(req, payload),
		sessionKey: codexReasoningReplaySessionKey(req, payload),
	}
}

func codexReasoningReplayEnabledForRequest(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.SourceProtocol), "anthropic-compatible")
}

func codexReplayModel(req contract.ConversationRequest, payload map[string]any) string {
	if model := strings.TrimSpace(codexStringValue(payload["model"])); model != "" {
		return model
	}
	if model := strings.TrimSpace(req.Mapping.UpstreamModelName); model != "" {
		return contract.NormalizeCodexUpstreamModelName(model)
	}
	return strings.TrimSpace(req.Model)
}

func codexReasoningReplaySessionKey(req contract.ConversationRequest, payload map[string]any) string {
	if key := codexReasoningReplaySessionKeyFromPayload(payload); key != "" {
		return key
	}
	if key := codexClaudeReplaySessionKey(req.RawBody); key != "" {
		return key
	}
	return ""
}

func codexReasoningReplaySessionKeyFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if promptCacheKey := codexStringValue(payload["prompt_cache_key"]); promptCacheKey != "" {
		return codexPromptCacheReplaySessionKey(promptCacheKey)
	}
	metadata, _ := payload["client_metadata"].(map[string]any)
	if metadata == nil {
		return ""
	}
	if turnMetadata := codexStringValue(metadata["x-codex-turn-metadata"]); turnMetadata != "" {
		if key := codexReasoningReplaySessionKeyFromTurnMetadata(turnMetadata); key != "" {
			return key
		}
	}
	if windowID := codexStringValue(metadata["x-codex-window-id"]); windowID != "" {
		return "window:" + windowID
	}
	return ""
}

func codexReasoningReplaySessionKeyFromTurnMetadata(raw string) string {
	var parsed struct {
		PromptCacheKey string `json:"prompt_cache_key"`
		WindowID       string `json:"window_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return ""
	}
	if promptCacheKey := strings.TrimSpace(parsed.PromptCacheKey); promptCacheKey != "" {
		return codexPromptCacheReplaySessionKey(promptCacheKey)
	}
	if windowID := strings.TrimSpace(parsed.WindowID); windowID != "" {
		return "window:" + windowID
	}
	return ""
}

func codexPromptCacheReplaySessionKey(promptCacheKey string) string {
	promptCacheKey = strings.TrimSpace(promptCacheKey)
	if promptCacheKey == "" {
		return ""
	}
	return "prompt-cache:" + promptCacheKey
}

func codexClaudeReplaySessionKey(rawBody []byte) string {
	if len(rawBody) == 0 {
		return ""
	}
	var body struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(rawBody), &body); err != nil || len(body.Metadata) == 0 {
		return ""
	}
	userID := strings.TrimSpace(codexStringValue(body.Metadata["user_id"]))
	if userID == "" {
		return ""
	}
	if strings.HasPrefix(userID, "{") {
		var parsed struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(userID), &parsed); err != nil {
			return ""
		}
		if sessionID := strings.TrimSpace(parsed.SessionID); sessionID != "" {
			return "claude:" + sessionID
		}
		return ""
	}
	if matches := codexReplayClaudeSessionRE.FindStringSubmatch(userID); len(matches) >= 2 {
		return "claude:" + strings.TrimSpace(matches[1])
	}
	return ""
}

func filterCodexReasoningReplayItemsForPayload(payload map[string]any, items [][]byte) [][]byte {
	input, ok := codexReplayPayloadInput(payload)
	if !ok {
		return nil
	}
	hasInputReasoning := codexInputHasReasoningEncryptedContent(input)
	existingCalls := map[string]bool{}
	existingOutputs := map[string]bool{}
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := codexStringValue(object["type"])
		if itemType == "function_call_output" || itemType == "custom_tool_call_output" {
			if callID := codexStringValue(object["call_id"]); callID != "" {
				for _, candidate := range codexReplayComparableCallIDs(callID) {
					existingOutputs[candidate] = true
				}
			}
		}
		for _, key := range codexReplayToolCallKeys(object) {
			existingCalls[key] = true
		}
	}

	filtered := make([][]byte, 0, len(items))
	for _, item := range items {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err != nil {
			continue
		}
		switch codexStringValue(object["type"]) {
		case "reasoning":
			if hasInputReasoning {
				continue
			}
		case "function_call", "custom_tool_call":
			keys := codexReplayToolCallKeys(object)
			if len(keys) == 0 || codexReplayAnyToolCallKeyExists(existingCalls, keys) {
				continue
			}
			hasMatchingOutput := false
			if callID := codexStringValue(object["call_id"]); callID != "" {
				for _, candidate := range codexReplayComparableCallIDs(callID) {
					if existingOutputs[candidate] {
						hasMatchingOutput = true
						break
					}
				}
			}
			if !hasMatchingOutput {
				continue
			}
			for _, key := range keys {
				existingCalls[key] = true
			}
		default:
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func codexInputHasReasoningEncryptedContent(input []any) bool {
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok || codexStringValue(object["type"]) != "reasoning" {
			continue
		}
		if codexStringValue(object["encrypted_content"]) != "" {
			return true
		}
	}
	return false
}

func insertCodexReasoningReplayItems(payload map[string]any, items [][]byte) {
	input, ok := codexReplayPayloadInput(payload)
	if !ok || len(items) == 0 {
		return
	}
	insertIndex := codexReasoningReplayInsertIndex(input, items)
	items = codexAlignReasoningReplayToolCallIDs(input, items)
	replayObjects := make([]any, 0, len(items))
	for _, item := range items {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err == nil {
			replayObjects = append(replayObjects, object)
		}
	}
	if len(replayObjects) == 0 {
		return
	}
	next := make([]any, 0, len(input)+len(replayObjects))
	for index, item := range input {
		if index == insertIndex {
			next = append(next, replayObjects...)
		}
		next = append(next, item)
	}
	if insertIndex == len(input) {
		next = append(next, replayObjects...)
	}
	payload["input"] = next
}

func codexReplayPayloadInput(payload map[string]any) ([]any, bool) {
	if payload == nil {
		return nil, false
	}
	switch input := payload["input"].(type) {
	case []any:
		return input, true
	case []codexResponsesInputItem:
		out := make([]any, 0, len(input))
		for _, item := range input {
			encoded, err := json.Marshal(item)
			if err != nil {
				return nil, false
			}
			var object map[string]any
			if err := json.Unmarshal(encoded, &object); err != nil {
				return nil, false
			}
			out = append(out, object)
		}
		payload["input"] = out
		return out, true
	default:
		return nil, false
	}
}

func codexReasoningReplayInsertIndex(input []any, replayItems [][]byte) int {
	replayCallIDs := map[string]bool{}
	for _, item := range replayItems {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err != nil {
			continue
		}
		itemType := codexStringValue(object["type"])
		if itemType != "function_call" && itemType != "custom_tool_call" {
			continue
		}
		for _, callID := range codexReplayComparableCallIDs(codexStringValue(object["call_id"])) {
			replayCallIDs[callID] = true
		}
	}
	if len(replayCallIDs) > 0 {
		for index, item := range input {
			object, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType := codexStringValue(object["type"])
			if itemType != "function_call_output" && itemType != "custom_tool_call_output" {
				continue
			}
			callID := codexStringValue(object["call_id"])
			if callID == "" || replayCallIDs[callID] {
				return index
			}
		}
	}
	for index := len(input) - 1; index >= 0; index-- {
		object, ok := input[index].(map[string]any)
		if ok && codexStringValue(object["type"]) == "message" && codexStringValue(object["role"]) == "assistant" {
			return index
		}
	}
	for index, item := range input {
		if shouldInsertCodexReasoningReplayBefore(item) {
			return index
		}
	}
	return len(input)
}

func codexAlignReasoningReplayToolCallIDs(input []any, replayItems [][]byte) [][]byte {
	outputCallIDs := codexReplayOutputCallIDs(input)
	if len(outputCallIDs) == 0 {
		return replayItems
	}
	aligned := make([][]byte, 0, len(replayItems))
	for _, item := range replayItems {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err != nil {
			aligned = append(aligned, item)
			continue
		}
		itemType := codexStringValue(object["type"])
		if itemType != "function_call" && itemType != "custom_tool_call" {
			aligned = append(aligned, item)
			continue
		}
		callID := codexStringValue(object["call_id"])
		outputCallID := ""
		for _, candidate := range codexReplayComparableCallIDs(callID) {
			if value := outputCallIDs[candidate]; value != "" {
				outputCallID = value
				break
			}
		}
		if outputCallID == "" || outputCallID == callID {
			aligned = append(aligned, item)
			continue
		}
		object["call_id"] = outputCallID
		encoded, err := json.Marshal(object)
		if err != nil {
			aligned = append(aligned, item)
			continue
		}
		aligned = append(aligned, encoded)
	}
	return aligned
}

func codexReplayOutputCallIDs(input []any) map[string]string {
	outputCallIDs := map[string]string{}
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := codexStringValue(object["type"])
		if itemType != "function_call_output" && itemType != "custom_tool_call_output" {
			continue
		}
		callID := codexStringValue(object["call_id"])
		if callID == "" {
			continue
		}
		for _, candidate := range codexReplayComparableCallIDs(callID) {
			outputCallIDs[candidate] = callID
		}
	}
	return outputCallIDs
}

func shouldInsertCodexReasoningReplayBefore(item any) bool {
	object, ok := item.(map[string]any)
	if !ok {
		return true
	}
	if codexStringValue(object["type"]) != "message" {
		return true
	}
	switch codexStringValue(object["role"]) {
	case "developer", "system":
		return false
	default:
		return true
	}
}

func codexReplayToolCallKeys(item map[string]any) []string {
	itemType := codexStringValue(item["type"])
	if itemType != "function_call" && itemType != "custom_tool_call" {
		return nil
	}
	callIDs := codexReplayComparableCallIDs(codexStringValue(item["call_id"]))
	if len(callIDs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(callIDs))
	for _, callID := range callIDs {
		keys = append(keys, itemType+":"+callID)
	}
	return keys
}

func codexReplayAnyToolCallKeyExists(existing map[string]bool, keys []string) bool {
	for _, key := range keys {
		if existing[key] {
			return true
		}
	}
	return false
}

func codexReplayComparableCallIDs(callID string) []string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}
	shortened := shortenCodexReplayCallIDIfNeeded(sanitizeCodexReplayClaudeToolID(callID))
	if shortened == "" || shortened == callID {
		return []string{callID}
	}
	return []string{callID, shortened}
}

func sanitizeCodexReplayClaudeToolID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func shortenCodexReplayCallIDIfNeeded(id string) string {
	const limit = 64
	if len(id) <= limit {
		return id
	}
	sum := sha256SumHex(id)
	suffix := "_" + sum[:16]
	prefixLen := limit - len(suffix)
	if prefixLen <= 0 {
		return suffix[len(suffix)-limit:]
	}
	return id[:prefixLen] + suffix
}

func sha256SumHex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// codexCaptureInboundWiring is the integration point on the response side:
// (1) reverse-map any echoed identifiers in the response body so the client
// receives its own original ids back, (2) persist completed reasoning /
// output items into the replay cache keyed on the original prompt_cache_key
// + model. Caching is best-effort — failures here must not affect the
// response delivered to the caller.
func codexCaptureInboundWiring(state CodexIdentityConfuseState, scope codexReasoningReplayScope, responseBody []byte) []byte {
	if state.Enabled && len(responseBody) > 0 {
		responseBody = ApplyCodexIdentityExposeResponsePayload(responseBody, state)
	}
	if !scope.valid() {
		return responseBody
	}
	if codexClearReasoningReplayOnInvalidSignatureFromSSE(scope, responseBody) {
		return responseBody
	}
	codexCacheOutputItems(scope.modelName, scope.sessionKey, responseBody)
	return responseBody
}

// codexCacheOutputItems extracts the response's `output` array, marshals
// each completed reasoning / tool-call item individually, and pushes them
// into the replay cache. Items the normalize functions don't recognise
// are silently skipped (matches CLIProxyAPI behaviour). Best-effort: a
// malformed responseBody is a no-op, not an error.
func codexCacheOutputItems(model, sessionKey string, responseBody []byte) {
	items := codexReplayOutputItems(responseBody)
	if len(items) == 0 {
		return
	}
	codexReasoningReplayCache().PutItems(model, sessionKey, items)
}

func codexClearReasoningReplayOnInvalidSignature(scope codexReasoningReplayScope, statusCode int, responseBody []byte) {
	if !scope.valid() {
		return
	}
	err := codexErrorFromHTTPBody(responseBody)
	message := codexHTTPErrorMessage(responseBody, statusCode, err)
	class, effectiveStatus := codexHTTPErrorClassAndStatus(statusCode, responseBody, err, message)
	if class != "invalid_request" || effectiveStatus != http.StatusBadRequest {
		return
	}
	lower := strings.ToLower(strings.TrimSpace(string(responseBody)))
	if !strings.Contains(lower, "invalid signature in thinking block") && !strings.Contains(lower, "invalid_encrypted_content") {
		return
	}
	codexReasoningReplayCache().Delete(scope.modelName, scope.sessionKey)
}

func codexClearReasoningReplayOnInvalidSignatureFromSSE(scope codexReasoningReplayScope, responseBody []byte) bool {
	if !scope.valid() {
		return false
	}
	for _, body := range codexInvalidSignatureTerminalErrorBodies(responseBody) {
		err := codexErrorFromHTTPBody(body)
		message := codexHTTPErrorMessage(body, http.StatusBadRequest, err)
		class, effectiveStatus := codexHTTPErrorClassAndStatus(http.StatusBadRequest, body, err, message)
		if class != "invalid_request" || effectiveStatus != http.StatusBadRequest {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(string(body)))
		if strings.Contains(lower, "invalid signature in thinking block") || strings.Contains(lower, "invalid_encrypted_content") {
			codexReasoningReplayCache().Delete(scope.modelName, scope.sessionKey)
			return true
		}
	}
	return false
}

func codexInvalidSignatureTerminalErrorBodies(responseBody []byte) [][]byte {
	frames, err := parseSSEFrames(responseBody)
	if err != nil {
		return nil
	}
	bodies := make([][]byte, 0)
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		body, ok := codexTerminalSSEErrorBody([]byte(data), strings.TrimSpace(frame.Event))
		if ok {
			bodies = append(bodies, body)
		}
	}
	return bodies
}

func codexTerminalSSEErrorBody(data []byte, eventName string) ([]byte, bool) {
	var event struct {
		Type     string               `json:"type"`
		Error    *codexResponsesError `json:"error"`
		Response struct {
			Error *codexResponsesError `json:"error"`
		} `json:"response"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, false
	}
	eventType := firstNonEmpty(strings.TrimSpace(event.Type), eventName)
	var terminalErr *codexResponsesError
	switch eventType {
	case "error":
		terminalErr = event.Error
	case "response.failed":
		if event.Response.Error != nil {
			terminalErr = event.Response.Error
		} else {
			terminalErr = event.Error
		}
	default:
		return nil, false
	}
	if terminalErr == nil {
		if strings.TrimSpace(event.Message) == "" && strings.TrimSpace(event.Code) == "" {
			return nil, false
		}
		terminalErr = &codexResponsesError{Message: event.Message, Code: event.Code}
	}
	if strings.TrimSpace(terminalErr.Message) == "" &&
		strings.TrimSpace(terminalErr.Code) == "" &&
		strings.TrimSpace(terminalErr.Type) == "" {
		return nil, false
	}
	body, err := json.Marshal(struct {
		Error codexResponsesError `json:"error"`
	}{Error: *terminalErr})
	if err != nil {
		return nil, false
	}
	return body, true
}

func codexReplayOutputItems(responseBody []byte) [][]byte {
	if items := codexReplayOutputItemsFromJSON(responseBody); len(items) > 0 {
		return items
	}
	return codexReplayOutputItemsFromSSE(responseBody)
}

func codexReplayOutputItemsFromJSON(responseBody []byte) [][]byte {
	var parsed struct {
		Output   []json.RawMessage `json:"output"`
		Response struct {
			Output []json.RawMessage `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil
	}
	rawItems := parsed.Output
	if len(rawItems) == 0 {
		rawItems = parsed.Response.Output
	}
	return cloneReplayRawItems(rawItems)
}

func codexReplayOutputItemsFromSSE(responseBody []byte) [][]byte {
	frames, err := parseSSEFrames(responseBody)
	if err != nil {
		return nil
	}
	items := make([]json.RawMessage, 0)
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var envelope struct {
			Type     string          `json:"type"`
			Item     json.RawMessage `json:"item"`
			Response struct {
				Output []json.RawMessage `json:"output"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			continue
		}
		eventType := frame.EventType(envelope.Type)
		switch eventType {
		case "response.output_item.done":
			if len(envelope.Item) > 0 {
				items = append(items, envelope.Item)
			}
		case "response.completed", "response.done":
			if len(envelope.Response.Output) > 0 {
				items = append(items, envelope.Response.Output...)
			}
		}
	}
	return cloneReplayRawItems(items)
}

func cloneReplayRawItems(rawItems []json.RawMessage) [][]byte {
	items := make([][]byte, 0, len(rawItems))
	for _, raw := range rawItems {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}
		items = append(items, append([]byte(nil), trimmed...))
	}
	return items
}
