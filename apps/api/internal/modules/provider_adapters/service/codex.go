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
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const (
	codexOriginator                         = "codex_cli_rs"
	codexDefaultVersion                     = "0.125.0"
	codexDefaultUserAgent                   = codexOriginator + "/" + codexDefaultVersion
	codexDefaultInstructions                = "You are a concise assistant."
	codexImageGenerationBridgeMarker        = "<srapi-codex-image-generation>"
	codexImageGenerationBridgeText          = codexImageGenerationBridgeMarker + "\nWhen the user asks for raster image generation or editing, use the OpenAI Responses native `image_generation` tool attached to this request. The local Codex client may not expose an `image_gen` namespace, but image generation is still available through this tool.\n</srapi-codex-image-generation>"
	codexSparkImageUnsupportedMarker        = "<srapi-codex-spark-image-unsupported>"
	codexSparkImageUnsupportedText          = codexSparkImageUnsupportedMarker + "\nThe current model is gpt-5.3-codex-spark, which does not support image generation, image editing, image input, the `image_generation` tool, or Codex `image_gen` workflows. If the user asks for image generation or image editing, explain this model limitation and ask them to switch to a non-Spark Codex model such as gpt-5.3-codex or gpt-5.4.\n</srapi-codex-spark-image-unsupported>"
	codexResponsesBetaHeaderValue           = "responses=experimental"
	codexResponsesWebsocketBetaHeaderValue  = "responses_websockets=2026-02-06"
	codexDefaultAccountSessionIDPrefix      = "srapi-codex-account-"
	codexResponsesDefaultInternalStoreValue = false
	codexResponsesEncryptedReasoningInclude = "reasoning.encrypted_content"
)

type codexResponsesInputItem struct {
	Type    string                       `json:"type"`
	Role    string                       `json:"role,omitempty"`
	Content []codexResponsesInputContent `json:"content,omitempty"`
	CallID  string                       `json:"call_id,omitempty"`
	Name    string                       `json:"name,omitempty"`
	Args    string                       `json:"arguments,omitempty"`
	Input   string                       `json:"input,omitempty"`
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
	ID                string                     `json:"id"`
	Object            string                     `json:"object"`
	Model             string                     `json:"model"`
	Output            []codexResponsesOutputItem `json:"output"`
	OutputText        string                     `json:"output_text"`
	InputTokens       *int                       `json:"input_tokens"`
	OutputTokens      *int                       `json:"output_tokens"`
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
	Input        string                        `json:"input"`
	Output       *string                       `json:"output"`
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
	Message         string `json:"message"`
	Code            string `json:"code"`
	Type            string `json:"type"`
	PlanType        string `json:"plan_type"`
	ResetsAt        any    `json:"resets_at"`
	ResetsInSeconds any    `json:"resets_in_seconds"`
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
		URL:          codexResponsesEndpoint(baseURL, req),
		Headers:      codexResponsesHeaders(req, stream, payload),
		Body:         raw,
		ExpectStream: stream,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		if retryPayload, ok := codexResponsesPreviousResponseRecoveryPayload(req, payload, runtimeResp.Body); ok {
			retryRaw, marshalErr := json.Marshal(retryPayload)
			if marshalErr != nil {
				return contract.ConversationResponse{}, marshalErr
			}
			retryResp, retryErr := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
				Account:      codexReverseProxyAccount(req),
				Method:       http.MethodPost,
				URL:          codexResponsesEndpoint(baseURL, req),
				Headers:      codexResponsesHeaders(req, stream, retryPayload),
				Body:         retryRaw,
				ExpectStream: stream,
			})
			if retryErr != nil {
				return contract.ConversationResponse{}, providerErrorFromReverseProxy(retryErr)
			}
			if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
				parsed, parseErr := parseCodexResponsesBody(retryResp.Body, retryResp.StatusCode)
				if parseErr != nil {
					return contract.ConversationResponse{}, parseErr
				}
				return withCodexQuotaSignals(parsed, retryResp.Headers), nil
			}
			return contract.ConversationResponse{}, classifyCodexProviderHTTPErrorWithHeaders(retryResp.StatusCode, retryResp.Headers, retryResp.Body)
		}
		return contract.ConversationResponse{}, classifyCodexProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseCodexResponsesBody(runtimeResp.Body, runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withCodexQuotaSignals(parsed, runtimeResp.Headers), nil
}

func (s *Service) invokeReverseProxyCodexResponseInputItems(ctx context.Context, req contract.ResponseInputItemsRequest, baseURL string) (contract.ResponseInputItemsResponse, error) {
	if s.reverseProxy == nil {
		return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if codexResponseInputItemsRuntimeIsAPIKey(req) {
		return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: responseInputItemsReverseProxyAccount(req),
		Method:  http.MethodGet,
		URL:     responseInputItemsEndpoint(baseURL, req.ResponseID, req.Query),
		Headers: codexResponseInputItemsHeaders(req),
	})
	if err != nil {
		return contract.ResponseInputItemsResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ResponseInputItemsResponse{}, classifyCodexProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	return withCodexInputItemsQuotaSignals(contract.ResponseInputItemsResponse{Raw: append([]byte(nil), bytes.TrimSpace(runtimeResp.Body)...), StatusCode: runtimeResp.StatusCode}, runtimeResp.Headers), nil
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
	initialFrame := codexRealtimeInitialFrame(req)
	headers := codexRealtimeHeaders(req, initialFrame)
	return contract.RealtimeSession{
		URL:          wsURL,
		Headers:      headers,
		InitialFrame: initialFrame,
	}, nil
}

func codexResponsesHeaders(req contract.ConversationRequest, stream bool, payload map[string]any) http.Header {
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	headers := http.Header{
		"Accept":       {accept},
		"Content-Type": {"application/json"},
	}
	headers.Set("OpenAI-Beta", codexResponsesBetaHeaderValue)
	headers.Set("Originator", codexResponsesOriginator(req))
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
	promptCacheKey := codexPayloadPromptCacheKey(payload)
	if sessionID := requestSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("Session_id", sessionID)
	} else if promptCacheKey != "" {
		headers.Set("Session_id", promptCacheKey)
	} else if req.Account.ID > 0 {
		headers.Set("Session_id", codexDefaultAccountSessionID(req.Account.ID))
	}
	codexApplySessionIdentityHeaders(headers, promptCacheKey)
	return headers
}

func codexResponseInputItemsHeaders(req contract.ResponseInputItemsRequest) http.Header {
	headers := http.Header{
		"Accept": {"application/json"},
	}
	headers.Set("OpenAI-Beta", codexResponsesBetaHeaderValue)
	headers.Set("Originator", codexResponseInputItemsOriginator(req))
	headers.Set("User-Agent", codexResponseInputItemsUserAgent(req))
	if accountID := responseInputItemsSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	if betaFeatures := responseInputItemsSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		headers.Set("X-Codex-Beta-Features", betaFeatures)
	}
	if version := responseInputItemsSetting(req, "codex_version", "version", "Version"); version != "" {
		headers.Set("Version", version)
	} else {
		headers.Set("Version", codexDefaultVersion)
	}
	if requestID := responseInputItemsSetting(req, "codex_client_request_id", "x_client_request_id", "X-Client-Request-Id"); requestID != "" {
		headers.Set("X-Client-Request-Id", requestID)
	} else if strings.TrimSpace(req.RequestID) != "" {
		headers.Set("X-Client-Request-Id", strings.TrimSpace(req.RequestID))
	}
	if sessionID := responseInputItemsSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
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

func codexResponseInputItemsUserAgent(req contract.ResponseInputItemsRequest) string {
	if userAgent := responseInputItemsSetting(req, "user_agent"); userAgent != "" {
		return userAgent
	}
	return codexDefaultUserAgent
}

func codexResponsesOriginator(req contract.ConversationRequest) string {
	if originator := requestSetting(req, "codex_originator", "originator"); originator != "" {
		return originator
	}
	return codexOriginator
}

func codexResponseInputItemsOriginator(req contract.ResponseInputItemsRequest) string {
	if originator := responseInputItemsSetting(req, "codex_originator", "originator"); originator != "" {
		return originator
	}
	return codexOriginator
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

func codexResponseInputItemsRuntimeIsAPIKey(req contract.ResponseInputItemsRequest) bool {
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

func responseInputItemsSetting(req contract.ResponseInputItemsRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func codexRealtimeHeaders(req contract.RealtimeRequest, initialFrame []byte) http.Header {
	headers := http.Header{
		"OpenAI-Beta": {codexResponsesWebsocketBetaHeaderValue},
	}
	headers.Set("Originator", codexRealtimeOriginator(req))
	headers.Set("User-Agent", codexRealtimeUserAgent(req))
	if accountID := realtimeSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	if betaFeatures := realtimeSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		headers.Set("X-Codex-Beta-Features", betaFeatures)
	}
	if version := realtimeSetting(req, "codex_version", "version", "Version"); version != "" {
		headers.Set("Version", version)
	} else {
		headers.Set("Version", codexDefaultVersion)
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
	promptCacheKey := codexInitialFramePromptCacheKey(initialFrame)
	if sessionID := realtimeSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("session_id", sessionID)
	} else if strings.Contains(realtimeSetting(req, "user_agent"), "Mac OS") && strings.TrimSpace(req.RequestID) != "" {
		headers.Set("session_id", strings.TrimSpace(req.RequestID))
	} else if promptCacheKey != "" {
		headers.Set("session_id", promptCacheKey)
	} else if req.Account.ID > 0 {
		headers.Set("session_id", codexDefaultAccountSessionID(req.Account.ID))
	}
	codexApplySessionIdentityHeaders(headers, promptCacheKey)
	return headers
}

func codexRealtimeUserAgent(req contract.RealtimeRequest) string {
	if userAgent := realtimeSetting(req, "user_agent"); userAgent != "" {
		return userAgent
	}
	return codexDefaultUserAgent
}

func codexRealtimeOriginator(req contract.RealtimeRequest) string {
	if originator := realtimeSetting(req, "codex_originator", "originator"); originator != "" {
		return originator
	}
	return codexOriginator
}

func codexRealtimeInitialFrame(req contract.RealtimeRequest) []byte {
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(req.RequestPayload), &payload); err != nil {
		return append([]byte(nil), req.RequestPayload...)
	}
	codexApplyResponsesPayloadDefaults(codexRealtimeConversationRequest(req), payload)
	delete(payload, "background")
	payload["type"] = "response.create"
	encoded, err := json.Marshal(payload)
	if err != nil {
		return append([]byte(nil), req.RequestPayload...)
	}
	return encoded
}

func codexPayloadPromptCacheKey(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	return codexStringValue(payload["prompt_cache_key"])
}

func codexInitialFramePromptCacheKey(frame []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(frame), &payload); err != nil {
		return ""
	}
	return codexPayloadPromptCacheKey(payload)
}

func codexApplySessionIdentityHeaders(headers http.Header, promptCacheKey string) {
	promptCacheKey = strings.TrimSpace(promptCacheKey)
	if promptCacheKey == "" {
		return
	}
	headers.Set("Conversation_id", promptCacheKey)
	headers.Set("Thread-Id", promptCacheKey)
	headers.Set("X-Codex-Window-Id", promptCacheKey+":0")
}

func codexRealtimeConversationRequest(req contract.RealtimeRequest) contract.ConversationRequest {
	return contract.ConversationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: req.SourceProtocol,
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.Model,
		RawBody:        append([]byte(nil), req.RequestPayload...),
		Provider:       req.Provider,
		Account:        req.Account,
		Mapping:        req.Mapping,
		Credential:     req.Credential,
	}
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
	return parseCodexResponsesBodyWithOptions(body, statusCode, codexResponsesParseOptions{})
}

type codexResponsesParseOptions struct {
	RequireTerminalEvent bool
}

func parseCodexResponsesBodyWithOptions(body []byte, statusCode int, options codexResponsesParseOptions) (contract.ConversationResponse, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:")) {
		return parseCodexResponsesStream(body, statusCode, options)
	}
	return parseCodexResponsesJSON(trimmed, statusCode)
}

func parseCodexResponsesStream(body []byte, statusCode int, options codexResponsesParseOptions) (contract.ConversationResponse, error) {
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
	seenTerminalEvent := false
	seenRenderableEvent := false
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
		if providerErr, ok := codexEventProviderError(event); ok && eventType != "response.failed" {
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
		case "response.created", "response.in_progress", "response.queued":
			if !seenRenderableEvent {
				appendStreamEvent(codexMetadataStreamEvent(event, eventType, data))
			}
		case "response.output_item.added":
			seenRenderableEvent = true
			if streamEvent, ok := functionStates.startEvent(event, eventType, data); ok {
				appendStreamEvent(streamEvent)
			}
		case "response.output_item.done":
			seenRenderableEvent = true
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
			seenRenderableEvent = true
			if streamEvent, ok := codexImageGenerationPartialStreamEvent(event, eventType, data); ok {
				appendStreamEvent(streamEvent)
			}
		case "response.output_text.delta":
			seenRenderableEvent = true
			deltaBuilder.WriteString(event.Delta)
			if event.Delta != "" {
				appendStreamEvent(codexContentStreamEvent(event, eventType, data, textContentDelta(event.Delta)))
			}
		case "response.output_text.annotation.added":
			seenRenderableEvent = true
			if len(event.Annotation) > 0 {
				key := codexTextAnnotationKeyForEvent(event)
				annotation := cloneMap(event.Annotation)
				textAnnotationsByIndex[key] = append(textAnnotationsByIndex[key], annotation)
				appendStreamEvent(codexContentStreamEvent(event, eventType, data, codexAnnotationContentDelta(annotation)))
			}
		case "response.refusal.delta":
			seenRenderableEvent = true
			refusalBuilder.WriteString(event.Delta)
			if event.Delta != "" {
				appendStreamEvent(codexContentStreamEvent(event, eventType, data, contract.ContentPart{
					Kind:           contract.ContentPartRefusal,
					Text:           event.Delta,
					OriginProtocol: "openai-compatible",
				}))
			}
		case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
			seenRenderableEvent = true
			reasoningBuilder.WriteString(event.Delta)
			if event.Delta != "" {
				appendStreamEvent(codexReasoningStreamEvent(event, eventType, data))
			}
		case "response.function_call_arguments.delta":
			seenRenderableEvent = true
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
		case "response.reasoning_text.done", "response.reasoning_summary_text.done":
			if strings.TrimSpace(event.Text) != "" {
				completedReasoning = event.Text
			}
		case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled", "response.failed":
			seenTerminalEvent = true
			if eventType != "response.failed" {
				if providerErr, ok := codexEventProviderError(event); ok {
					return contract.ConversationResponse{}, providerErr
				}
			}
			if text := codexEventText(event); strings.TrimSpace(text) != "" {
				completedText = text
			}
			appendStreamEvent(codexTerminalStreamEvent(event, eventType, data, completedRefusal, refusalBuilder.String()))
		default:
			if text := codexEventText(event); strings.TrimSpace(text) != "" && strings.TrimSpace(completedText) == "" {
				completedText = text
			}
		}
	}
	if !seenEvent {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	if options.RequireTerminalEvent && !seenTerminalEvent {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before terminal event"}
	}
	parts, stopReason, err := codexResponsesStreamPartsAndStopReason(
		finalResponse,
		indexedItems,
		fallbackItems,
		completedText,
		deltaBuilder.String(),
		completedRefusal,
		refusalBuilder.String(),
		completedReasoning,
		reasoningBuilder.String(),
		textAnnotationsByIndex,
		streamEvents,
	)
	if err != nil {
		return contract.ConversationResponse{}, err
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

func codexResponsesStreamPartsAndStopReason(
	finalResponse *codexResponsesResponse,
	indexedItems map[int]codexResponsesOutputItem,
	fallbackItems []codexResponsesOutputItem,
	completedText string,
	streamedText string,
	completedRefusal string,
	streamedRefusal string,
	completedReasoning string,
	streamedReasoning string,
	textAnnotationsByIndex map[codexTextAnnotationKey][]map[string]any,
	streamEvents []contract.ConversationStreamEvent,
) ([]contract.ContentPart, contract.StopReason, error) {
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
		text := strings.TrimSpace(firstNonEmpty(completedText, streamedText))
		if text != "" {
			part := textContentPart(text)
			part.Metadata = codexCombinedStreamAnnotationsMetadata(textAnnotationsByIndex)
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		refusalText := strings.TrimSpace(firstNonEmpty(completedRefusal, streamedRefusal))
		if refusalText != "" {
			parts = append(parts, contract.ContentPart{Kind: contract.ContentPartRefusal, Text: refusalText, OriginProtocol: "openai"})
			stopReason = contract.StopReasonRefusal
		}
	}
	parts = prependCodexReasoningPart(parts, completedReasoning, streamedReasoning)
	if len(parts) == 0 && codexStreamEventsEndWithFailed(streamEvents) {
		return []contract.ContentPart{{Kind: contract.ContentPartMetadata, Metadata: map[string]any{"type": "response.failed"}, OriginProtocol: "openai-compatible"}}, contract.StopReasonContentFilter, nil
	}
	if len(parts) == 0 {
		return nil, "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	return parts, stopReason, nil
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

func codexMetadataStreamEvent(event codexResponsesEvent, eventType string, raw string) contract.ConversationStreamEvent {
	metadata := map[string]any{"type": eventType}
	if event.Response != nil {
		if id := strings.TrimSpace(event.Response.ID); id != "" {
			metadata["response_id"] = id
		}
		if model := strings.TrimSpace(event.Response.Model); model != "" {
			metadata["model"] = model
		}
		if status := strings.TrimSpace(event.Response.Status); status != "" {
			metadata["status"] = status
		}
	}
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventMetadata,
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
		Metadata:       metadata,
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
	metadata := map[string]any(nil)
	if strings.HasPrefix(eventType, "response.reasoning_summary_text.") {
		metadata = map[string]any{"reasoning_event_type": "summary_text"}
	}
	return contract.ConversationStreamEvent{
		Type:         contract.ConversationStreamEventReasoning,
		ContentIndex: codexOutputIndex(event),
		Delta: contract.ContentPart{
			Kind:           contract.ContentPartThinking,
			Text:           event.Delta,
			Metadata:       metadata,
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
	if codexResponseIsCompaction(response) {
		return contract.ConversationResponse{
			StopReason: contract.StopReasonEndTurn,
			StatusCode: statusCode,
			Usage:      codexCompactionUsage(response),
			Raw:        append(json.RawMessage(nil), body...),
		}, nil
	}
	resp, err := response.ConversationResponse(statusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	resp.Raw = append(json.RawMessage(nil), body...)
	return resp, nil
}

func codexResponseIsCompaction(response codexResponsesResponse) bool {
	return strings.EqualFold(strings.TrimSpace(response.Object), "response.compaction")
}

func codexCompactionUsage(response codexResponsesResponse) contract.Usage {
	usage := response.Usage
	if usage.InputTokens == nil && response.InputTokens != nil {
		usage.InputTokens = response.InputTokens
	}
	if usage.OutputTokens == nil && response.OutputTokens != nil {
		usage.OutputTokens = response.OutputTokens
	}
	if usage.HasTokenUsage() {
		return usage.ToUsage("")
	}
	return contract.Usage{}
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

func codexTerminalStreamEvent(event codexResponsesEvent, eventType string, raw string, completedRefusal string, streamedRefusal string) contract.ConversationStreamEvent {
	stopReason := codexStreamStopReason(event, completedRefusal, streamedRefusal)
	metadata := map[string]any(nil)
	if eventType == "response.failed" {
		stopReason = contract.StopReasonContentFilter
		metadata = codexFailedStreamEventMetadata(event)
	}
	return contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventStop,
		StopReason:     stopReason,
		RawEventType:   eventType,
		Raw:            append(json.RawMessage(nil), raw...),
		OriginProtocol: "openai-compatible",
		Metadata:       metadata,
	}
}

func codexFailedStreamEventMetadata(event codexResponsesEvent) map[string]any {
	metadata := map[string]any{"type": "response.failed"}
	mergeError := func(err *codexResponsesError) {
		if err == nil {
			return
		}
		if value := strings.TrimSpace(err.Message); value != "" {
			if _, ok := metadata["error_message"]; !ok {
				metadata["error_message"] = value
			}
		}
		if value := strings.TrimSpace(err.Code); value != "" {
			if _, ok := metadata["error_code"]; !ok {
				metadata["error_code"] = value
			}
		}
		if value := strings.TrimSpace(err.Type); value != "" {
			if _, ok := metadata["error_type"]; !ok {
				metadata["error_type"] = value
			}
		}
	}
	mergeError(event.Error)
	if value := strings.TrimSpace(event.Message); value != "" {
		metadata["message"] = value
	}
	if value := strings.TrimSpace(event.Code); value != "" {
		metadata["code"] = value
	}
	if event.Response != nil {
		if status := strings.TrimSpace(event.Response.Status); status != "" {
			metadata["status"] = status
		}
		mergeError(event.Response.Error)
	}
	return metadata
}

func codexStreamEventsEndWithFailed(events []contract.ConversationStreamEvent) bool {
	if len(events) == 0 {
		return false
	}
	last := events[len(events)-1]
	return last.Type == contract.ConversationStreamEventStop &&
		strings.TrimSpace(last.RawEventType) == "response.failed"
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
	if codexResponsesToolResultTypeIsSupported(itemType) {
		if part, ok := codexFunctionCallOutputPart(item); ok {
			parts = append(parts, part)
		}
		return parts
	}
	if codexResponsesToolCallTypeIsSupported(itemType) {
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

func codexResponsesToolResultTypeIsSupported(itemType string) bool {
	switch itemType {
	case "function_call_output", "custom_tool_call_output", "mcp_tool_call_output", "tool_search_output":
		return true
	default:
		return false
	}
}

func codexFunctionCallOutputPart(item codexResponsesOutputItem) (contract.ContentPart, bool) {
	callID := strings.TrimSpace(firstNonEmpty(item.CallID, item.ID))
	output := item.Text
	if item.Output != nil {
		output = *item.Output
	}
	if callID == "" && strings.TrimSpace(output) == "" {
		return contract.ContentPart{}, false
	}
	metadata := map[string]any{"type": strings.TrimSpace(item.Type)}
	if status := strings.TrimSpace(item.Status); status != "" {
		metadata["status"] = status
	}
	return contract.ContentPart{
		Kind:            contract.ContentPartToolResult,
		ToolResultForID: callID,
		Text:            output,
		Metadata:        metadata,
		OriginProtocol:  "openai",
	}, true
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
	arguments := item.Arguments
	if arguments == "" {
		arguments = item.Input
	}
	if id == "" && name == "" && strings.TrimSpace(arguments) == "" {
		return contract.ContentPart{}, false
	}
	metadata := map[string]any{"type": strings.TrimSpace(item.Type)}
	if status := strings.TrimSpace(item.Status); status != "" {
		metadata["status"] = status
	}
	if item.Input != "" && item.Arguments == "" {
		metadata["arguments_field"] = "input"
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
	return codexResponsesToolCallTypeIsSupported(strings.TrimSpace(item.Type))
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
	class, statusCode := codexProviderErrorClassAndStatus(err, message)
	metadata := map[string]any(nil)
	if planType := strings.TrimSpace(err.PlanType); planType != "" {
		metadata = map[string]any{"plan_type": planType}
	}
	return contract.ProviderError{
		Class:      class,
		StatusCode: statusCode,
		Message:    message,
		RetryAfter: codexRetryAfterFromError(err, time.Now()),
		Metadata:   metadata,
	}
}

func classifyCodexProviderHTTPErrorWithHeaders(statusCode int, headers http.Header, body []byte) contract.ProviderError {
	err := codexErrorFromHTTPBody(body)
	message := codexHTTPErrorMessage(body, statusCode, err)
	class, effectiveStatus := codexHTTPErrorClassAndStatus(statusCode, body, err, message)
	metadata := map[string]any(nil)
	if err != nil && strings.TrimSpace(err.PlanType) != "" {
		metadata = map[string]any{"plan_type": strings.TrimSpace(err.PlanType)}
	}
	return contract.ProviderError{
		Class:      class,
		StatusCode: effectiveStatus,
		Message:    message,
		RetryAfter: providerRetryAfter(headers, body, time.Now()),
		Metadata:   metadata,
	}
}

func codexErrorFromHTTPBody(body []byte) *codexResponsesError {
	var decoded struct {
		Error codexResponsesError `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(body), &decoded); err != nil {
		return nil
	}
	if strings.TrimSpace(decoded.Error.Message) == "" &&
		strings.TrimSpace(decoded.Error.Code) == "" &&
		strings.TrimSpace(decoded.Error.Type) == "" {
		return nil
	}
	return &decoded.Error
}

func codexHTTPErrorMessage(body []byte, statusCode int, err *codexResponsesError) string {
	if err != nil {
		for _, value := range []string{err.Message, err.Code, err.Type} {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	var decoded struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Type    string `json:"type"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &decoded) == nil {
		for _, value := range []string{decoded.Message, decoded.Code, decoded.Type} {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	if message := strings.TrimSpace(string(body)); message != "" {
		return message
	}
	return http.StatusText(statusCode)
}

func codexHTTPErrorClassAndStatus(statusCode int, body []byte, err *codexResponsesError, message string) (string, int) {
	if err != nil {
		class, effectiveStatus := codexProviderErrorClassAndStatus(*err, message)
		if class != "provider_5xx" {
			return class, effectiveStatus
		}
	}
	if codexHTTPBodyIsCapacityError(body) {
		return "rate_limit", http.StatusTooManyRequests
	}
	return providerClassForHTTPStatus(statusCode), statusCode
}

func codexProviderErrorClassAndStatus(err codexResponsesError, message string) (string, int) {
	lowerCode := strings.ToLower(strings.TrimSpace(err.Code))
	lowerType := strings.ToLower(strings.TrimSpace(err.Type))
	lowerMessage := strings.ToLower(strings.TrimSpace(message))
	lowerCombined := strings.Join([]string{lowerCode, lowerType, lowerMessage}, " ")
	switch {
	case strings.Contains(lowerCombined, "usage_limit_reached") ||
		strings.Contains(lowerCombined, "rate_limit") ||
		strings.Contains(lowerCombined, "too many requests") ||
		strings.Contains(lowerCombined, "selected model is at capacity") ||
		strings.Contains(lowerCombined, "model is at capacity"):
		return "rate_limit", http.StatusTooManyRequests
	case strings.Contains(lowerCombined, "context") ||
		strings.Contains(lowerCombined, "too many tokens") ||
		strings.Contains(lowerCombined, "previous_response_not_found") ||
		strings.Contains(lowerCombined, "previous_response_id") && strings.Contains(lowerCombined, "not found") ||
		strings.Contains(lowerCombined, "invalid signature in thinking block") ||
		strings.Contains(lowerCombined, "invalid_encrypted_content"):
		return "invalid_request", http.StatusBadRequest
	case strings.Contains(lowerCombined, "authentication") ||
		strings.Contains(lowerCombined, "unauthorized") ||
		strings.Contains(lowerCombined, "invalid_api_key") ||
		strings.Contains(lowerCombined, "invalid or expired token") ||
		strings.Contains(lowerCombined, "refresh_token_reused"):
		return "auth_failed", http.StatusUnauthorized
	default:
		return "provider_5xx", http.StatusBadGateway
	}
}

func codexHTTPBodyIsCapacityError(body []byte) bool {
	lower := strings.ToLower(strings.TrimSpace(string(body)))
	return strings.Contains(lower, "selected model is at capacity") ||
		strings.Contains(lower, "model is at capacity. please try a different model")
}

func codexRetryAfterFromError(err codexResponsesError, now time.Time) *time.Time {
	if resetAt := retryAfterTimestampValue(err.ResetsAt, now); resetAt != nil {
		return resetAt
	}
	if seconds, ok := positiveInt64(err.ResetsInSeconds); ok {
		value := now.UTC().Add(time.Duration(seconds) * time.Second)
		return &value
	}
	return nil
}
