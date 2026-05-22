package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func (s *Service) prepareOpenAIRealtime(_ context.Context, req contract.RealtimeRequest, baseURL string) (contract.RealtimeSession, error) {
	if openAIRealtimeRuntimeIsAPIKey(req) {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "OpenAI realtime reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
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

func openAIRealtimeRuntimeIsAPIKey(req contract.RealtimeRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func isOpenAIRealtimeReverseProxy(req contract.RealtimeRequest) bool {
	adapterType := strings.TrimSpace(strings.ToLower(req.Provider.AdapterType))
	if adapterType == "reverse-proxy-codex-cli" {
		return false
	}
	if adapterType == "reverse-proxy-openai-compatible" {
		return true
	}
	if adapterType != "openai-compatible" && adapterType != "native-openai" {
		return false
	}
	return !openAIRealtimeRuntimeIsAPIKey(req)
}
