package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const (
	antigravityGeneratePath = "/v1internal:generateContent"
	antigravityStreamPath   = "/v1internal:streamGenerateContent"
)

type antigravityRequest struct {
	Project            string                         `json:"project"`
	RequestID          string                         `json:"requestId"`
	UserAgent          string                         `json:"userAgent"`
	RequestType        string                         `json:"requestType"`
	Model              string                         `json:"model"`
	EnabledCreditTypes []string                       `json:"enabledCreditTypes,omitempty"`
	Request            antigravityGenerateTextRequest `json:"request"`
}

type antigravityGenerateTextRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiContent           `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
	Tools             []map[string]any         `json:"tools,omitempty"`
	ToolConfig        *antigravityToolConfig   `json:"toolConfig,omitempty"`
	SafetySettings    []antigravitySafetyEntry `json:"safetySettings,omitempty"`
	SessionID         string                   `json:"sessionId,omitempty"`
}

type antigravityToolConfig struct {
	FunctionCallingConfig map[string]string `json:"functionCallingConfig,omitempty"`
}

type antigravitySafetyEntry struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type antigravityResponseEnvelope struct {
	Response geminiGenerateContentResponse `json:"response"`
	TraceID  string                        `json:"traceId"`
}

func (s *Service) invokeReverseProxyAntigravity(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if antigravityReverseProxyRuntimeIsAPIKey(req) {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "antigravity reverse proxy requires OAuth/session/desktop/IDE/client-token runtime credentials"}
	}
	payload, err := antigravityPayload(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account:      antigravityReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          antigravityEndpoint(baseURL, req.Stream),
		Headers:      antigravityHeaders(req),
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyGeminiProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	if req.Stream {
		parsed, err := parseAntigravityStream(runtimeResp.Body, runtimeResp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
	}
	unwrapped, err := parseAntigravityResponse(runtimeResp.Body)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	parsed, err := unwrapped.ConversationResponse(runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	parsed.Raw = append([]byte(nil), runtimeResp.Body...)
	return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
}

func (s *Service) invokeReverseProxyAntigravityImageGeneration(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if antigravityReverseProxyImageRuntimeIsAPIKey(req) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "antigravity reverse proxy requires OAuth/session/desktop/IDE/client-token runtime credentials"}
	}
	payload, err := antigravityImagePayload(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: antigravityReverseProxyImageAccount(req),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(strings.TrimSpace(baseURL), "/") + antigravityGeneratePath,
		Headers: http.Header{
			"Accept":       {"application/json"},
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyGeminiProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseAntigravityImageGenerationResponse(runtimeResp.Body, runtimeResp.StatusCode, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func antigravityPayload(req contract.ConversationRequest) (antigravityRequest, error) {
	projectID := requestSetting(req, "project_id", "antigravity_project_id", "cloudaicompanion_project")
	if projectID == "" {
		return antigravityRequest{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "antigravity reverse proxy project_id missing"}
	}
	inner := antigravityInnerRequest(req)
	payload := antigravityRequest{
		Project:     projectID,
		RequestID:   antigravityRequestID(req),
		UserAgent:   "antigravity",
		RequestType: antigravityRequestType(req),
		Model:       req.Mapping.UpstreamModelName,
		Request:     inner,
	}
	if antigravityCreditsEnabled(req) {
		payload.EnabledCreditTypes = []string{"GOOGLE_ONE_AI"}
	}
	return payload, nil
}

func antigravityImagePayload(req contract.ImageGenerationRequest) (antigravityRequest, error) {
	projectID := antigravityImageSetting(req, "project_id", "antigravity_project_id", "cloudaicompanion_project")
	if projectID == "" {
		return antigravityRequest{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "antigravity reverse proxy project_id missing"}
	}
	payload := antigravityRequest{
		Project:     projectID,
		RequestID:   antigravityImageRequestID(req),
		UserAgent:   "antigravity",
		RequestType: "image_gen",
		Model:       req.Mapping.UpstreamModelName,
		Request: antigravityGenerateTextRequest{
			Contents: []geminiContent{{
				Role:  "user",
				Parts: []geminiPart{{Text: strings.TrimSpace(req.Prompt)}},
			}},
			SafetySettings: antigravitySafetySettings(),
		},
	}
	if antigravityImageCreditsEnabled(req) {
		payload.EnabledCreditTypes = []string{"GOOGLE_ONE_AI"}
	}
	return payload, nil
}

func antigravityInnerRequest(req contract.ConversationRequest) antigravityGenerateTextRequest {
	inner := antigravityGenerateTextRequest{
		Contents:         geminiCompatibleContents(req),
		GenerationConfig: geminiCompatibleGenerationConfig(req),
		Tools:            antigravityTools(req),
		SafetySettings:   antigravitySafetySettings(),
		SessionID:        antigravitySessionID(req),
	}
	if system := geminiCompatibleSystem(req); system != "" {
		inner.SystemInstruction = &geminiContent{Role: "user", Parts: []geminiPart{{Text: system}}}
	}
	if len(inner.Tools) > 0 || strings.Contains(strings.ToLower(req.Mapping.UpstreamModelName), "claude") {
		inner.ToolConfig = &antigravityToolConfig{
			FunctionCallingConfig: map[string]string{"mode": "VALIDATED"},
		}
	}
	if !strings.Contains(strings.ToLower(req.Mapping.UpstreamModelName), "claude") && inner.GenerationConfig != nil {
		inner.GenerationConfig.MaxOutputTokens = nil
	}
	return inner
}

func antigravityTools(req contract.ConversationRequest) []map[string]any {
	tools := geminiCompatibleTools(req.Tools)
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		next := cloneMap(tool)
		if declarations, ok := next["functionDeclarations"].([]map[string]any); ok {
			next["functionDeclarations"] = antigravityFunctionDeclarations(declarations)
		}
		out = append(out, next)
	}
	return out
}

func antigravityFunctionDeclarations(values []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, declaration := range values {
		next := cloneMap(declaration)
		if params, ok := next["parameters"].(map[string]any); ok {
			next["parameters"] = antigravityCleanSchema(params)
		}
		out = append(out, next)
	}
	return out
}

func antigravityCleanSchema(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	nullable, _ := value["nullable"].(bool)
	for key, item := range value {
		switch key {
		case "$schema", "$id", "deprecated", "enumTitles", "prefill":
			continue
		case "nullable":
			continue
		case "type":
			if nullable {
				out["type"] = antigravityNullableType(item)
				continue
			}
		}
		out[key] = antigravityCleanSchemaValue(item)
	}
	if nullable {
		if _, ok := value["type"]; !ok {
			out["type"] = antigravityNullableType(nil)
		}
	}
	return out
}

func antigravityCleanSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return antigravityCleanSchema(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, antigravityCleanSchemaValue(item))
		}
		return out
	default:
		return value
	}
}

func antigravityNullableType(value any) any {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return []string{"null"}
		}
		return []string{typed, "null"}
	case []any:
		out := append([]any(nil), typed...)
		for _, item := range out {
			if item == "null" {
				return out
			}
		}
		return append(out, "null")
	default:
		return []string{"null"}
	}
}

func antigravitySafetySettings() []antigravitySafetyEntry {
	return []antigravitySafetyEntry{
		{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_CIVIC_INTEGRITY", Threshold: "OFF"},
	}
}

func antigravityEndpoint(baseURL string, stream bool) string {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if stream {
		endpoint += antigravityStreamPath
		return appendAntigravityAlt(endpoint, "sse")
	}
	return endpoint + antigravityGeneratePath
}

func appendAntigravityAlt(rawURL string, alt string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		separator := "?"
		if strings.Contains(rawURL, "?") {
			separator = "&"
		}
		return rawURL + separator + "alt=" + url.QueryEscape(alt)
	}
	query := parsed.Query()
	if query.Get("alt") == "" {
		query.Set("alt", alt)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func antigravityHeaders(req contract.ConversationRequest) http.Header {
	headers := http.Header{
		"Content-Type": {"application/json"},
	}
	if req.Stream {
		headers.Set("Accept", "text/event-stream")
		headers.Set("Accept-Encoding", "identity")
	} else {
		headers.Set("Accept", "application/json")
	}
	return headers
}

func parseAntigravityResponse(body []byte) (geminiGenerateContentResponse, error) {
	var envelope antigravityResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && (len(envelope.Response.Candidates) > 0 || geminiUsageMetadataPresent(envelope.Response.Usage())) {
		return envelope.Response, nil
	}
	var direct geminiGenerateContentResponse
	if err := json.Unmarshal(body, &direct); err != nil {
		return geminiGenerateContentResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	return direct, nil
}

func parseAntigravityImageGenerationResponse(body []byte, statusCode int, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	decoded, err := parseAntigravityResponse(body)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	images := antigravityImagesFromResponse(decoded, req)
	if len(images) == 0 {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no images"}
	}
	return contract.ImageGenerationResponse{
		Created:    time.Now().Unix(),
		Data:       images,
		Model:      strings.TrimSpace(req.Mapping.UpstreamModelName),
		StatusCode: statusCode,
		Usage:      decoded.Usage().ToImageUsage(req),
	}, nil
}

func antigravityImagesFromResponse(resp geminiGenerateContentResponse, req contract.ImageGenerationRequest) []contract.Image {
	images := make([]contract.Image, 0)
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			inlineData := part.InlineData
			if len(inlineData) == 0 {
				inlineData = part.InlineDataSnake
			}
			data := strings.TrimSpace(mapString(inlineData, "data"))
			if data == "" {
				continue
			}
			mimeType := firstNonEmpty(mapString(inlineData, "mimeType"), mapString(inlineData, "mime_type"))
			if mimeType == "" {
				mimeType = "image/png"
			}
			image := contract.Image{
				Metadata: map[string]any{"mime_type": mimeType},
			}
			if strings.EqualFold(strings.TrimSpace(req.ResponseFormat), "b64_json") {
				image.Base64JSON = data
			} else {
				image.URL = "data:" + mimeType + ";base64," + data
			}
			images = append(images, image)
		}
	}
	return images
}

func geminiUsageMetadataPresent(usage geminiUsageMetadata) bool {
	return usage.PromptTokenCount != nil ||
		usage.CandidatesTokenCount != nil ||
		usage.ThoughtsTokenCount != nil ||
		usage.TotalTokenCount != nil ||
		usage.CachedContentTokenCount != nil
}

func parseAntigravityStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	var usage geminiUsageMetadata
	var parts []contract.ContentPart
	stopReason := contract.StopReasonEndTurn
	seenChunk := false
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		if providerErr, ok := providerErrorFromStreamFrame(frame, data, "gemini-compatible"); ok {
			return contract.ConversationResponse{}, providerErr
		}
		chunk, err := parseAntigravityResponse([]byte(data))
		if err != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		seenChunk = true
		parts = appendStreamContentParts(parts, chunk.ContentParts())
		if reason := chunk.StopReason(); reason != contract.StopReasonEndTurn {
			stopReason = reason
		}
		usage.Merge(chunk.Usage())
	}
	if !seenChunk {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:      parts,
		StopReason: stopReason,
		StatusCode: statusCode,
		Usage:      usage.ToUsage(text),
	}, nil
}

func antigravityReverseProxyAccount(req contract.ConversationRequest) reverseproxycontract.AccountRuntime {
	upstreamClient := req.Account.UpstreamClient
	if upstreamClient == nil || strings.TrimSpace(*upstreamClient) == "" {
		value := "antigravity_desktop"
		upstreamClient = &value
	}
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: upstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Metadata:       req.Account.Metadata,
		Credential:     req.Credential,
	}
}

func isAntigravityReverseProxy(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-antigravity")
}

func antigravityCreditsEnabled(req contract.ConversationRequest) bool {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"antigravity_credits_enabled", "antigravity_credits", "antigravity-credits"} {
			if mapBool(values, key) {
				return true
			}
		}
	}
	return false
}

func antigravityReverseProxyRuntimeIsAPIKey(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func antigravityReverseProxyImageRuntimeIsAPIKey(req contract.ImageGenerationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func antigravityReverseProxyImageAccount(req contract.ImageGenerationRequest) reverseproxycontract.AccountRuntime {
	upstreamClient := req.Account.UpstreamClient
	if upstreamClient == nil || strings.TrimSpace(*upstreamClient) == "" {
		value := "antigravity_desktop"
		upstreamClient = &value
	}
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: upstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Metadata:       req.Account.Metadata,
		Credential:     req.Credential,
	}
}

func isAntigravityImageGenerationReverseProxy(req contract.ImageGenerationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-antigravity")
}

func antigravityImageCreditsEnabled(req contract.ImageGenerationRequest) bool {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"antigravity_credits_enabled", "antigravity_credits", "antigravity-credits"} {
			if mapBool(values, key) {
				return true
			}
		}
	}
	return false
}

func antigravityRequestType(req contract.ConversationRequest) string {
	if value := requestSetting(req, "antigravity_request_type", "request_type"); value != "" {
		return value
	}
	if strings.Contains(strings.ToLower(req.Mapping.UpstreamModelName), "image") {
		return "image_gen"
	}
	return "agent"
}

func antigravityRequestID(req contract.ConversationRequest) string {
	if value := requestSetting(req, "antigravity_request_id", "request_id"); value != "" {
		return value
	}
	if strings.Contains(strings.ToLower(req.Mapping.UpstreamModelName), "image") {
		return fmt.Sprintf("image_gen/%s/%s/12", strconv.FormatInt(timeNowMillis(), 10), randomHex(16))
	}
	return "agent-" + randomHex(16)
}

func antigravityImageRequestID(req contract.ImageGenerationRequest) string {
	if value := antigravityImageSetting(req, "antigravity_request_id", "request_id"); value != "" {
		return value
	}
	return fmt.Sprintf("image_gen/%s/%s/12", strconv.FormatInt(timeNowMillis(), 10), randomHex(16))
}

func antigravitySessionID(req contract.ConversationRequest) string {
	if value := requestSetting(req, "antigravity_session_id", "session_id"); value != "" {
		return value
	}
	for _, content := range antigravityInnerContentsForSession(req) {
		if text := strings.TrimSpace(content); text != "" {
			sum := sha256.Sum256([]byte(text))
			n := int64(binary.BigEndian.Uint64(sum[:8])) & 0x7FFFFFFFFFFFFFFF
			return "-" + strconv.FormatInt(n, 10)
		}
	}
	return "-" + strconv.FormatInt(timeNowMillis(), 10)
}

func antigravityImageSetting(req contract.ImageGenerationRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Extra, req.Account.Metadata, req.Credential, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func antigravityInnerContentsForSession(req contract.ConversationRequest) []string {
	out := make([]string, 0, len(req.Messages)+1)
	for _, message := range req.Messages {
		if strings.TrimSpace(message.Role) != "user" {
			continue
		}
		out = append(out, conversationMessageText(message))
	}
	if len(out) == 0 {
		out = append(out, conversationPrompt(req))
	}
	return out
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d", timeNowMillis())))
		return hex.EncodeToString(sum[:bytesLen])
	}
	return hex.EncodeToString(buf)
}

func timeNowMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
