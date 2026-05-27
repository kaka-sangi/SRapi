package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const (
	codexOriginator                         = "codex_cli_rs"
	codexDefaultVersion                     = "0.125.0"
	codexDefaultUserAgent                   = codexOriginator + "/" + codexDefaultVersion
	codexResponsesBetaHeaderValue           = "responses=experimental"
	codexResponsesWebsocketBetaHeaderValue  = "responses_websockets=2026-02-06"
	codexDefaultAccountSessionIDPrefix      = "srapi-codex-account-"
	codexResponsesDefaultInternalStoreValue = false
)

type codexResponsesInputItem struct {
	Type    string                       `json:"type"`
	Role    string                       `json:"role,omitempty"`
	Content []codexResponsesInputContent `json:"content,omitempty"`
	CallID  string                       `json:"call_id,omitempty"`
	Name    string                       `json:"name,omitempty"`
	Args    string                       `json:"arguments,omitempty"`
	Output  string                       `json:"output,omitempty"`
	Raw     map[string]any               `json:"-"`
}

func (item codexResponsesInputItem) MarshalJSON() ([]byte, error) {
	if len(item.Raw) > 0 {
		return json.Marshal(item.Raw)
	}
	type alias codexResponsesInputItem
	return json.Marshal(alias(item))
}

type codexResponsesInputContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

type codexResponsesEvent struct {
	Type         string                    `json:"type"`
	Delta        string                    `json:"delta"`
	Text         string                    `json:"text"`
	Refusal      string                    `json:"refusal"`
	ItemID       string                    `json:"item_id"`
	PartialImage string                    `json:"partial_image_b64"`
	OutputFormat string                    `json:"output_format"`
	Background   string                    `json:"background"`
	Item         *codexResponsesOutputItem `json:"item"`
	OutputIndex  *int                      `json:"output_index"`
	ContentIndex *int                      `json:"content_index"`
	PartialIndex any                       `json:"partial_image_index"`
	Annotation   map[string]any            `json:"annotation,omitempty"`
	Response     *codexResponsesResponse   `json:"response"`
	Usage        *openAIUsage              `json:"usage"`
	Error        *codexResponsesError      `json:"error"`
	Message      string                    `json:"message"`
	Code         string                    `json:"code"`
}

type codexResponsesResponse struct {
	Output            []codexResponsesOutputItem `json:"output"`
	OutputText        string                     `json:"output_text"`
	Status            string                     `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage openAIUsage          `json:"usage"`
	Error *codexResponsesError `json:"error"`
}

type codexResponsesOutputItem struct {
	ID           string                        `json:"id"`
	Type         string                        `json:"type"`
	CallID       string                        `json:"call_id"`
	Name         string                        `json:"name"`
	Arguments    string                        `json:"arguments"`
	Status       string                        `json:"status"`
	Text         string                        `json:"text"`
	Refusal      string                        `json:"refusal"`
	Result       string                        `json:"result"`
	OutputFormat string                        `json:"output_format"`
	Content      []codexResponsesOutputContent `json:"content"`
	Annotations  []map[string]any              `json:"-"`
}

type codexResponsesOutputContent struct {
	Type        string           `json:"type"`
	Text        string           `json:"text"`
	Refusal     string           `json:"refusal"`
	Annotations []map[string]any `json:"annotations,omitempty"`
}

type codexResponsesError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Type    string `json:"type"`
}

func (s *Service) invokeReverseProxyCodexResponses(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if codexReverseProxyRuntimeIsAPIKey(req) {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	payload, stream, err := codexResponsesPayload(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account:      codexReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(baseURL, "/") + "/responses",
		Headers:      codexResponsesHeaders(req, stream),
		Body:         raw,
		ExpectStream: stream,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	return parseCodexResponsesBody(runtimeResp.Body, runtimeResp.StatusCode)
}

func (s *Service) prepareCodexRealtime(_ context.Context, req contract.RealtimeRequest, baseURL string) (contract.RealtimeSession, error) {
	if codexRealtimeRuntimeIsAPIKey(req) {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex websocket reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	if len(bytes.TrimSpace(req.RequestPayload)) == 0 {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex websocket request payload missing"}
	}
	wsURL, err := codexResponsesWebSocketURL(strings.TrimRight(baseURL, "/") + "/responses")
	if err != nil {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	headers := codexRealtimeHeaders(req)
	return contract.RealtimeSession{
		URL:          wsURL,
		Headers:      headers,
		InitialFrame: codexRealtimeInitialFrame(req.RequestPayload, req.Mapping.UpstreamModelName),
	}, nil
}

func codexResponsesPayload(req contract.ConversationRequest) (map[string]any, bool, error) {
	payload, err := codexRawResponsesPayload(req)
	if err != nil {
		return nil, false, err
	}
	if payload == nil {
		payload = codexCanonicalResponsesPayload(req)
	}
	codexApplyResponsesPayloadDefaults(req, payload)
	return payload, codexResponsesPayloadStream(payload), nil
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
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(req.SourceEndpoint)), "/responses")
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
	return payload
}

func codexApplyResponsesPayloadDefaults(req contract.ConversationRequest, payload map[string]any) {
	if payload == nil {
		return
	}
	if model := strings.TrimSpace(req.Mapping.UpstreamModelName); model != "" {
		payload["model"] = model
	}
	codexNormalizeResponsesInput(payload)
	codexLiftInstructionInputItems(payload)
	codexNormalizeResponsesText(payload)
	codexNormalizeServiceTier(req, payload)
	payload["stream"] = true
	payload["store"] = codexResponsesDefaultInternalStoreValue
	for _, field := range codexUnsupportedResponsesFields() {
		delete(payload, field)
	}
}

func codexUnsupportedResponsesFields() []string {
	return []string{
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
		"user",
	}
}

func codexResponsesPayloadStream(payload map[string]any) bool {
	value, ok := payload["stream"].(bool)
	return !ok || value
}

func codexNormalizeResponsesText(payload map[string]any) {
	responseFormat, ok := payload["response_format"]
	if !ok {
		return
	}
	if _, hasText := payload["text"]; !hasText {
		payload["text"] = map[string]any{"format": cloneAny(responseFormat)}
	}
}

func codexNormalizeServiceTier(req contract.ConversationRequest, payload map[string]any) {
	if value, ok := payload["service_tier"].(string); ok {
		if strings.EqualFold(strings.TrimSpace(value), "fast") {
			payload["service_tier"] = "priority"
		}
		return
	}
	if serviceTier := requestSetting(req, "codex_service_tier", "service_tier"); serviceTier != "" {
		if strings.EqualFold(serviceTier, "fast") {
			serviceTier = "priority"
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
	role := codexResponsesRole(codexStringValue(out["role"]))
	if _, hasType := out["type"]; !hasType && codexStringValue(out["role"]) != "" {
		out["type"] = "message"
	}
	if _, hasContent := out["content"]; hasContent {
		out["content"] = codexNormalizeMessageContent(out["content"], role)
	}
	return out
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
				normalized["text"] = codexInputItemText(text)
			}
			out = append(out, normalized)
		}
		return out
	default:
		return content
	}
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
			flushMessage()
			out = append(out, codexResponsesInputItem{
				Type:   "function_call_output",
				CallID: callID,
				Output: strings.TrimSpace(part.Text),
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
	arguments := strings.TrimSpace(part.ToolArgumentsJSON)
	if callID == "" && name == "" && arguments == "" {
		return codexResponsesInputItem{}, false
	}
	return codexResponsesInputItem{
		Type:   "function_call",
		CallID: callID,
		Name:   name,
		Args:   arguments,
	}, true
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

func codexResponsesHeaders(req contract.ConversationRequest, stream bool) http.Header {
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	headers := http.Header{
		"Accept":       {accept},
		"Content-Type": {"application/json"},
	}
	headers.Set("OpenAI-Beta", codexResponsesBetaHeaderValue)
	headers.Set("Originator", codexOriginator)
	headers.Set("User-Agent", codexUserAgent(req))
	if accountID := requestSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	if betaFeatures := requestSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		headers.Set("X-Codex-Beta-Features", betaFeatures)
	}
	if version := requestSetting(req, "codex_version", "version", "Version"); version != "" {
		headers.Set("Version", version)
	} else {
		headers.Set("Version", codexDefaultVersion)
	}
	if turnMetadata := requestSetting(req, "codex_turn_metadata", "x_codex_turn_metadata", "X-Codex-Turn-Metadata"); turnMetadata != "" {
		headers.Set("X-Codex-Turn-Metadata", turnMetadata)
	}
	if requestID := requestSetting(req, "codex_client_request_id", "x_client_request_id", "X-Client-Request-Id"); requestID != "" {
		headers.Set("X-Client-Request-Id", requestID)
	} else if strings.TrimSpace(req.RequestID) != "" {
		headers.Set("X-Client-Request-Id", strings.TrimSpace(req.RequestID))
	}
	if sessionID := requestSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("Session_id", sessionID)
	} else if req.Account.ID > 0 {
		headers.Set("Session_id", codexDefaultAccountSessionID(req.Account.ID))
	}
	return headers
}

func codexUserAgent(req contract.ConversationRequest) string {
	if userAgent := requestSetting(req, "user_agent"); userAgent != "" {
		return userAgent
	}
	return codexDefaultUserAgent
}

func codexDefaultAccountSessionID(accountID int) string {
	if accountID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%d", codexDefaultAccountSessionIDPrefix, accountID)
}

func codexReverseProxyRuntimeIsAPIKey(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func codexRealtimeRuntimeIsAPIKey(req contract.RealtimeRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func codexReverseProxyAccount(req contract.ConversationRequest) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: req.Account.UpstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Metadata:       req.Account.Metadata,
		Credential:     req.Credential,
	}
}

func codexRealtimeHeaders(req contract.RealtimeRequest) http.Header {
	headers := http.Header{
		"OpenAI-Beta": {codexResponsesWebsocketBetaHeaderValue},
	}
	headers.Set("Originator", codexOriginator)
	if accountID := realtimeSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	if betaFeatures := realtimeSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		headers.Set("X-Codex-Beta-Features", betaFeatures)
	}
	if version := realtimeSetting(req, "codex_version", "version", "Version"); version != "" {
		headers.Set("Version", version)
	}
	if turnMetadata := realtimeSetting(req, "codex_turn_metadata", "x_codex_turn_metadata", "X-Codex-Turn-Metadata"); turnMetadata != "" {
		headers.Set("X-Codex-Turn-Metadata", turnMetadata)
	}
	if requestID := realtimeSetting(req, "codex_client_request_id", "x_client_request_id", "X-Client-Request-Id"); requestID != "" {
		headers.Set("X-Client-Request-Id", requestID)
	} else if strings.TrimSpace(req.RequestID) != "" {
		headers.Set("X-Client-Request-Id", strings.TrimSpace(req.RequestID))
	}
	if includeTiming := realtimeSetting(req, "x_responsesapi_include_timing_metrics", "X-ResponsesAPI-Include-Timing-Metrics"); includeTiming != "" {
		headers.Set("X-ResponsesAPI-Include-Timing-Metrics", includeTiming)
	}
	if sessionID := realtimeSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("session_id", sessionID)
	} else if strings.Contains(realtimeSetting(req, "user_agent"), "Mac OS") && strings.TrimSpace(req.RequestID) != "" {
		headers.Set("session_id", strings.TrimSpace(req.RequestID))
	}
	return headers
}

func codexRealtimeInitialFrame(payload []byte, upstreamModel string) []byte {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(payload), &object); err != nil {
		return append([]byte(nil), payload...)
	}
	encodedType, err := json.Marshal("response.create")
	if err != nil {
		return append([]byte(nil), payload...)
	}
	object["type"] = encodedType
	if model := strings.TrimSpace(upstreamModel); model != "" {
		encodedModel, err := json.Marshal(model)
		if err != nil {
			return append([]byte(nil), payload...)
		}
		object["model"] = encodedModel
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return append([]byte(nil), payload...)
	}
	return encoded
}

func codexResponsesWebSocketURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("codex websocket upstream URL scheme %q is unsupported", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("codex websocket upstream URL host is empty")
	}
	return parsed.String(), nil
}

func realtimeSetting(req contract.RealtimeRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func parseCodexResponsesBody(body []byte, statusCode int) (contract.ConversationResponse, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:")) {
		return parseCodexResponsesStream(body, statusCode)
	}
	return parseCodexResponsesJSON(trimmed, statusCode)
}

func parseCodexResponsesStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	var deltaBuilder strings.Builder
	var completedText string
	var reasoningBuilder strings.Builder
	var completedReasoning string
	var refusalBuilder strings.Builder
	var completedRefusal string
	var usage *openAIUsage
	indexedItems := map[int]codexResponsesOutputItem{}
	fallbackItems := []codexResponsesOutputItem{}
	textAnnotationsByIndex := map[codexTextAnnotationKey][]map[string]any{}
	var finalResponse *codexResponsesResponse
	streamEvents := make([]contract.ConversationStreamEvent, 0)
	functionStates := newCodexFunctionCallStreamStates()
	eventIndex := 0
	seenEvent := false
	appendStreamEvent := func(event contract.ConversationStreamEvent) {
		event.Index = eventIndex
		streamEvents = append(streamEvents, event)
		eventIndex++
	}
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		eventType := frame.EventType(event.Type)
		event.Type = eventType
		seenEvent = true
		if providerErr, ok := codexEventProviderError(event); ok {
			return contract.ConversationResponse{}, providerErr
		}
		if event.Usage != nil && event.Usage.HasTokenUsage() {
			copied := *event.Usage
			usage = &copied
			appendStreamEvent(codexStreamUsageEvent(copied, data, deltaBuilder.String()))
		}
		functionStates.mergeEvent(event)
		if event.Response != nil {
			copiedResponse := *event.Response
			if len(copiedResponse.Output) == 0 {
				copiedResponse.Output = codexCollectedOutputItems(indexedItems, fallbackItems)
			}
			copiedResponse = codexResponseWithStreamAnnotations(copiedResponse, textAnnotationsByIndex)
			finalResponse = &copiedResponse
			if copiedResponse.Usage.HasTokenUsage() {
				copiedUsage := copiedResponse.Usage
				usage = &copiedUsage
				appendStreamEvent(codexStreamUsageEvent(copiedUsage, data, deltaBuilder.String()))
			}
		}
		switch eventType {
		case "response.output_item.added":
			if streamEvent, ok := functionStates.startEvent(event, eventType, data); ok {
				appendStreamEvent(streamEvent)
			}
		case "response.output_item.done":
			if event.Item != nil {
				item := codexOutputItemWithStreamAnnotations(*event.Item, codexOutputIndex(event), textAnnotationsByIndex)
				if event.OutputIndex != nil {
					indexedItems[*event.OutputIndex] = item
				} else {
					fallbackItems = append(fallbackItems, item)
				}
				if codexOutputItemIsFunctionCall(item) && !functionStates.hasArgumentDeltas(event) {
					if streamEvent, ok := codexFunctionCallStreamEvent(item, codexOutputIndex(event), data); ok {
						appendStreamEvent(streamEvent)
					}
				}
			}
		case "response.image_generation_call.partial_image":
			if streamEvent, ok := codexImageGenerationPartialStreamEvent(event, eventType, data); ok {
				appendStreamEvent(streamEvent)
			}
		case "response.output_text.delta":
			deltaBuilder.WriteString(event.Delta)
			if event.Delta != "" {
				appendStreamEvent(codexContentStreamEvent(event, eventType, data, textContentDelta(event.Delta)))
			}
		case "response.output_text.annotation.added":
			if len(event.Annotation) > 0 {
				key := codexTextAnnotationKeyForEvent(event)
				annotation := cloneMap(event.Annotation)
				textAnnotationsByIndex[key] = append(textAnnotationsByIndex[key], annotation)
				appendStreamEvent(codexContentStreamEvent(event, eventType, data, codexAnnotationContentDelta(annotation)))
			}
		case "response.refusal.delta":
			refusalBuilder.WriteString(event.Delta)
			if event.Delta != "" {
				appendStreamEvent(codexContentStreamEvent(event, eventType, data, contract.ContentPart{
					Kind:           contract.ContentPartRefusal,
					Text:           event.Delta,
					OriginProtocol: "openai-compatible",
				}))
			}
		case "response.reasoning_text.delta":
			reasoningBuilder.WriteString(event.Delta)
			if event.Delta != "" {
				appendStreamEvent(codexReasoningStreamEvent(event, eventType, data))
			}
		case "response.function_call_arguments.delta":
			if event.Delta != "" {
				appendStreamEvent(functionStates.deltaEvent(event, eventType, data))
			}
		case "response.output_text.done":
			if strings.TrimSpace(event.Text) != "" {
				completedText = event.Text
			}
		case "response.refusal.done":
			if strings.TrimSpace(event.Refusal) != "" {
				completedRefusal = event.Refusal
			}
		case "response.reasoning_text.done":
			if strings.TrimSpace(event.Text) != "" {
				completedReasoning = event.Text
			}
		case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled":
			if text := codexEventText(event); strings.TrimSpace(text) != "" {
				completedText = text
			}
			appendStreamEvent(contract.ConversationStreamEvent{
				Type:           contract.ConversationStreamEventStop,
				StopReason:     codexStreamStopReason(event, completedRefusal, refusalBuilder.String()),
				RawEventType:   eventType,
				Raw:            append(json.RawMessage(nil), data...),
				OriginProtocol: "openai-compatible",
			})
		default:
			if text := codexEventText(event); strings.TrimSpace(text) != "" && strings.TrimSpace(completedText) == "" {
				completedText = text
			}
		}
	}
	if !seenEvent {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	parts := []contract.ContentPart(nil)
	stopReason := contract.StopReasonEndTurn
	if finalResponse != nil {
		parts = finalResponse.Parts()
		stopReason = codexStopReason(*finalResponse)
	}
	if len(parts) == 0 {
		collectedItems := codexCollectedOutputItems(indexedItems, fallbackItems)
		parts = codexResponsesOutputItemsParts(collectedItems)
		if codexOutputItemsIncludeFunctionCall(collectedItems) {
			stopReason = contract.StopReasonToolUse
		} else if codexOutputItemsIncludeRefusal(collectedItems) {
			stopReason = contract.StopReasonRefusal
		}
	}
	if len(parts) == 0 {
		text := strings.TrimSpace(completedText)
		if text == "" {
			text = strings.TrimSpace(deltaBuilder.String())
		}
		if text != "" {
			part := textContentPart(text)
			part.Metadata = codexCombinedStreamAnnotationsMetadata(textAnnotationsByIndex)
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		refusalText := strings.TrimSpace(completedRefusal)
		if refusalText == "" {
			refusalText = strings.TrimSpace(refusalBuilder.String())
		}
		if refusalText != "" {
			parts = append(parts, contract.ContentPart{Kind: contract.ContentPartRefusal, Text: refusalText, OriginProtocol: "openai"})
			stopReason = contract.StopReasonRefusal
		}
	}
	parts = prependCodexReasoningPart(parts, completedReasoning, reasoningBuilder.String())
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(parts)
	parsedUsage := estimatedUsage(text)
	if usage != nil {
		parsedUsage = usage.ToUsage(text)
	}
	if len(streamEvents) > 0 && streamEvents[len(streamEvents)-1].Type != contract.ConversationStreamEventStop {
		streamEvents = append(streamEvents, contract.ConversationStreamEvent{
			Index:          eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     stopReason,
			RawEventType:   "done",
			OriginProtocol: "openai-compatible",
		})
	}
	return contract.ConversationResponse{
		Parts:        parts,
		StopReason:   stopReason,
		StatusCode:   statusCode,
		Usage:        parsedUsage,
		Raw:          append(json.RawMessage(nil), body...),
		StreamEvents: streamEvents,
	}, nil
}

func prependCodexReasoningPart(parts []contract.ContentPart, completedReasoning string, streamedReasoning string) []contract.ContentPart {
	reasoningText := strings.TrimSpace(completedReasoning)
	if reasoningText == "" {
		reasoningText = strings.TrimSpace(streamedReasoning)
	}
	if reasoningText == "" {
		return parts
	}
	for _, part := range parts {
		if part.Kind == contract.ContentPartThinking && strings.TrimSpace(part.Text) == reasoningText {
			return parts
		}
	}
	reasoningPart := contract.ContentPart{Kind: contract.ContentPartThinking, Text: reasoningText, OriginProtocol: "openai"}
	return append([]contract.ContentPart{reasoningPart}, parts...)
}

func codexContentStreamEvent(event codexResponsesEvent, eventType string, raw string, delta contract.ContentPart) contract.ConversationStreamEvent {
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventContentDelta,
		ContentIndex:   codexOutputIndex(event),
		Delta:          delta,
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}
}

func codexAnnotationContentDelta(annotation map[string]any) contract.ContentPart {
	return contract.ContentPart{
		Kind:           contract.ContentPartText,
		Metadata:       map[string]any{"annotations": []map[string]any{cloneMap(annotation)}},
		OriginProtocol: "openai-compatible",
	}
}

func codexImageGenerationPartialStreamEvent(event codexResponsesEvent, eventType string, raw string) (contract.ConversationStreamEvent, bool) {
	partial := strings.TrimSpace(event.PartialImage)
	if partial == "" {
		return contract.ConversationStreamEvent{}, false
	}
	metadata := map[string]any{
		"type":              eventType,
		"partial_image_b64": partial,
	}
	if itemID := strings.TrimSpace(event.ItemID); itemID != "" {
		metadata["item_id"] = itemID
	}
	if format := strings.TrimSpace(event.OutputFormat); format != "" {
		metadata["output_format"] = format
	}
	if background := strings.TrimSpace(event.Background); background != "" {
		metadata["background"] = background
	}
	if event.PartialIndex != nil {
		metadata["partial_image_index"] = event.PartialIndex
	}
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventContentDelta,
		ContentIndex:   codexOutputIndex(event),
		Delta:          contract.ContentPart{Kind: contract.ContentPartImage, Metadata: metadata, OriginProtocol: "openai-compatible"},
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}, true
}

type codexTextAnnotationKey struct {
	OutputIndex  int
	ContentIndex int
}

func codexTextAnnotationKeyForEvent(event codexResponsesEvent) codexTextAnnotationKey {
	key := codexTextAnnotationKey{OutputIndex: codexOutputIndex(event)}
	if event.ContentIndex != nil {
		key.ContentIndex = *event.ContentIndex
	}
	return key
}

func codexCombinedStreamAnnotationsMetadata(values map[codexTextAnnotationKey][]map[string]any) map[string]any {
	annotations := make([]map[string]any, 0)
	for _, key := range sortedCodexAnnotationKeys(values) {
		annotations = append(annotations, cloneMapSlice(values[key])...)
	}
	if len(annotations) == 0 {
		return nil
	}
	return map[string]any{"annotations": annotations}
}

func sortedCodexAnnotationKeys(values map[codexTextAnnotationKey][]map[string]any) []codexTextAnnotationKey {
	keys := make([]codexTextAnnotationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].OutputIndex != keys[j].OutputIndex {
			return keys[i].OutputIndex < keys[j].OutputIndex
		}
		return keys[i].ContentIndex < keys[j].ContentIndex
	})
	return keys
}

func codexResponseWithStreamAnnotations(response codexResponsesResponse, annotations map[codexTextAnnotationKey][]map[string]any) codexResponsesResponse {
	if len(response.Output) == 0 || len(annotations) == 0 {
		return response
	}
	for idx := range response.Output {
		response.Output[idx] = codexOutputItemWithStreamAnnotations(response.Output[idx], idx, annotations)
	}
	return response
}

func codexOutputItemWithStreamAnnotations(item codexResponsesOutputItem, outputIndex int, annotations map[codexTextAnnotationKey][]map[string]any) codexResponsesOutputItem {
	if len(annotations) == 0 {
		return item
	}
	if len(item.Content) == 0 {
		item.Annotations = appendCodexAnnotations(item.Annotations, annotations[codexTextAnnotationKey{OutputIndex: outputIndex}])
		return item
	}
	for contentIndex := range item.Content {
		key := codexTextAnnotationKey{OutputIndex: outputIndex, ContentIndex: contentIndex}
		item.Content[contentIndex].Annotations = appendCodexAnnotations(item.Content[contentIndex].Annotations, annotations[key])
	}
	return item
}

func appendCodexAnnotations(dst []map[string]any, src []map[string]any) []map[string]any {
	if len(src) == 0 {
		return dst
	}
	out := cloneMapSlice(dst)
	for _, annotation := range src {
		if codexAnnotationExists(out, annotation) {
			continue
		}
		out = append(out, cloneMap(annotation))
	}
	return out
}

func codexAnnotationExists(values []map[string]any, candidate map[string]any) bool {
	candidateKey := codexAnnotationDedupeKey(candidate)
	for _, value := range values {
		if codexAnnotationDedupeKey(value) == candidateKey {
			return true
		}
	}
	return false
}

func codexAnnotationDedupeKey(annotation map[string]any) string {
	return strings.Join([]string{
		strings.TrimSpace(mapStringAny(annotation, "type")),
		strings.TrimSpace(mapStringAny(annotation, "url")),
		strings.TrimSpace(fmt.Sprint(annotation["start_index"])),
		strings.TrimSpace(fmt.Sprint(annotation["end_index"])),
		strings.TrimSpace(mapStringAny(annotation, "title")),
	}, "\x00")
}

func codexReasoningStreamEvent(event codexResponsesEvent, eventType string, raw string) contract.ConversationStreamEvent {
	return contract.ConversationStreamEvent{
		Type:         contract.ConversationStreamEventReasoning,
		ContentIndex: codexOutputIndex(event),
		Delta: contract.ContentPart{
			Kind:           contract.ContentPartThinking,
			Text:           event.Delta,
			OriginProtocol: "openai-compatible",
		},
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}
}

func codexStreamUsageEvent(usage openAIUsage, raw string, text string) contract.ConversationStreamEvent {
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventUsage,
		Usage:          usage.ToUsage(text),
		RawEventType:   "usage",
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}
}

func codexOutputIndex(event codexResponsesEvent) int {
	if event.OutputIndex != nil {
		return *event.OutputIndex
	}
	return 0
}

type codexFunctionCallStreamStates struct {
	byOutputIndex map[int]*codexFunctionCallStreamState
	byItemID      map[string]*codexFunctionCallStreamState
}

type codexFunctionCallStreamState struct {
	OutputIndex  int
	ItemID       string
	CallID       string
	Name         string
	ArgumentsLen int
}

func newCodexFunctionCallStreamStates() *codexFunctionCallStreamStates {
	return &codexFunctionCallStreamStates{
		byOutputIndex: map[int]*codexFunctionCallStreamState{},
		byItemID:      map[string]*codexFunctionCallStreamState{},
	}
}

func (s *codexFunctionCallStreamStates) mergeEvent(event codexResponsesEvent) {
	if event.Item == nil || !codexOutputItemIsFunctionCall(*event.Item) {
		return
	}
	state := s.stateFor(event)
	if id := strings.TrimSpace(event.Item.ID); id != "" {
		state.ItemID = id
		s.byItemID[id] = state
	}
	if callID := strings.TrimSpace(event.Item.CallID); callID != "" {
		state.CallID = callID
	}
	if name := strings.TrimSpace(event.Item.Name); name != "" {
		state.Name = name
	}
}

func (s *codexFunctionCallStreamStates) hasArgumentDeltas(event codexResponsesEvent) bool {
	state := s.stateFor(event)
	return state.ArgumentsLen > 0
}

func (s *codexFunctionCallStreamStates) startEvent(event codexResponsesEvent, eventType string, raw string) (contract.ConversationStreamEvent, bool) {
	if event.Item == nil || !codexOutputItemIsFunctionCall(*event.Item) {
		return contract.ConversationStreamEvent{}, false
	}
	state := s.stateFor(event)
	part := contract.ContentPart{
		Kind:           contract.ContentPartToolUse,
		ToolCallID:     firstNonEmpty(state.CallID, state.ItemID),
		ToolName:       state.Name,
		Metadata:       map[string]any{"type": "function_call"},
		OriginProtocol: "openai-compatible",
	}
	if part.ToolCallID == "" && part.ToolName == "" {
		return contract.ConversationStreamEvent{}, false
	}
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventToolCallDelta,
		ContentIndex:   state.OutputIndex,
		Delta:          part,
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}, true
}

func (s *codexFunctionCallStreamStates) deltaEvent(event codexResponsesEvent, eventType string, raw string) contract.ConversationStreamEvent {
	state := s.stateFor(event)
	state.ArgumentsLen += len(event.Delta)
	return contract.ConversationStreamEvent{
		Type:         contract.ConversationStreamEventToolCallDelta,
		ContentIndex: state.OutputIndex,
		Delta: contract.ContentPart{
			Kind:              contract.ContentPartToolUse,
			ToolCallID:        firstNonEmpty(state.CallID, state.ItemID),
			ToolName:          state.Name,
			ToolArgumentsJSON: event.Delta,
			Metadata:          map[string]any{"type": "function_call"},
			OriginProtocol:    "openai-compatible",
		},
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}
}

func (s *codexFunctionCallStreamStates) stateFor(event codexResponsesEvent) *codexFunctionCallStreamState {
	if itemID := strings.TrimSpace(event.ItemID); itemID != "" {
		if state := s.byItemID[itemID]; state != nil {
			return state
		}
	}
	if event.Item != nil {
		if itemID := strings.TrimSpace(event.Item.ID); itemID != "" {
			if state := s.byItemID[itemID]; state != nil {
				return state
			}
		}
	}
	outputIndex := codexOutputIndex(event)
	if state := s.byOutputIndex[outputIndex]; state != nil {
		return state
	}
	state := &codexFunctionCallStreamState{
		OutputIndex: outputIndex,
		ItemID:      firstNonEmpty(strings.TrimSpace(event.ItemID), fmt.Sprintf("fc_%d", outputIndex)),
	}
	s.byOutputIndex[outputIndex] = state
	s.byItemID[state.ItemID] = state
	return state
}

func codexFunctionCallStreamEvent(item codexResponsesOutputItem, contentIndex int, raw string) (contract.ConversationStreamEvent, bool) {
	part, ok := codexFunctionCallPart(item)
	if !ok {
		return contract.ConversationStreamEvent{}, false
	}
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventToolCallDelta,
		ContentIndex:   contentIndex,
		Delta:          part,
		RawEventType:   "response.output_item.done",
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
	}, true
}

func parseCodexResponsesJSON(body []byte, statusCode int) (contract.ConversationResponse, error) {
	var event codexResponsesEvent
	if err := json.Unmarshal(body, &event); err == nil {
		if providerErr, ok := codexEventProviderError(event); ok {
			return contract.ConversationResponse{}, providerErr
		}
		if parts := codexEventParts(event); len(parts) > 0 {
			text := contentPartsText(parts)
			resp := contract.ConversationResponse{
				Parts:      parts,
				StopReason: codexEventStopReason(event),
				StatusCode: statusCode,
				Usage:      codexEventUsage(event, text),
				Raw:        append(json.RawMessage(nil), body...),
			}
			return resp, nil
		}
	}
	var response codexResponsesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	if providerErr, ok := codexResponseProviderError(response); ok {
		return contract.ConversationResponse{}, providerErr
	}
	resp, err := response.ConversationResponse(statusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	resp.Raw = append(json.RawMessage(nil), body...)
	return resp, nil
}

func codexEventText(event codexResponsesEvent) string {
	if strings.TrimSpace(event.Text) != "" {
		return event.Text
	}
	if strings.TrimSpace(event.Refusal) != "" {
		return event.Refusal
	}
	if event.Response != nil {
		return event.Response.Text()
	}
	return ""
}

func codexEventUsage(event codexResponsesEvent, text string) contract.Usage {
	if event.Response != nil && event.Response.Usage.HasTokenUsage() {
		return event.Response.Usage.ToUsage(text)
	}
	if event.Usage != nil {
		return event.Usage.ToUsage(text)
	}
	return estimatedUsage(text)
}

func codexEventParts(event codexResponsesEvent) []contract.ContentPart {
	if event.Response != nil {
		return event.Response.Parts()
	}
	if event.Item != nil {
		return codexResponsesOutputItemParts(*event.Item)
	}
	if refusal := strings.TrimSpace(event.Refusal); refusal != "" {
		return []contract.ContentPart{{Kind: contract.ContentPartRefusal, Text: refusal, OriginProtocol: "openai"}}
	}
	if text := strings.TrimSpace(event.Text); text != "" {
		return []contract.ContentPart{textContentPart(text)}
	}
	return nil
}

func codexEventStopReason(event codexResponsesEvent) contract.StopReason {
	if event.Response != nil {
		return codexStopReason(*event.Response)
	}
	if event.Item != nil && codexOutputItemIsFunctionCall(*event.Item) {
		return contract.StopReasonToolUse
	}
	if event.Item != nil && codexOutputItemIsRefusal(*event.Item) {
		return contract.StopReasonRefusal
	}
	if strings.TrimSpace(event.Refusal) != "" {
		return contract.StopReasonRefusal
	}
	return contract.StopReasonEndTurn
}

func codexStreamStopReason(event codexResponsesEvent, completedRefusal string, streamedRefusal string) contract.StopReason {
	stopReason := codexEventStopReason(event)
	if stopReason == contract.StopReasonEndTurn &&
		(strings.TrimSpace(completedRefusal) != "" || strings.TrimSpace(streamedRefusal) != "") {
		return contract.StopReasonRefusal
	}
	return stopReason
}

func codexCollectedOutputItems(indexed map[int]codexResponsesOutputItem, fallback []codexResponsesOutputItem) []codexResponsesOutputItem {
	if len(indexed) == 0 {
		return append([]codexResponsesOutputItem(nil), fallback...)
	}
	indexes := make([]int, 0, len(indexed))
	for index := range indexed {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]codexResponsesOutputItem, 0, len(indexed)+len(fallback))
	for _, index := range indexes {
		out = append(out, indexed[index])
	}
	out = append(out, fallback...)
	return out
}

func (r codexResponsesResponse) Text() string {
	if strings.TrimSpace(r.OutputText) != "" {
		return r.OutputText
	}
	parts := make([]string, 0, len(r.Output))
	for _, item := range r.Output {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
		if strings.TrimSpace(item.Refusal) != "" {
			parts = append(parts, strings.TrimSpace(item.Refusal))
		}
		for _, content := range item.Content {
			contentType := strings.ToLower(strings.TrimSpace(content.Type))
			if contentType == "refusal" {
				if refusal := strings.TrimSpace(firstNonEmpty(content.Refusal, content.Text)); refusal != "" {
					parts = append(parts, refusal)
				}
				continue
			}
			if text := strings.TrimSpace(content.Text); text != "" && (contentType == "" || strings.Contains(contentType, "text")) {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func (r codexResponsesResponse) ConversationResponse(statusCode int) (contract.ConversationResponse, error) {
	parts := r.Parts()
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:      parts,
		StopReason: codexStopReason(r),
		StatusCode: statusCode,
		Usage:      r.Usage.ToUsage(text),
	}, nil
}

func (r codexResponsesResponse) Parts() []contract.ContentPart {
	parts := codexResponsesOutputItemsParts(r.Output)
	if len(parts) == 0 {
		if text := strings.TrimSpace(r.OutputText); text != "" {
			parts = append(parts, textContentPart(text))
		}
	}
	return parts
}

func codexResponsesOutputItemsParts(items []codexResponsesOutputItem) []contract.ContentPart {
	parts := make([]contract.ContentPart, 0, len(items))
	for _, item := range items {
		parts = append(parts, codexResponsesOutputItemParts(item)...)
	}
	return parts
}

func codexResponsesOutputItemParts(item codexResponsesOutputItem) []contract.ContentPart {
	parts := []contract.ContentPart(nil)
	itemType := strings.ToLower(strings.TrimSpace(item.Type))
	if itemType == "function_call" {
		if part, ok := codexFunctionCallPart(item); ok {
			parts = append(parts, part)
		}
		return parts
	}
	if itemType == "image_generation_call" {
		if part, ok := codexImageGenerationPart(item); ok {
			parts = append(parts, part)
		}
		return parts
	}
	if itemType == "refusal" {
		if text := strings.TrimSpace(firstNonEmpty(item.Refusal, item.Text)); text != "" {
			parts = append(parts, contract.ContentPart{Kind: contract.ContentPartRefusal, Text: text, OriginProtocol: "openai"})
		}
		return parts
	}
	if text := strings.TrimSpace(item.Text); text != "" {
		kind := contract.ContentPartText
		if itemType == "reasoning" {
			kind = contract.ContentPartThinking
		}
		part := contract.ContentPart{Kind: kind, Text: text, OriginProtocol: "openai"}
		if kind == contract.ContentPartText {
			part.Metadata = codexOutputItemTextMetadata(item)
		}
		parts = append(parts, part)
	}
	for _, content := range item.Content {
		contentType := strings.ToLower(strings.TrimSpace(content.Type))
		if contentType == "refusal" {
			if text := strings.TrimSpace(firstNonEmpty(content.Refusal, content.Text)); text != "" {
				parts = append(parts, contract.ContentPart{Kind: contract.ContentPartRefusal, Text: text, OriginProtocol: "openai"})
			}
			continue
		}
		text := strings.TrimSpace(content.Text)
		if text != "" && (contentType == "" || strings.Contains(contentType, "text")) {
			part := textContentPart(text)
			part.Metadata = codexResponsesOutputContentMetadata(content)
			part.OriginProtocol = "openai"
			parts = append(parts, part)
		}
	}
	return parts
}

func codexResponsesOutputContentMetadata(content codexResponsesOutputContent) map[string]any {
	metadata := map[string]any{}
	if len(content.Annotations) > 0 {
		values := make([]map[string]any, len(content.Annotations))
		for idx, annotation := range content.Annotations {
			values[idx] = cloneMap(annotation)
		}
		metadata["annotations"] = values
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func codexOutputItemTextMetadata(item codexResponsesOutputItem) map[string]any {
	metadata := map[string]any{}
	if len(item.Annotations) > 0 {
		metadata["annotations"] = cloneMapSlice(item.Annotations)
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func codexImageGenerationPart(item codexResponsesOutputItem) (contract.ContentPart, bool) {
	result := strings.TrimSpace(item.Result)
	if result == "" {
		return contract.ContentPart{}, false
	}
	metadata := map[string]any{"type": strings.TrimSpace(item.Type)}
	if id := strings.TrimSpace(item.ID); id != "" {
		metadata["id"] = id
	}
	if status := strings.TrimSpace(item.Status); status != "" {
		metadata["status"] = status
	}
	if format := strings.TrimSpace(item.OutputFormat); format != "" {
		metadata["output_format"] = format
	}
	return contract.ContentPart{
		Kind:           contract.ContentPartImage,
		MediaBase64:    result,
		Metadata:       metadata,
		OriginProtocol: "openai",
	}, true
}

func codexFunctionCallPart(item codexResponsesOutputItem) (contract.ContentPart, bool) {
	id := strings.TrimSpace(item.CallID)
	if id == "" {
		id = strings.TrimSpace(item.ID)
	}
	name := strings.TrimSpace(item.Name)
	arguments := strings.TrimSpace(item.Arguments)
	if id == "" && name == "" && arguments == "" {
		return contract.ContentPart{}, false
	}
	metadata := map[string]any{"type": strings.TrimSpace(item.Type)}
	if status := strings.TrimSpace(item.Status); status != "" {
		metadata["status"] = status
	}
	return contract.ContentPart{
		Kind:              contract.ContentPartToolUse,
		ToolCallID:        id,
		ToolName:          name,
		ToolArgumentsJSON: arguments,
		Metadata:          metadata,
		OriginProtocol:    "openai",
	}, true
}

func codexStopReason(response codexResponsesResponse) contract.StopReason {
	if response.IncompleteDetails != nil {
		reason := strings.ToLower(strings.TrimSpace(response.IncompleteDetails.Reason))
		if strings.Contains(reason, "filter") || strings.Contains(reason, "safety") {
			return contract.StopReasonContentFilter
		}
		if reason != "" {
			return contract.StopReasonMaxTokens
		}
	}
	if codexOutputItemsIncludeFunctionCall(response.Output) {
		return contract.StopReasonToolUse
	}
	if codexOutputItemsIncludeRefusal(response.Output) {
		return contract.StopReasonRefusal
	}
	if strings.EqualFold(strings.TrimSpace(response.Status), "incomplete") {
		return contract.StopReasonMaxTokens
	}
	return contract.StopReasonEndTurn
}

func codexOutputItemsIncludeFunctionCall(items []codexResponsesOutputItem) bool {
	for _, item := range items {
		if codexOutputItemIsFunctionCall(item) {
			return true
		}
	}
	return false
}

func codexOutputItemIsFunctionCall(item codexResponsesOutputItem) bool {
	return strings.EqualFold(strings.TrimSpace(item.Type), "function_call")
}

func codexOutputItemsIncludeRefusal(items []codexResponsesOutputItem) bool {
	for _, item := range items {
		if codexOutputItemIsRefusal(item) {
			return true
		}
	}
	return false
}

func codexOutputItemIsRefusal(item codexResponsesOutputItem) bool {
	if strings.EqualFold(strings.TrimSpace(item.Type), "refusal") || strings.TrimSpace(item.Refusal) != "" {
		return true
	}
	for _, content := range item.Content {
		if strings.EqualFold(strings.TrimSpace(content.Type), "refusal") {
			return true
		}
	}
	return false
}

func codexEventProviderError(event codexResponsesEvent) (contract.ProviderError, bool) {
	if event.Response != nil {
		if providerErr, ok := codexResponseProviderError(*event.Response); ok {
			return providerErr, true
		}
	}
	if event.Error != nil {
		return codexProviderError(*event.Error), true
	}
	if event.Type != "error" && event.Type != "response.failed" {
		return contract.ProviderError{}, false
	}
	err := codexResponsesError{Message: event.Message, Code: event.Code}
	if err.Message == "" {
		err.Message = "codex upstream returned terminal error event"
	}
	return codexProviderError(err), true
}

func codexResponseProviderError(response codexResponsesResponse) (contract.ProviderError, bool) {
	if response.Error != nil {
		return codexProviderError(*response.Error), true
	}
	if strings.EqualFold(strings.TrimSpace(response.Status), "failed") {
		return codexProviderError(codexResponsesError{Message: "codex upstream returned failed response"}), true
	}
	return contract.ProviderError{}, false
}

func codexProviderError(err codexResponsesError) contract.ProviderError {
	message := strings.TrimSpace(err.Message)
	if message == "" {
		message = strings.TrimSpace(err.Code)
	}
	if message == "" {
		message = strings.TrimSpace(err.Type)
	}
	if message == "" {
		message = "codex upstream returned an error"
	}
	class := "provider_5xx"
	lowerCode := strings.ToLower(strings.TrimSpace(err.Code))
	lowerMessage := strings.ToLower(message)
	if strings.Contains(lowerCode, "context") ||
		strings.Contains(lowerMessage, "context length") ||
		strings.Contains(lowerMessage, "context window") ||
		strings.Contains(lowerMessage, "too many tokens") {
		class = "invalid_request"
	}
	return contract.ProviderError{Class: class, StatusCode: http.StatusBadGateway, Message: message}
}
