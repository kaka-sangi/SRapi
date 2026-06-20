package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func (s *Service) prepareOpenAIRealtime(_ context.Context, req contract.RealtimeRequest, baseURL string) (contract.RealtimeSession, error) {
	wsURL, err := openAIRealtimeWebSocketURL(strings.TrimRight(baseURL, "/")+"/realtime", req.Mapping.UpstreamModelName)
	if err != nil {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	return contract.RealtimeSession{
		URL:     wsURL,
		Headers: openAIRealtimeHeaders(req),
	}, nil
}

func openAIRealtimeWebSocketURL(rawURL string, model string) (string, error) {
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
		return "", fmt.Errorf("OpenAI realtime websocket upstream URL scheme %q is unsupported", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("OpenAI realtime websocket upstream URL host is empty")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("OpenAI realtime upstream model is empty")
	}
	query := parsed.Query()
	query.Set("model", strings.TrimSpace(model))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func openAIRealtimeHeaders(req contract.RealtimeRequest) http.Header {
	headers := http.Header{}
	if safetyID := strings.TrimSpace(req.Headers.Get("OpenAI-Safety-Identifier")); safetyID != "" {
		headers.Set("OpenAI-Safety-Identifier", safetyID)
	}
	return headers
}

func isOpenAIRealtimeCompatible(req contract.RealtimeRequest) bool {
	adapterType := strings.TrimSpace(strings.ToLower(req.Provider.AdapterType))
	if adapterType == "reverse-proxy-codex-cli" {
		return false
	}
	if adapterType == "native-grok" || adapterType == "xai-compatible" {
		return false
	}
	if adapterType == "reverse-proxy-openai-compatible" {
		return !strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
	}
	return adapterType == "openai-compatible" || adapterType == "native-openai"
}

func isXAIResponsesWebSocket(req contract.RealtimeRequest) bool {
	adapterType := strings.TrimSpace(strings.ToLower(req.Provider.AdapterType))
	return (adapterType == "native-grok" || adapterType == "xai-compatible") &&
		strings.HasSuffix(strings.ToLower(strings.TrimSpace(req.SourceEndpoint)), "/responses/ws")
}

func (s *Service) prepareXAIResponsesWebSocket(_ context.Context, req contract.RealtimeRequest, baseURL string) (contract.RealtimeSession, error) {
	wsURL, err := xAIResponsesWebSocketURL(strings.TrimRight(baseURL, "/") + "/responses")
	if err != nil {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	frame := xAIResponsesWebSocketInitialFrame(req)
	headers := http.Header{}
	if conversationID := xAIResponsesWebSocketConversationID(req, frame); conversationID != "" {
		headers.Set("x-grok-conv-id", conversationID)
	}
	return contract.RealtimeSession{
		URL:          wsURL,
		Headers:      headers,
		InitialFrame: frame,
	}, nil
}

func xAIResponsesWebSocketURL(rawURL string) (string, error) {
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
		return "", fmt.Errorf("xAI responses websocket upstream URL scheme %q is unsupported", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("xAI responses websocket upstream URL host is empty")
	}
	return parsed.String(), nil
}

func xAIResponsesWebSocketInitialFrame(req contract.RealtimeRequest) []byte {
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(req.RequestPayload), &payload); err != nil {
		return append([]byte(nil), req.RequestPayload...)
	}
	openAIApplyResponsesPayloadDefaults(xAIRealtimeConversationRequest(req), payload)
	normalizeOpenAIResponsesServiceTier(payload)
	normalizeOpenAIResponsesImageGenerationTools(payload)
	delete(payload, "background")
	delete(payload, "stream")
	payload["type"] = "response.create"
	encoded, err := json.Marshal(payload)
	if err != nil {
		return append([]byte(nil), req.RequestPayload...)
	}
	return encoded
}

func xAIRealtimeConversationRequest(req contract.RealtimeRequest) contract.ConversationRequest {
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

func xAIResponsesWebSocketConversationID(req contract.RealtimeRequest, frame []byte) string {
	if value := realtimeSetting(req, "x_grok_conv_id", "x-grok-conv-id", "grok_conversation_id", "conversation_id", "prompt_cache_key"); value != "" {
		return value
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(frame), &payload); err != nil {
		return strings.TrimSpace(req.RequestID)
	}
	if value := codexStringValue(payload["prompt_cache_key"]); value != "" {
		return value
	}
	return strings.TrimSpace(req.RequestID)
}
