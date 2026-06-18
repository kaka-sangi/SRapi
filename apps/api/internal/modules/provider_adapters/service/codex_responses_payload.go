package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func codexResponsesPayload(req contract.ConversationRequest) (map[string]any, bool, error) {
	payload, err := codexRawResponsesPayload(req)
	if err != nil {
		return nil, false, err
	}
	if payload == nil {
		payload = codexCanonicalResponsesPayload(req)
	}
	codexApplyResponsesPayloadDefaults(req, payload)
	stream := codexResponsesPayloadStream(payload)
	if codexResponsesCompactRequest(req) {
		// /v1/responses/compact is non-streaming by contract — sub2api
		// applyCodexOAuthTransform deletes the stream field outright
		// (openai_codex_transform.go:131-139), and Hermes (Codex CLI in
		// Rust) sends stream:false on the client-side request. After the
		// compact body normalizer deletes payload["stream"] entirely the
		// historical codexResponsesPayloadStream returns true for an
		// absent field, so without this explicit override we route the
		// request through the streaming proxy path with Accept:
		// text/event-stream — the upstream then returns SSE that Hermes
		// cannot parse, producing the client-side error "stream
		// disconnected before completion: missing field 'text' at line 1
		// column 203" (diagnosed live via the new system-log panel). Force
		// stream=false here so the codex adapter buffers the upstream
		// response and returns it as a single JSON body the client expects.
		stream = false
	}
	return payload, stream, nil
}

func codexRawResponsesPayload(req contract.ConversationRequest) (map[string]any, error) {
	if !codexShouldUseRawResponsesPayload(req) {
		return nil, nil
	}
	raw := bytes.TrimSpace(req.RawBody)
	if len(raw) == 0 {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid raw responses payload"}
	}
	return payload, nil
}

func codexShouldUseRawResponsesPayload(req contract.ConversationRequest) bool {
	if !strings.EqualFold(strings.TrimSpace(req.SourceProtocol), "openai-compatible") {
		return false
	}
	sourceEndpoint := strings.ToLower(strings.TrimSpace(req.SourceEndpoint))
	return strings.HasSuffix(sourceEndpoint, "/responses") || strings.HasSuffix(sourceEndpoint, "/responses/compact")
}

func codexResponsesEndpoint(baseURL string, req contract.ConversationRequest) string {
	endpoint := "/responses"
	if codexResponsesCompactRequest(req) {
		endpoint = "/responses/compact"
	}
	return strings.TrimRight(baseURL, "/") + endpoint
}

func codexResponsesCompactRequest(req contract.ConversationRequest) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(req.SourceEndpoint)), "/responses/compact")
}

func codexResponsesPreviousResponseRecoveryPayload(req contract.ConversationRequest, payload map[string]any, responseBody []byte) (map[string]any, bool) {
	// sub2api's recoverPrevResponseNotFound (openai_gateway_service.go:2775)
	// gates only on "already tried once", "previous_response_id present",
	// and "no function_call_output". It does NOT skip /compact — and
	// skipping compact here was the exact reason Hermes' "remote compact
	// task" never self-healed when the prior turn's anchor outlived the
	// account it was bound to. Removing the carve-out so /compact gets
	// the same Layer-2 recovery path as /responses; the downstream
	// stateful/replayable input guards below still protect us against
	// dropping the anchor when the request can't be safely replayed.
	if payload == nil {
		return nil, false
	}
	if strings.TrimSpace(codexStringValue(payload["previous_response_id"])) == "" {
		return nil, false
	}
	if !codexResponseBodyPreviousResponseNotFound(responseBody) {
		return nil, false
	}
	if codexResponsesInputHasToolOutput(payload["input"]) ||
		codexResponsesInputHasStatefulContext(payload["input"]) ||
		!codexResponsesInputHasReplayableContent(payload["input"]) {
		return nil, false
	}
	retryPayload := cloneMap(payload)
	delete(retryPayload, "previous_response_id")
	return retryPayload, true
}

func codexResponseBodyPreviousResponseNotFound(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	var payload any
	if err := json.Unmarshal(trimmed, &payload); err == nil {
		return codexAnyValuePreviousResponseNotFound(payload)
	}
	text := strings.ToLower(string(trimmed))
	return strings.Contains(text, "previous_response_not_found") ||
		(strings.Contains(text, "previous response") && strings.Contains(text, "not found"))
}

func codexAnyValuePreviousResponseNotFound(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if lowerKey == "code" || lowerKey == "message" || lowerKey == "type" {
				if codexPreviousResponseNotFoundText(codexStringValue(item)) {
					return true
				}
			}
			if codexAnyValuePreviousResponseNotFound(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if codexAnyValuePreviousResponseNotFound(item) {
				return true
			}
		}
	case string:
		return codexPreviousResponseNotFoundText(typed)
	}
	return false
}

func codexPreviousResponseNotFoundText(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	return text == "previous_response_not_found" ||
		strings.Contains(text, "previous_response_not_found") ||
		(strings.Contains(text, "previous response") && strings.Contains(text, "not found"))
}

func codexResponsesInputHasToolOutput(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if codexResponsesInputHasToolOutput(item) {
				return true
			}
		}
	case map[string]any:
		if codexResponsesToolResultTypeIsSupported(strings.TrimSpace(codexStringValue(typed["type"]))) {
			return true
		}
		return codexResponsesInputHasToolOutput(typed["content"])
	}
	return false
}

func codexResponsesInputHasStatefulContext(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if codexResponsesInputHasStatefulContext(item) {
				return true
			}
		}
	case map[string]any:
		switch strings.TrimSpace(codexStringValue(typed["type"])) {
		case "item_reference", "reasoning", "function_call", "tool_call", "local_shell_call", "tool_search_call", "custom_tool_call", "mcp_tool_call":
			return true
		}
		return codexResponsesInputHasStatefulContext(typed["content"])
	}
	return false
}

func codexResponsesInputHasReplayableContent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if codexResponsesInputHasReplayableContent(item) {
				return true
			}
		}
	case map[string]any:
		itemType := strings.TrimSpace(codexStringValue(typed["type"]))
		switch itemType {
		case "message", "input_text", "output_text", "input_image":
			if codexInputItemText(typed) != "" {
				return true
			}
			if strings.TrimSpace(codexStringValue(typed["image_url"])) != "" || strings.TrimSpace(codexStringValue(typed["file_id"])) != "" {
				return true
			}
			return codexResponsesInputHasReplayableContent(typed["content"])
		default:
			return false
		}
	}
	return false
}

func codexCanonicalResponsesPayload(req contract.ConversationRequest) map[string]any {
	payload := map[string]any{
		"model":  req.Mapping.UpstreamModelName,
		"input":  codexResponsesInput(req),
		"stream": true,
	}
	if instructions := codexResponsesInstructions(req); instructions != "" {
		payload["instructions"] = instructions
	}
	if len(req.Stop) > 0 {
		payload["stop"] = cloneStrings(req.Stop)
	}
	if len(req.Tools) > 0 {
		payload["tools"] = cloneMapSlice(req.Tools)
	}
	if req.ToolChoice != nil {
		payload["tool_choice"] = cloneAny(req.ToolChoice)
	}
	if len(req.ResponseFormat) > 0 {
		payload["text"] = map[string]any{"format": cloneMap(req.ResponseFormat)}
	}
	if len(req.Reasoning) > 0 {
		payload["reasoning"] = cloneMap(req.Reasoning)
	}
	if promptCacheKey := requestSetting(req, "codex_prompt_cache_key", "prompt_cache_key"); promptCacheKey != "" {
		payload["prompt_cache_key"] = promptCacheKey
	}
	// Session-id spoofing: pin the upstream session to a stable per-conversation
	// id so consecutive turns are treated as one session (overrides any caller key).
	if spoof := strings.TrimSpace(req.SpoofSessionID); spoof != "" {
		payload["prompt_cache_key"] = spoof
	}
	return payload
}

func codexApplyClientMetadataSettings(req contract.ConversationRequest, payload map[string]any) {
	if payload == nil {
		return
	}
	metadata := codexPayloadClientMetadata(payload)
	setMetadata := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			metadata[key] = value
		}
	}
	setMetadata("x-codex-installation-id", requestSetting(req, "codex_installation_id", "x_codex_installation_id"))
	setMetadata("x-codex-turn-metadata", requestSetting(req, "codex_turn_metadata", "x_codex_turn_metadata", "X-Codex-Turn-Metadata"))
	setMetadata("x-codex-window-id", requestSetting(req, "codex_window_id", "x_codex_window_id", "X-Codex-Window-Id"))
	if betaFeatures := requestSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		setMetadata("x-codex-beta-features", betaFeatures)
	}
	if includeTiming := requestSetting(req, "x_responsesapi_include_timing_metrics", "X-ResponsesAPI-Include-Timing-Metrics"); includeTiming != "" {
		setMetadata("x-responsesapi-include-timing-metrics", includeTiming)
	}
	if len(metadata) > 0 {
		payload["client_metadata"] = metadata
	}
}

func codexPayloadClientMetadata(payload map[string]any) map[string]any {
	switch existing := payload["client_metadata"].(type) {
	case map[string]any:
		return existing
	case map[string]string:
		next := make(map[string]any, len(existing))
		for key, value := range existing {
			next[key] = value
		}
		return next
	default:
		return map[string]any{}
	}
}

func codexApplyResponsesPayloadDefaults(req contract.ConversationRequest, payload map[string]any) {
	if payload == nil {
		return
	}
	codexApplyClientMetadataSettings(req, payload)
	if model := contract.NormalizeCodexUpstreamModelName(req.Mapping.UpstreamModelName); model != "" {
		payload["model"] = model
	}
	// Component #5 (instructions normalization), ported from CLIProxyAPI
	// internal/runtime/executor/codex_executor_instructions_test.go: when the
	// caller supplies instructions: null (or an empty/whitespace string), the
	// upstream rejects the request unless we forward an explicit empty string.
	// codexEnsureResponsesInstructions below will substitute the model default
	// when the field is missing, so we only need to coerce typed-null and
	// whitespace into "" here.
	codexNormalizeInstructionsField(payload)
	codexNormalizeResponsesInput(payload)
	codexLiftInstructionInputItems(payload)
	codexNormalizeResponsesText(payload)
	// Mirror sub2api, which always sets text={verbosity:"medium"} and
	// reasoning.summary="auto". These are additive defaults: they never override
	// an existing text payload (e.g. text.format from response_format handling
	// above) or an existing reasoning.summary.
	if _, hasText := payload["text"]; !hasText {
		payload["text"] = map[string]any{"verbosity": "medium"}
	}
	if reasoning, ok := payload["reasoning"].(map[string]any); ok {
		if _, hasSummary := reasoning["summary"]; !hasSummary {
			reasoning["summary"] = "auto"
		}
	}
	// Drop text.verbosity when the resolved upstream model doesn't accept it
	// — mirrors sub2api's SupportsVerbosity gate
	// (openai_gateway_service.go:2631-2633). Models normalized to gpt-5.2 and
	// earlier reject `text.verbosity` with
	//	{"error":{"message":"Unknown parameter: 'text.verbosity'.","type":
	//	          "invalid_request_error","param":"text.verbosity",
	//	          "code":"unknown_parameter"}}
	// The current SRapi alias map remaps gpt-5/gpt-5.1 to gpt-5.4, so in
	// practice this only fires on gpt-5.2 today — but the helper also covers
	// the legacy aliases so a future remap revert can't silently regress.
	codexDropVerbosityForUnsupportedModel(payload)
	codexNormalizeServiceTier(req, payload)
	if !codexResponsesCompactRequest(req) {
		codexApplyImageGenerationBridgeTool(req, payload)
	}
	applyDisableImageGenerationToResponsesPayload(req, payload)
	codexNormalizeResponsesTools(payload)
	applyDisableImageGenerationToResponsesPayload(req, payload)
	// Compact and non-compact require completely different normalization.
	// sub2api openai_codex_transform.go:131-165 makes the rule explicit:
	// the compact endpoint is delete-only — strip the fields Codex rejects,
	// add NOTHING. The non-compact path is the additive one (store=false,
	// stream=true, parallel_tool_calls=true, include=[reasoning.encrypted_content],
	// default instructions, image_generation tool, etc.). Earlier srapi
	// borrowed CLIProxyAPI's translator and tried to add include + parallel
	// + instructions="" for compact too, which upstream Codex rejects with
	// {"error":{"code":"unknown_parameter","param":"include", ...}} —
	// diagnosed live against srapi.senran.net production traffic. This
	// commit aligns srapi with sub2api: compact is delete-only.
	if codexResponsesCompactRequest(req) {
		// sub2api applyCodexOAuthTransform openai_codex_transform.go:131-139:
		// for compact, delete store + stream; do nothing else.
		delete(payload, "store")
		delete(payload, "stream")
		// Codex /v1/responses/compact rejects srapi-specific client_metadata
		// (carries x-codex-installation-id / x-codex-turn-metadata /
		// x-codex-window-id from per-account request settings via
		// codexApplyClientMetadataSettings). Accepted on /responses, rejected
		// on /compact with {"code":"unknown_parameter","param":"client_metadata"}.
		// sub2api never reaches this populator for compact; in srapi
		// codexApplyClientMetadataSettings runs unconditionally so we
		// strip it back out here.
		delete(payload, "client_metadata")
	} else {
		codexApplyImageGenerationInstructions(payload)
		codexEnsureReasoningEncryptedInclude(payload)
		payload["store"] = codexResponsesDefaultInternalStoreValue
		payload["parallel_tool_calls"] = true
		codexEnsureResponsesInstructions(req, payload)
		payload["stream"] = true
	}
	for _, field := range codexUnsupportedResponsesFields() {
		delete(payload, field)
	}
}

// codexInstructionsNormalizedSentinel marks an explicit caller-supplied
// null/empty instructions string so codexEnsureResponsesInstructions will
// preserve it instead of substituting a default. Mirrors CLIProxyAPI's
// behaviour (see codex_executor_instructions_test.go).
type codexInstructionsNormalizedSentinel struct{}

// codexNormalizeInstructionsField rewrites a caller-supplied typed-null or
// whitespace-only instructions field to a normalized empty-string marker.
// codexEnsureResponsesInstructions inspects the marker and forwards "" to
// the upstream instead of substituting the model default.
func codexNormalizeInstructionsField(payload map[string]any) {
	if payload == nil {
		return
	}
	value, exists := payload["instructions"]
	if !exists {
		return
	}
	switch value.(type) {
	case nil:
		// Caller-supplied JSON null. CLIProxyAPI's
		// TestCodexExecutorExecuteNormalizesNullInstructions asserts that
		// the upstream receives "" — we mark this so
		// codexEnsureResponsesInstructions does not replace it with the
		// configured/model default. Whitespace-only strings are left to the
		// existing default-substitution path (srapi has historically used
		// the configured per-account default for whitespace, see
		// TestReverseProxyCodexCLIAdapterUsesConfiguredDefaultInstructions).
		payload["instructions"] = codexInstructionsNormalizedSentinel{}
	}
}

// codexInstructionsWasNormalizedEmpty reports whether the caller explicitly
// supplied null/empty/whitespace instructions, which must be forwarded as
// "" instead of replaced with the model default.
func codexInstructionsWasNormalizedEmpty(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	_, ok := payload["instructions"].(codexInstructionsNormalizedSentinel)
	return ok
}

func codexEnsureResponsesInstructions(req contract.ConversationRequest, payload map[string]any) {
	// Caller explicitly sent null/empty/whitespace instructions: forward an
	// empty string. Mirrors CLIProxyAPI's
	// TestCodexExecutorExecuteNormalizesNullInstructions assertion.
	if codexInstructionsWasNormalizedEmpty(payload) {
		payload["instructions"] = ""
		return
	}
	if strings.TrimSpace(codexStringValue(payload["instructions"])) != "" {
		return
	}
	if instructions := requestSetting(req, "codex_default_instructions", "default_instructions"); instructions != "" {
		payload["instructions"] = instructions
		return
	}
	// No caller-supplied prompt: fall back to the real Codex CLI base prompt for
	// this model. The upstream backend validates `instructions`, so a placeholder
	// gets rejected — this is what blocked gpt-5.5 and the other gpt-5.x models.
	payload["instructions"] = codexBaseInstructionsForModel(codexStringValue(payload["model"]))
}

func codexApplyImageGenerationInstructions(payload map[string]any) {
	if payload == nil {
		return
	}
	if contract.NormalizeCodexUpstreamModelName(codexStringValue(payload["model"])) == "gpt-5.3-codex-spark" {
		codexAppendInstructionsOnce(payload, codexSparkImageUnsupportedMarker, codexSparkImageUnsupportedText)
		return
	}
	if !codexResponsesToolsContainType(payload["tools"], "image_generation") {
		return
	}
	codexAppendInstructionsOnce(payload, codexImageGenerationBridgeMarker, codexImageGenerationBridgeText)
}

func codexApplyImageGenerationBridgeTool(req contract.ConversationRequest, payload map[string]any) {
	if payload == nil || !codexImageGenerationBridgeEnabled(req) || imageGenerationDisabledForConversation(req) {
		return
	}
	if contract.NormalizeCodexUpstreamModelName(codexStringValue(payload["model"])) == "gpt-5.3-codex-spark" {
		return
	}
	if codexResponsesToolsContainType(payload["tools"], "image_generation") {
		return
	}
	tool := map[string]any{
		"type":          "image_generation",
		"output_format": "png",
	}
	switch tools := payload["tools"].(type) {
	case []any:
		payload["tools"] = append(tools, tool)
	case []map[string]any:
		payload["tools"] = append(tools, tool)
	default:
		payload["tools"] = []any{tool}
	}
}

func codexImageGenerationBridgeEnabled(req contract.ConversationRequest) bool {
	for _, values := range []map[string]any{
		req.Account.Metadata,
		req.Provider.ConfigSchema,
		req.Provider.Capabilities,
	} {
		if values == nil {
			continue
		}
		for _, key := range []string{"codex_image_generation_bridge", "codex_image_generation_bridge_enabled"} {
			if _, ok := values[key]; !ok {
				continue
			}
			return mapBool(values, key)
		}
	}
	return false
}

func codexAppendInstructionsOnce(payload map[string]any, marker string, text string) {
	existing := strings.TrimRight(codexStringValue(payload["instructions"]), " \t\r\n")
	if strings.Contains(existing, marker) {
		return
	}
	if strings.TrimSpace(existing) == "" {
		payload["instructions"] = text
		return
	}
	payload["instructions"] = existing + "\n\n" + text
}

func codexEnsureReasoningEncryptedInclude(payload map[string]any) {
	if payload == nil {
		return
	}
	switch include := payload["include"].(type) {
	case nil:
		payload["include"] = []any{codexResponsesEncryptedReasoningInclude}
	case []any:
		for _, item := range include {
			if strings.TrimSpace(codexStringValue(item)) == codexResponsesEncryptedReasoningInclude {
				return
			}
		}
		payload["include"] = append(include, codexResponsesEncryptedReasoningInclude)
	case []string:
		for _, item := range include {
			if strings.TrimSpace(item) == codexResponsesEncryptedReasoningInclude {
				return
			}
		}
		next := make([]string, 0, len(include)+1)
		next = append(next, include...)
		next = append(next, codexResponsesEncryptedReasoningInclude)
		payload["include"] = next
	}
}

func codexUnsupportedResponsesFields() []string {
	return []string{
		"context_management",
		"frequency_penalty",
		"max_completion_tokens",
		"max_output_tokens",
		"metadata",
		"presence_penalty",
		"prompt_cache_retention",
		"response_format",
		"safety_identifier",
		"stream_options",
		"temperature",
		"top_p",
		"truncation",
		"user",
	}
}

func codexResponsesPayloadStream(payload map[string]any) bool {
	value, ok := payload["stream"].(bool)
	return !ok || value
}

func codexNormalizeResponsesText(payload map[string]any) {
	responseFormat, ok := payload["response_format"]
	if ok {
		if _, hasText := payload["text"]; !hasText {
			payload["text"] = map[string]any{"format": cloneAny(responseFormat)}
		}
	}
	codexEnsureResponsesTextFormatType(payload)
}

// codexEnsureResponsesTextFormatType backstops a long-standing upstream
// rejection on /v1/responses:
//
//	{"error":{"message":"Missing required parameter: 'text.format.type'.",
//	          "type":"invalid_request_error","param":"text.format.type",
//	          "code":"missing_required_parameter"}}
//
// (Diagnosed live against srapi.senran.net on req_c480b448... — provider 25,
// account 238, model gpt-5.5.) The Codex Responses upstream requires
// `text.format.type` whenever `text.format` is present. Two real-world
// shapes hit us:
//
//  1. Chat-completions-style payload forwarded raw to /v1/responses where
//     the caller's `response_format` was lifted into `text.format` but the
//     lift dropped the `type` field. CLIProxyAPI's chat-completions
//     translator (codex_openai_request.go:238-265) handles this case by
//     explicit-mapping `response_format.type` → `text.format.type`; we
//     mirror that mapping here so the same input shape is accepted on the
//     /responses path.
//
//  2. Caller sends a Codex Responses-native payload where `text.format`
//     carries `json_schema` (the legacy chat-completions wrapper) or a
//     bare `schema` instead of inline `type:"json_schema"` + `schema`.
//     Lift the wrapper into the outer format object and pin
//     `type:"json_schema"`.
//
// Fallback: when text.format exists but no type can be inferred (only
// verbosity, only an unknown field, etc.) coerce to `type:"text"` — the
// upstream default — instead of letting the request 400. The original
// caller fields are preserved; only the missing `type` is filled in.
func codexEnsureResponsesTextFormatType(payload map[string]any) {
	if payload == nil {
		return
	}
	text, ok := payload["text"].(map[string]any)
	if !ok {
		return
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		return
	}
	// Pre-pass: misplaced verbosity on text.format. The upstream
	// /v1/responses contract puts verbosity at `text.verbosity`, not
	// `text.format.verbosity` (live rejection req_ae057a05... — provider
	// 25, account 271, model gpt-5.5:
	//	{"error":{"message":"Unknown parameter: 'text.format.verbosity'.",
	//	          "type":"invalid_request_error",
	//	          "param":"text.format.verbosity",
	//	          "code":"unknown_parameter"}}
	// CLIProxyAPI's chat-completions translator (codex_openai_request.go:
	// 268-272) also reads source verbosity from text.verbosity, not
	// text.format.verbosity. Hoist it up before the type inference below
	// so the fallback ("only verbosity → type=text") still fires AFTER
	// the misplacement is fixed, and so the verbosity itself reaches the
	// upstream at the right path.
	if v, ok := format["verbosity"]; ok {
		if _, alreadySet := text["verbosity"]; !alreadySet {
			text["verbosity"] = v
		}
		delete(format, "verbosity")
	}
	// Shape (1): chat-completions response_format wrapper has
	// `json_schema:{name,strict,schema}`. Lift the wrapper into the format
	// object inline (sub2api / CLIProxyAPI parity) and pin type. Runs
	// BEFORE the type-already-set short-circuit because some callers send
	// both `type` and the wrapper — we still want the wrapper contents
	// lifted up so they reach the upstream at the right path.
	if wrapped, ok := format["json_schema"].(map[string]any); ok {
		format["type"] = "json_schema"
		for _, key := range []string{"name", "strict", "schema", "description"} {
			if value, ok := wrapped[key]; ok {
				if _, exists := format[key]; !exists {
					format[key] = cloneAny(value)
				}
			}
		}
		delete(format, "json_schema")
		// Continue to the whitelist pass below so any remaining unknown
		// fields the wrapper carried (e.g. response_format wrappers from
		// third-party clients that bolt on extras) get stripped.
	}
	// Type inference for shapes that didn't already set it.
	hasValidType := false
	if value, hasType := format["type"]; hasType {
		if typed, ok := value.(string); ok && strings.TrimSpace(typed) != "" {
			hasValidType = true
		}
	}
	if !hasValidType {
		switch {
		case codexFormatLooksLikeJSONSchema(format):
			// Shape (2): inline schema without `type`. Infer json_schema.
			format["type"] = "json_schema"
		default:
			// Fallback: unknown shape — coerce to "text" so the request is
			// not rejected outright.
			format["type"] = "text"
		}
	}
	// Final pass: strict whitelist on text.format. Upstream Codex /responses
	// rejects ANY unknown key on text.format with
	//	{"error":{"code":"unknown_parameter","param":"text.format.<key>"}}
	// The only fields the upstream contract accepts are {type, name,
	// strict, schema, description}. Anything else came from a forwarded
	// chat-completions wrapper that wasn't fully unwrapped, a third-party
	// client extension, or a misplaced caller field. Strip them after the
	// lift+inference above so the legitimate fields survive but stray
	// extras don't 400 the request.
	codexStripUnknownTextFormatFields(format)
}

// codexFormatLooksLikeJSONSchema returns true when text.format carries a
// concrete schema-shaped field — `schema` (the new Responses-native shape)
// or, defensively, a non-empty `json_schema` (callers that left the
// wrapper in despite our earlier lift). Used to choose between
// json_schema vs text when the caller forgot `type`.
func codexFormatLooksLikeJSONSchema(format map[string]any) bool {
	if _, ok := format["schema"]; ok {
		return true
	}
	if wrapped, ok := format["json_schema"].(map[string]any); ok && len(wrapped) > 0 {
		return true
	}
	return false
}

// codexDropVerbosityForUnsupportedModel removes `text.verbosity` when the
// payload's resolved upstream model does NOT accept the verbosity field.
// sub2api openai_gateway_service.go:2631-2633 gates verbosity by
// `SupportsVerbosity(upstreamModel)` for exactly this reason — the field
// was introduced in the gpt-5.3 generation and older models reject it
// with `Unknown parameter: 'text.verbosity'`. Runs after the
// default-substitution above so the additive default we just set is
// stripped back out when the resolved model can't accept it.
//
// If, after the strip, `text` becomes empty AND no format object was
// requested, drop `text` entirely so the request doesn't carry an
// empty text object the upstream would also reject.
func codexDropVerbosityForUnsupportedModel(payload map[string]any) {
	if payload == nil {
		return
	}
	model, _ := payload["model"].(string)
	if model == "" {
		return
	}
	if contract.CodexUpstreamModelSupportsVerbosity(model) {
		return
	}
	text, ok := payload["text"].(map[string]any)
	if !ok {
		return
	}
	if _, hasVerbosity := text["verbosity"]; !hasVerbosity {
		return
	}
	delete(text, "verbosity")
	if len(text) == 0 {
		delete(payload, "text")
	}
}

// codexStripUnknownTextFormatFields enforces the upstream /responses
// contract for text.format keys. Anything not in the allow-list is
// removed in-place. See codexEnsureResponsesTextFormatType for the live
// production rejection this guards.
func codexStripUnknownTextFormatFields(format map[string]any) {
	allowed := map[string]struct{}{
		"type":        {},
		"name":        {},
		"strict":      {},
		"schema":      {},
		"description": {},
	}
	for key := range format {
		if _, ok := allowed[key]; !ok {
			delete(format, key)
		}
	}
}

func codexNormalizeServiceTier(req contract.ConversationRequest, payload map[string]any) {
	if value, ok := payload["service_tier"].(string); ok {
		switch {
		case strings.EqualFold(strings.TrimSpace(value), "fast"):
			payload["service_tier"] = "priority"
		case !strings.EqualFold(strings.TrimSpace(value), "priority"):
			delete(payload, "service_tier")
		}
		return
	}
	if serviceTier := requestSetting(req, "codex_service_tier", "service_tier"); serviceTier != "" {
		switch {
		case strings.EqualFold(serviceTier, "fast"):
			serviceTier = "priority"
		case !strings.EqualFold(serviceTier, "priority"):
			return
		}
		payload["service_tier"] = serviceTier
	}
}

func codexNormalizeResponsesInput(payload map[string]any) {
	input, ok := payload["input"]
	if !ok || input == nil {
		payload["input"] = []any{}
		return
	}
	switch typed := input.(type) {
	case string:
		payload["input"] = codexStringInputMessage(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, codexNormalizeResponsesInputItem(item))
		}
		payload["input"] = out
	}
}

func codexStringInputMessage(text string) []any {
	text = strings.TrimSpace(text)
	if text == "" {
		return []any{}
	}
	return []any{map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": text,
		}},
	}}
}

func codexNormalizeResponsesInputItem(item any) any {
	object, ok := item.(map[string]any)
	if !ok {
		return item
	}
	out := cloneMap(object)
	roleValue := codexStringValue(out["role"])
	if strings.EqualFold(roleValue, "tool") {
		if toolOutput, ok := codexToolRoleMessageAsFunctionOutput(out); ok {
			return toolOutput
		}
		out["role"] = "user"
		roleValue = "user"
	}
	role := codexResponsesRole(roleValue)
	// Codex Responses upstream rejects role=system in the input array
	// (CLIProxyAPI translator codex_openai-responses_request.go:65-86
	// convertSystemRoleToDeveloper). Rewrite to "developer" before the
	// payload leaves the gateway; preserves the system-channel semantics
	// while satisfying the upstream contract.
	if role == "system" {
		out["role"] = "developer"
		role = "developer"
	}
	if _, hasType := out["type"]; !hasType && codexStringValue(out["role"]) != "" {
		out["type"] = "message"
	}
	if _, hasContent := out["content"]; hasContent {
		out["content"] = codexNormalizeMessageContent(out["content"], role)
	}
	return out
}

func codexToolRoleMessageAsFunctionOutput(item map[string]any) (map[string]any, bool) {
	callID := codexFirstString(item, "call_id", "tool_call_id", "id")
	if callID == "" {
		return nil, false
	}
	output := any("")
	if content, ok := item["content"]; ok {
		output = codexToolRoleOutput(content)
	} else if value, ok := item["output"]; ok && value != nil {
		output = codexToolRoleOutput(value)
	}
	out := map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	}
	if status := strings.TrimSpace(codexStringValue(item["status"])); status != "" {
		out["status"] = status
	}
	return out, true
}

func codexToolRoleOutput(value any) any {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		text := make([]string, 0, len(typed))
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch strings.TrimSpace(codexStringValue(part["type"])) {
			case "text", "input_text", "output_text":
				if partText := codexToolOutputTextValue(part["text"]); partText != "" {
					text = append(text, partText)
				}
			}
		}
		if len(text) > 0 {
			return strings.Join(text, "")
		}
		return codexJSONText(typed)
	case map[string]any:
		if text := codexToolOutputTextValue(typed["text"]); text != "" {
			return text
		}
		if content, ok := typed["content"]; ok {
			return codexToolRoleOutput(content)
		}
		return codexJSONText(typed)
	default:
		return codexJSONText(typed)
	}
}

func codexToolOutputTextValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return codexJSONText(typed)
	}
}

func codexNormalizeMessageContent(content any, role string) any {
	switch typed := content.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []any{}
		}
		return []any{map[string]any{
			"type": codexMessageContentType(role),
			"text": text,
		}}
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				out = append(out, item)
				continue
			}
			normalized := cloneMap(part)
			if text, ok := normalized["text"]; ok {
				normalized["text"] = codexInputTextValue(text)
			}
			out = append(out, normalized)
		}
		return out
	default:
		return content
	}
}

func codexInputTextValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return codexJSONText(typed)
	}
}

func codexJSONText(value any) string {
	if value == nil {
		return ""
	}
	if encoded, err := json.Marshal(value); err == nil {
		return string(encoded)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func codexFirstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(codexStringValue(values[key])); value != "" {
			return value
		}
	}
	return ""
}

func codexMessageContentType(role string) string {
	if codexResponsesRole(role) == "assistant" {
		return "output_text"
	}
	return "input_text"
}

func codexLiftInstructionInputItems(payload map[string]any) {
	input, ok := payload["input"].([]any)
	if !ok {
		return
	}
	instructions := []string{codexStringValue(payload["instructions"])}
	kept := make([]any, 0, len(input))
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		role := codexResponsesRole(codexStringValue(object["role"]))
		if role != "system" && role != "developer" {
			kept = append(kept, item)
			continue
		}
		if text := codexInputItemText(object["content"]); text != "" {
			instructions = append(instructions, text)
		}
	}
	payload["input"] = kept
	if joined := strings.Join(uniqueTrimmedStrings(instructions), "\n"); joined != "" {
		payload["instructions"] = joined
	}
}

func codexInputItemText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := codexInputItemText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text := codexStringValue(typed["text"]); text != "" {
			return text
		}
		return codexInputItemText(typed["content"])
	default:
		return ""
	}
}

func codexStringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func codexResponsesInput(req contract.ConversationRequest) []codexResponsesInputItem {
	out := make([]codexResponsesInputItem, 0, len(req.Messages)+1)
	for _, message := range req.Messages {
		role := codexResponsesRole(message.Role)
		if role == "system" || role == "developer" {
			continue
		}
		items := codexResponsesInputItemsFromMessage(role, message.Parts)
		if len(items) == 0 {
			continue
		}
		out = append(out, items...)
	}
	if len(out) == 0 {
		prompt := conversationPrompt(req)
		if prompt == "" {
			prompt = strings.TrimSpace(req.Instructions)
		}
		out = append(out, codexResponsesInputItem{
			Type:    "message",
			Role:    "user",
			Content: []codexResponsesInputContent{{Type: "input_text", Text: prompt}},
		})
	}
	return out
}

func codexResponsesInputItemsFromMessage(role string, parts []contract.ContentPart) []codexResponsesInputItem {
	out := make([]codexResponsesInputItem, 0, 1)
	messageContent := make([]codexResponsesInputContent, 0, len(parts))
	flushMessage := func() {
		if len(messageContent) == 0 {
			return
		}
		out = append(out, codexResponsesInputItem{
			Type:    "message",
			Role:    role,
			Content: messageContent,
		})
		messageContent = nil
	}
	for _, part := range parts {
		switch part.Kind {
		case contract.ContentPartToolUse:
			item, ok := codexResponsesFunctionCallItem(part)
			if !ok {
				continue
			}
			flushMessage()
			out = append(out, item)
		case contract.ContentPartToolResult:
			callID := strings.TrimSpace(firstNonEmpty(part.ToolResultForID, part.ToolCallID))
			if callID == "" {
				continue
			}
			itemType := codexResponsesToolResultType(part)
			flushMessage()
			// Codex requires function_call_output.output to be present; an empty
			// tool result would be dropped by omitempty and the upstream rejects
			// the turn. Default to "(empty)" like sub2api.
			output := part.Text
			if output == "" {
				output = "(empty)"
			}
			out = append(out, codexResponsesInputItem{
				Type:   itemType,
				CallID: callID,
				Output: output,
			})
		case contract.ContentPartMetadata:
			item, ok := codexResponsesRawInputItem(part)
			if !ok {
				continue
			}
			flushMessage()
			out = append(out, item)
		default:
			if content, ok := codexResponsesInputContentFromPart(role, part); ok {
				messageContent = append(messageContent, content)
			}
		}
	}
	flushMessage()
	return out
}

func codexResponsesFunctionCallItem(part contract.ContentPart) (codexResponsesInputItem, bool) {
	callID := strings.TrimSpace(part.ToolCallID)
	name := strings.TrimSpace(part.ToolName)
	arguments := part.ToolArgumentsJSON
	if callID == "" && name == "" && strings.TrimSpace(arguments) == "" {
		return codexResponsesInputItem{}, false
	}
	item := codexResponsesInputItem{
		Type:   codexResponsesToolCallType(part),
		CallID: callID,
		Name:   name,
	}
	if codexResponsesToolCallArgumentsField(part) == "input" {
		item.Input = arguments
	} else {
		// Codex requires function_call.arguments to be present and valid JSON;
		// an empty string would be dropped by omitempty and the upstream rejects
		// the tool call. Default to "{}" like sub2api.
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		item.Args = arguments
	}
	return item, true
}

func codexResponsesToolCallType(part contract.ContentPart) string {
	itemType := strings.TrimSpace(metadataString(part.Metadata, "type"))
	if codexResponsesToolCallTypeIsSupported(itemType) {
		return itemType
	}
	return "function_call"
}

func codexResponsesToolResultType(part contract.ContentPart) string {
	itemType := strings.TrimSpace(metadataString(part.Metadata, "type"))
	switch itemType {
	case "custom_tool_call_output", "mcp_tool_call_output", "tool_search_output":
		return itemType
	default:
		return "function_call_output"
	}
}

func codexResponsesToolCallArgumentsField(part contract.ContentPart) string {
	if strings.TrimSpace(metadataString(part.Metadata, "arguments_field")) == "input" ||
		codexResponsesToolCallType(part) == "custom_tool_call" {
		return "input"
	}
	return "arguments"
}

func codexResponsesToolCallTypeIsSupported(itemType string) bool {
	switch itemType {
	case "function_call", "custom_tool_call", "mcp_tool_call", "tool_call", "local_shell_call", "tool_search_call":
		return true
	default:
		return false
	}
}

func codexResponsesRawInputItem(part contract.ContentPart) (codexResponsesInputItem, bool) {
	if part.OriginProtocol != "openai-compatible" && part.OriginProtocol != "openai" {
		return codexResponsesInputItem{}, false
	}
	var item map[string]any
	if len(part.Raw) > 0 {
		if err := json.Unmarshal(part.Raw, &item); err != nil {
			return codexResponsesInputItem{}, false
		}
	} else {
		item = cloneMap(part.Metadata)
	}
	itemType := strings.TrimSpace(codexStringValue(item["type"]))
	if itemType == "" || itemType == "message" || itemType == "function_call" || itemType == "function_call_output" {
		return codexResponsesInputItem{}, false
	}
	return codexResponsesInputItem{Raw: item}, true
}

func codexNormalizeResponsesTools(payload map[string]any) {
	if payload == nil {
		return
	}
	codexMigrateLegacyFunctionFields(payload)
	normalizeOpenAIResponsesImageGenerationTools(payload)
	codexNormalizeResponsesBuiltinTools(payload)
	codexNormalizeResponsesToolSchemas(payload)
	codexNormalizeResponsesToolChoice(payload)
}

func codexMigrateLegacyFunctionFields(payload map[string]any) {
	if functions, ok := payload["functions"]; ok {
		if converted := codexLegacyFunctionsAsTools(functions); len(converted) > 0 {
			payload["tools"] = codexMergeResponseTools(payload["tools"], converted)
		}
		delete(payload, "functions")
	}
	if functionCall, ok := payload["function_call"]; ok {
		payload["tool_choice"] = codexFunctionCallAsToolChoice(functionCall)
		delete(payload, "function_call")
	}
}

func codexLegacyFunctionsAsTools(value any) []any {
	functions, ok := codexAnySlice(value)
	if !ok {
		return nil
	}
	tools := make([]any, 0, len(functions))
	for _, rawFunction := range functions {
		function, ok := rawFunction.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(codexStringValue(function["name"]))
		if name == "" {
			continue
		}
		tool := map[string]any{
			"type": "function",
			"name": name,
		}
		if description := strings.TrimSpace(codexStringValue(function["description"])); description != "" {
			tool["description"] = description
		}
		if parameters, ok := function["parameters"]; ok {
			tool["parameters"] = cloneAny(parameters)
		}
		if strict, ok := function["strict"]; ok {
			tool["strict"] = strict
		}
		tools = append(tools, tool)
	}
	return tools
}

func codexMergeResponseTools(existing any, additions []any) []any {
	tools, ok := codexAnySlice(existing)
	if !ok {
		tools = nil
	}
	out := make([]any, 0, len(tools)+len(additions))
	seenFunctionNames := map[string]bool{}
	for _, tool := range tools {
		out = append(out, tool)
		if name := codexResponseFunctionToolName(tool); name != "" {
			seenFunctionNames[name] = true
		}
	}
	for _, tool := range additions {
		name := codexResponseFunctionToolName(tool)
		if name != "" && seenFunctionNames[name] {
			continue
		}
		out = append(out, tool)
		if name != "" {
			seenFunctionNames[name] = true
		}
	}
	return out
}

func codexFunctionCallAsToolChoice(value any) any {
	switch typed := value.(type) {
	case string:
		choice := strings.TrimSpace(typed)
		if choice == "" {
			return "auto"
		}
		return choice
	case map[string]any:
		name := strings.TrimSpace(codexStringValue(typed["name"]))
		if name == "" {
			return "auto"
		}
		return map[string]any{"type": "function", "name": name}
	default:
		return "auto"
	}
}

func codexNormalizeResponsesToolSchemas(payload map[string]any) {
	tools, ok := codexAnySlice(payload["tools"])
	if !ok {
		return
	}
	normalized := make([]any, 0, len(tools))
	modified := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			normalized = append(normalized, rawTool)
			continue
		}
		if strings.TrimSpace(codexStringValue(tool["type"])) != "function" {
			normalized = append(normalized, tool)
			continue
		}
		next, keep, changed := codexNormalizeResponsesFunctionTool(tool)
		modified = modified || changed || !keep
		if keep {
			normalized = append(normalized, next)
		}
	}
	if modified {
		payload["tools"] = normalized
	}
}

func codexNormalizeResponsesFunctionTool(tool map[string]any) (map[string]any, bool, bool) {
	out := cloneMap(tool)
	function, _ := out["function"].(map[string]any)
	name := strings.TrimSpace(codexStringValue(out["name"]))
	if name == "" && function != nil {
		name = strings.TrimSpace(codexStringValue(function["name"]))
	}
	if name == "" {
		return nil, false, true
	}
	changed := false
	if strings.TrimSpace(codexStringValue(out["name"])) != name {
		out["name"] = name
		changed = true
	}
	if function != nil {
		for _, key := range []string{"description", "parameters", "strict"} {
			if _, exists := out[key]; !exists {
				if value, ok := function[key]; ok {
					out[key] = cloneAny(value)
					changed = true
				}
			}
		}
		delete(out, "function")
		changed = true
	}
	return out, true, changed
}

func codexNormalizeResponsesBuiltinTools(payload map[string]any) {
	if tools, ok := codexAnySlice(payload["tools"]); ok {
		modified := false
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				continue
			}
			modified = codexNormalizeResponsesBuiltinTool(tool) || modified
		}
		if modified {
			payload["tools"] = tools
		}
	}
	choiceMap, ok := payload["tool_choice"].(map[string]any)
	if !ok {
		return
	}
	codexNormalizeResponsesBuiltinTool(choiceMap)
	tools, ok := codexAnySlice(choiceMap["tools"])
	if !ok {
		return
	}
	modified := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		modified = codexNormalizeResponsesBuiltinTool(tool) || modified
	}
	if modified {
		choiceMap["tools"] = tools
	}
}

func codexNormalizeResponsesBuiltinTool(tool map[string]any) bool {
	current := strings.TrimSpace(codexStringValue(tool["type"]))
	normalized := codexNormalizeResponsesBuiltinToolType(current)
	if normalized == "" {
		return false
	}
	tool["type"] = normalized
	return true
}

func codexNormalizeResponsesBuiltinToolType(toolType string) string {
	switch strings.TrimSpace(toolType) {
	case "web_search_preview", "web_search_preview_2025_03_11":
		return "web_search"
	default:
		return ""
	}
}

func codexNormalizeResponsesToolChoice(payload map[string]any) {
	choice, ok := payload["tool_choice"]
	if !ok || choice == nil {
		return
	}
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return
	}
	choiceType := strings.TrimSpace(codexStringValue(choiceMap["type"]))
	if choiceType == "" {
		payload["tool_choice"] = "auto"
		return
	}
	if choiceType == "allowed_tools" || codexResponsesBuiltinToolChoiceType(choiceType) {
		return
	}
	if choiceType == "function" {
		name := strings.TrimSpace(codexStringValue(choiceMap["name"]))
		if name == "" {
			if function, ok := choiceMap["function"].(map[string]any); ok {
				name = strings.TrimSpace(codexStringValue(function["name"]))
			}
		}
		if name == "" || !codexResponsesToolsContainFunction(payload["tools"], name) {
			payload["tool_choice"] = "auto"
			return
		}
		payload["tool_choice"] = map[string]any{"type": "function", "name": name}
		return
	}
	if !codexResponsesToolsContainType(payload["tools"], choiceType) {
		payload["tool_choice"] = "auto"
	}
}

func codexResponsesBuiltinToolChoiceType(toolType string) bool {
	return strings.TrimSpace(toolType) == "web_search"
}

func codexResponsesToolsContainFunction(tools any, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, rawTool := range codexResponseToolItems(tools) {
		if codexResponseFunctionToolName(rawTool) == name {
			return true
		}
	}
	return false
}

func codexResponsesToolsContainType(tools any, toolType string) bool {
	toolType = strings.TrimSpace(toolType)
	if toolType == "" {
		return false
	}
	for _, rawTool := range codexResponseToolItems(tools) {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(codexStringValue(tool["type"])) == toolType {
			return true
		}
	}
	return false
}

func codexResponseToolItems(tools any) []any {
	items, ok := codexAnySlice(tools)
	if !ok {
		return nil
	}
	return items
}

func codexResponseFunctionToolName(rawTool any) string {
	tool, ok := rawTool.(map[string]any)
	if !ok || strings.TrimSpace(codexStringValue(tool["type"])) != "function" {
		return ""
	}
	if name := strings.TrimSpace(codexStringValue(tool["name"])); name != "" {
		return name
	}
	if function, ok := tool["function"].(map[string]any); ok {
		return strings.TrimSpace(codexStringValue(function["name"]))
	}
	return ""
}

func codexAnySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}

func codexResponsesInputContentFromPart(role string, part contract.ContentPart) (codexResponsesInputContent, bool) {
	switch part.Kind {
	case "", contract.ContentPartText, contract.ContentPartThinking, contract.ContentPartRefusal:
		if text := strings.TrimSpace(part.Text); text != "" {
			return codexResponsesTextContent(role, text), true
		}
	case contract.ContentPartImage:
		if url := mediaURLValue(part); url != "" {
			return codexResponsesInputContent{Type: "input_image", ImageURL: url}, true
		}
		if fileID := strings.TrimSpace(part.FileID); fileID != "" {
			return codexResponsesInputContent{Type: "input_image", FileID: fileID}, true
		}
		if text := strings.TrimSpace(part.Text); text != "" {
			return codexResponsesTextContent(role, text), true
		}
	case contract.ContentPartFile:
		if text := strings.TrimSpace(part.Text); text != "" {
			return codexResponsesTextContent(role, text), true
		}
	default:
		if text := strings.TrimSpace(part.Text); text != "" {
			return codexResponsesTextContent(role, text), true
		}
	}
	return codexResponsesInputContent{}, false
}

func codexResponsesTextContent(role string, text string) codexResponsesInputContent {
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return codexResponsesInputContent{Type: contentType, Text: strings.TrimSpace(text)}
}

func codexResponsesRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "developer":
		return "developer"
	default:
		return "user"
	}
}

func codexResponsesInstructions(req contract.ConversationRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, message := range req.Messages {
		role := codexResponsesRole(message.Role)
		if role != "system" && role != "developer" {
			continue
		}
		if content := conversationMessageText(message); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(uniqueTrimmedStrings(parts), "\n")
}
