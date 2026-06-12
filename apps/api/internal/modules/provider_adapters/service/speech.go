package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeAudioSpeech(ctx context.Context, req contract.AudioSpeechRequest) (contract.AudioSpeechResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || strings.TrimSpace(req.Input) == "" || strings.TrimSpace(req.Voice) == "" {
		return contract.AudioSpeechResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLSpeech(req); baseURL != "" {
		if isReverseProxyAudioSpeechRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleAudioSpeech(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleAudioSpeech(ctx, req, baseURL)
	}
	if isReverseProxyAudioSpeechRuntime(req) {
		return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	return synthesizeLocalAudioSpeech(req), nil
}

func (s *Service) invokeOpenAICompatibleAudioSpeech(ctx context.Context, req contract.AudioSpeechRequest, baseURL string) (contract.AudioSpeechResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := json.Marshal(openAIAudioSpeechPayload(req))
	if err != nil {
		return contract.AudioSpeechResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/audio/speech", bytes.NewReader(raw))
	if err != nil {
		return contract.AudioSpeechResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.AudioSpeechResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	parsed, err := parseOpenAICompatibleAudioSpeech(body, resp.Header.Get("Content-Type"), resp.StatusCode, req)
	if err != nil {
		return contract.AudioSpeechResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleAudioSpeech(ctx context.Context, req contract.AudioSpeechRequest, baseURL string) (contract.AudioSpeechResponse, error) {
	if s.reverseProxy == nil {
		return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	raw, err := json.Marshal(openAIAudioSpeechPayload(req))
	if err != nil {
		return contract.AudioSpeechResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Metadata:       req.Account.Metadata,
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/audio/speech",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.AudioSpeechResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.AudioSpeechResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAICompatibleAudioSpeech(runtimeResp.Body, runtimeResp.Headers.Get("Content-Type"), runtimeResp.StatusCode, req)
	if err != nil {
		return contract.AudioSpeechResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func openAIAudioSpeechPayload(req contract.AudioSpeechRequest) map[string]any {
	payload := map[string]any{
		"model": req.Mapping.UpstreamModelName,
		"input": strings.TrimSpace(req.Input),
		"voice": strings.TrimSpace(req.Voice),
	}
	if format := audioSpeechResponseFormat(req); format != "" {
		payload["response_format"] = format
	}
	if req.Speed != nil {
		payload["speed"] = *req.Speed
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		payload["instructions"] = instructions
	}
	if user := strings.TrimSpace(req.User); user != "" {
		payload["user"] = user
	}
	for key, value := range req.Extra {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		switch key {
		case "model", "input", "voice", "response_format", "speed", "instructions", "user":
			continue
		}
		payload[key] = value
	}
	return payload
}

func parseOpenAICompatibleAudioSpeech(body []byte, contentType string, statusCode int, req contract.AudioSpeechRequest) (contract.AudioSpeechResponse, error) {
	if len(body) == 0 {
		return contract.AudioSpeechResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned empty audio"}
	}
	model := strings.TrimSpace(req.Mapping.UpstreamModelName)
	if model == "" {
		model = strings.TrimSpace(req.Model)
	}
	return contract.AudioSpeechResponse{
		ID:          fmt.Sprintf("speech_%s", url.PathEscape(model)),
		Audio:       append([]byte(nil), body...),
		ContentType: audioSpeechProviderContentType(contentType, req.ResponseFormat),
		Model:       model,
		StatusCode:  statusCode,
		Usage:       estimatedAudioSpeechUsage(req),
	}, nil
}

func audioSpeechResponseFormat(req contract.AudioSpeechRequest) string {
	format := strings.TrimSpace(req.ResponseFormat)
	if format == "" {
		return "mp3"
	}
	return format
}

func audioSpeechProviderContentType(contentType string, format string) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType != "" {
		return contentType
	}
	switch strings.TrimSpace(format) {
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "application/octet-stream"
	default:
		return "audio/mpeg"
	}
}

func upstreamBaseURLSpeech(req contract.AudioSpeechRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url", "audio_base_url", "speech_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func isReverseProxyAudioSpeechRuntime(req contract.AudioSpeechRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func synthesizeLocalAudioSpeech(req contract.AudioSpeechRequest) contract.AudioSpeechResponse {
	format := audioSpeechResponseFormat(req)
	audio := []byte("SRapi local speech:" + strings.TrimSpace(req.Voice) + ":" + strings.TrimSpace(req.Input))
	return contract.AudioSpeechResponse{
		ID:          "speech_local",
		Audio:       audio,
		ContentType: audioSpeechProviderContentType("", format),
		Model:       req.Mapping.UpstreamModelName,
		StatusCode:  http.StatusOK,
		Usage:       estimatedAudioSpeechUsage(req),
	}
}

func estimatedAudioSpeechUsage(req contract.AudioSpeechRequest) contract.Usage {
	text := strings.TrimSpace(req.Input)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		text += "\n" + instructions
	}
	return contract.Usage{
		InputTokens:  estimateTokens(text),
		OutputTokens: max(1, len(req.Input)/12),
		Estimated:    true,
	}
}
