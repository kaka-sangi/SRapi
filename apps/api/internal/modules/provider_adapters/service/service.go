package service

import (
	"bufio"
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

type Service struct {
	client       *http.Client
	reverseProxy reverseproxycontract.Runtime
}

func New(client *http.Client) (*Service, error) {
	return NewWithReverseProxy(client, nil)
}

func NewWithReverseProxy(client *http.Client, runtime reverseproxycontract.Runtime) (*Service, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Service{client: client, reverseProxy: runtime}, nil
}

func (s *Service) InvokeText(ctx context.Context, req contract.TextRequest) (contract.TextResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.TextResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURL(req); baseURL != "" {
		if isCodexReverseProxy(req) {
			return s.invokeReverseProxyCodexResponses(ctx, req, baseURL)
		}
		if isGeminiCompatible(req) {
			if isReverseProxyRuntime(req) {
				return s.invokeReverseProxyGeminiCompatible(ctx, req, baseURL)
			}
			return s.invokeGeminiCompatible(ctx, req, baseURL)
		}
		if isAnthropicCompatible(req) {
			if isReverseProxyRuntime(req) {
				return s.invokeReverseProxyAnthropicCompatible(ctx, req, baseURL)
			}
			return s.invokeAnthropicCompatible(ctx, req, baseURL)
		}
		if isReverseProxyRuntime(req) {
			return s.invokeReverseProxyOpenAICompatible(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatible(ctx, req, baseURL)
	}
	if isReverseProxyRuntime(req) {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	text := synthesizeLocalText(req.Model, req.Prompt)
	return contract.TextResponse{
		Text:       text,
		StatusCode: http.StatusOK,
		Usage:      estimatedUsage(text),
	}, nil
}

func (s *Service) PrepareRealtime(ctx context.Context, req contract.RealtimeRequest) (contract.RealtimeSession, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.RealtimeSession{}, ErrInvalidInput
	}
	baseURL := upstreamBaseURLRealtime(req)
	if baseURL == "" {
		return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "realtime upstream base url missing"}
	}
	if isCodexRealtimeReverseProxy(req) {
		return s.prepareCodexRealtime(ctx, req, baseURL)
	}
	return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "realtime reverse proxy adapter unsupported"}
}

func (s *Service) InvokeEmbeddings(ctx context.Context, req contract.EmbeddingRequest) (contract.EmbeddingResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || len(req.Input) == 0 {
		return contract.EmbeddingResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLEmbeddings(req); baseURL != "" {
		if isReverseProxyEmbeddingRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleEmbeddings(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleEmbeddings(ctx, req, baseURL)
	}
	if isReverseProxyEmbeddingRuntime(req) {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	return synthesizeLocalEmbeddings(req), nil
}

func (s *Service) InvokeImageGeneration(ctx context.Context, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || strings.TrimSpace(req.Prompt) == "" {
		return contract.ImageGenerationResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLImages(req); baseURL != "" {
		if isReverseProxyImageRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleImages(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleImages(ctx, req, baseURL)
	}
	if isReverseProxyImageRuntime(req) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	return synthesizeLocalImages(req), nil
}

func (s *Service) invokeOpenAICompatibleEmbeddings(ctx context.Context, req contract.EmbeddingRequest, baseURL string) (contract.EmbeddingResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := json.Marshal(openAIEmbeddingPayload(req))
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/embeddings", bytes.NewReader(raw))
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.EmbeddingResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.EmbeddingResponse{}, classifyProviderHTTPError(resp.StatusCode, body)
	}
	return parseOpenAICompatibleEmbeddings(body, resp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
}

func (s *Service) invokeReverseProxyOpenAICompatibleEmbeddings(ctx context.Context, req contract.EmbeddingRequest, baseURL string) (contract.EmbeddingResponse, error) {
	if s.reverseProxy == nil {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	raw, err := json.Marshal(openAIEmbeddingPayload(req))
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/embeddings",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.EmbeddingResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.EmbeddingResponse{}, classifyProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	return parseOpenAICompatibleEmbeddings(runtimeResp.Body, runtimeResp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
}

func (s *Service) invokeOpenAICompatibleImages(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := json.Marshal(openAIImageGenerationPayload(req))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/images/generations", bytes.NewReader(raw))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyProviderHTTPError(resp.StatusCode, body)
	}
	return parseOpenAICompatibleImages(body, resp.StatusCode, req.Mapping.UpstreamModelName, req)
}

func (s *Service) invokeReverseProxyOpenAICompatibleImages(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	raw, err := json.Marshal(openAIImageGenerationPayload(req))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/images/generations",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	return parseOpenAICompatibleImages(runtimeResp.Body, runtimeResp.StatusCode, req.Mapping.UpstreamModelName, req)
}

func (s *Service) invokeGeminiCompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	apiKey := geminiAPIKey(req)
	if apiKey == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	payload := geminiCompatiblePayload(req)
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	endpoint := geminiEndpoint(baseURL, req.Mapping.UpstreamModelName, req.Stream)
	headers := geminiCompatibleHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return contract.TextResponse{}, err
	}
	httpReq.Header = headers

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.TextResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if req.Stream {
			return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyGeminiProviderHTTPError(resp.StatusCode, body)
	}
	if req.Stream {
		return parseGeminiCompatibleStream(body, resp.StatusCode)
	}

	var decoded geminiGenerateContentResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: resp.StatusCode,
		Usage:      decoded.UsageMetadata.ToUsage(text),
	}, nil
}

func (s *Service) invokeAnthropicCompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	apiKey := anthropicAPIKey(req)
	if apiKey == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	payload := anthropicCompatiblePayload(req)
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	headers := anthropicCompatibleHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return contract.TextResponse{}, err
	}
	httpReq.Header = headers

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.TextResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if req.Stream {
			return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyAnthropicProviderHTTPError(resp.StatusCode, body)
	}
	if req.Stream {
		return parseAnthropicCompatibleStream(body, resp.StatusCode)
	}

	var decoded anthropicMessagesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: resp.StatusCode,
		Usage:      decoded.Usage.ToUsage(text),
	}, nil
}

func (s *Service) invokeOpenAICompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	payload := openAICompatiblePayload(req)
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return contract.TextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.TextResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if req.Stream {
			return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyProviderHTTPError(resp.StatusCode, body)
	}
	if req.Stream {
		return parseOpenAICompatibleStream(body, resp.StatusCode)
	}

	var decoded openAIChatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: resp.StatusCode,
		Usage:      decoded.Usage.ToUsage(text),
	}, nil
}

func (s *Service) invokeReverseProxyGeminiCompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	if s.reverseProxy == nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	payload := geminiCompatiblePayload(req)
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	endpoint := geminiEndpoint(baseURL, req.Mapping.UpstreamModelName, req.Stream)
	headers := geminiCompatibleHeaders(req, "", &endpoint)
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method:       http.MethodPost,
		URL:          endpoint,
		Headers:      headers,
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.TextResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyGeminiProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	if req.Stream {
		return parseGeminiCompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
	}
	var decoded geminiGenerateContentResponse
	if err := json.Unmarshal(runtimeResp.Body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: runtimeResp.StatusCode,
		Usage:      decoded.UsageMetadata.ToUsage(text),
	}, nil
}

func (s *Service) invokeReverseProxyAnthropicCompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	if s.reverseProxy == nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	payload := anthropicCompatiblePayload(req)
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	headers := anthropicCompatibleHeaders(req, "", &endpoint)
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method:       http.MethodPost,
		URL:          endpoint,
		Headers:      headers,
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.TextResponse{}, providerErrorFromReverseProxy(err)
	}
	if req.Stream {
		return parseAnthropicCompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
	}
	var decoded anthropicMessagesResponse
	if err := json.Unmarshal(runtimeResp.Body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: runtimeResp.StatusCode,
		Usage:      decoded.Usage.ToUsage(text),
	}, nil
}

func (s *Service) invokeReverseProxyOpenAICompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	if s.reverseProxy == nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	payload := openAICompatiblePayload(req)
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/chat/completions",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.TextResponse{}, providerErrorFromReverseProxy(err)
	}
	if req.Stream {
		return parseOpenAICompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
	}
	var decoded openAIChatCompletionResponse
	if err := json.Unmarshal(runtimeResp.Body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: runtimeResp.StatusCode,
		Usage:      decoded.Usage.ToUsage(text),
	}, nil
}

type geminiGenerateContentRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []map[string]any        `json:"tools,omitempty"`
	ToolConfig        any                     `json:"toolConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature      *float32       `json:"temperature,omitempty"`
	TopP             *float32       `json:"topP,omitempty"`
	MaxOutputTokens  *int           `json:"maxOutputTokens,omitempty"`
	StopSequences    []string       `json:"stopSequences,omitempty"`
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

func geminiCompatiblePayload(req contract.TextRequest) geminiGenerateContentRequest {
	payload := geminiGenerateContentRequest{
		Contents:         geminiCompatibleContents(req),
		GenerationConfig: geminiCompatibleGenerationConfig(req),
		Tools:            geminiCompatibleTools(req.Tools),
		ToolConfig:       geminiCompatibleToolConfig(req.ToolChoice),
	}
	if system := geminiCompatibleSystem(req); system != "" {
		payload.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: system}},
		}
	}
	return payload
}

func geminiCompatibleContents(req contract.TextRequest) []geminiContent {
	out := make([]geminiContent, 0, len(req.Messages)+1)
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := geminiRole(message.Role)
		if role == "system" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		out = append(out, geminiContent{Role: role, Parts: []geminiPart{{Text: content}}})
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			prompt = strings.TrimSpace(req.Instructions)
		}
		out = append(out, geminiContent{Role: "user", Parts: []geminiPart{{Text: prompt}}})
	}
	return out
}

func geminiRole(role string) string {
	switch strings.TrimSpace(role) {
	case "assistant", "model":
		return "model"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func geminiCompatibleSystem(req contract.TextRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, message := range req.Messages {
		if strings.TrimSpace(message.Role) != "system" {
			continue
		}
		if content := strings.TrimSpace(message.Content); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(uniqueTrimmedStrings(parts), "\n")
}

func geminiCompatibleGenerationConfig(req contract.TextRequest) *geminiGenerationConfig {
	cfg := &geminiGenerationConfig{
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxOutputTokens,
		StopSequences:   cloneStrings(req.Stop),
	}
	if len(req.ResponseFormat) > 0 {
		cfg.ResponseMimeType = geminiResponseMimeType(req.ResponseFormat)
		cfg.ResponseSchema = geminiResponseSchema(req.ResponseFormat)
	}
	if cfg.Temperature == nil && cfg.TopP == nil && cfg.MaxOutputTokens == nil && len(cfg.StopSequences) == 0 && cfg.ResponseMimeType == "" && len(cfg.ResponseSchema) == 0 {
		return nil
	}
	return cfg
}

func geminiResponseMimeType(format map[string]any) string {
	for _, key := range []string{"responseMimeType", "response_mime_type", "mime_type", "type"} {
		value := strings.TrimSpace(fmt.Sprint(format[key]))
		if value != "" && value != "<nil>" {
			if value == "json_object" || value == "json_schema" {
				return "application/json"
			}
			return value
		}
	}
	if len(format) > 0 {
		return "application/json"
	}
	return ""
}

func geminiResponseSchema(format map[string]any) map[string]any {
	for _, key := range []string{"responseSchema", "response_schema", "schema"} {
		if value, ok := format[key].(map[string]any); ok {
			return cloneMap(value)
		}
	}
	if value, ok := format["json_schema"].(map[string]any); ok {
		if schema, ok := value["schema"].(map[string]any); ok {
			return cloneMap(schema)
		}
		return cloneMap(value)
	}
	return nil
}

func geminiCompatibleTools(values []map[string]any) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	functionDeclarations := make([]map[string]any, 0, len(values))
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		if declaration := geminiFunctionDeclaration(value); len(declaration) > 0 {
			functionDeclarations = append(functionDeclarations, declaration)
			continue
		}
		out = append(out, cloneMap(value))
	}
	if len(functionDeclarations) > 0 {
		out = append(out, map[string]any{"functionDeclarations": functionDeclarations})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiFunctionDeclaration(value map[string]any) map[string]any {
	function, ok := value["function"].(map[string]any)
	if !ok {
		return nil
	}
	declaration := map[string]any{}
	if name := strings.TrimSpace(fmt.Sprint(function["name"])); name != "" && name != "<nil>" {
		declaration["name"] = name
	}
	if description := strings.TrimSpace(fmt.Sprint(function["description"])); description != "" && description != "<nil>" {
		declaration["description"] = description
	}
	if parameters, ok := function["parameters"]; ok && parameters != nil {
		declaration["parameters"] = cloneAny(parameters)
	}
	return declaration
}

func geminiCompatibleToolConfig(value any) any {
	if value == nil {
		return nil
	}
	return cloneAny(value)
}

type anthropicMessagesRequest struct {
	Model         string             `json:"model"`
	Messages      []anthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	Stream        bool               `json:"stream"`
	MaxTokens     int                `json:"max_tokens"`
	Temperature   *float32           `json:"temperature,omitempty"`
	TopP          *float32           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []map[string]any   `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func anthropicCompatiblePayload(req contract.TextRequest) anthropicMessagesRequest {
	maxTokens := 1024
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		maxTokens = *req.MaxOutputTokens
	}
	return anthropicMessagesRequest{
		Model:         req.Mapping.UpstreamModelName,
		Messages:      anthropicCompatibleMessages(req),
		System:        anthropicCompatibleSystem(req),
		Stream:        req.Stream,
		MaxTokens:     maxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: cloneStrings(req.Stop),
		Tools:         anthropicCompatibleTools(req.Tools),
		ToolChoice:    anthropicCompatibleToolChoice(req.ToolChoice),
	}
}

func anthropicCompatibleMessages(req contract.TextRequest) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(req.Messages)+1)
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		switch role {
		case "":
			role = "user"
		case "system":
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		out = append(out, anthropicMessage{Role: role, Content: content})
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		out = append(out, anthropicMessage{Role: "user", Content: strings.TrimSpace(req.Prompt)})
	}
	return out
}

func anthropicCompatibleSystem(req contract.TextRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, message := range req.Messages {
		if strings.TrimSpace(message.Role) != "system" {
			continue
		}
		if content := strings.TrimSpace(message.Content); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(uniqueTrimmedStrings(parts), "\n")
}

func anthropicCompatibleTools(values []map[string]any) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		if function, ok := value["function"].(map[string]any); ok {
			tool := map[string]any{}
			if name := strings.TrimSpace(fmt.Sprint(function["name"])); name != "" && name != "<nil>" {
				tool["name"] = name
			}
			if description := strings.TrimSpace(fmt.Sprint(function["description"])); description != "" && description != "<nil>" {
				tool["description"] = description
			}
			if parameters, ok := function["parameters"]; ok && parameters != nil {
				tool["input_schema"] = cloneAny(parameters)
			}
			if len(tool) > 0 {
				out = append(out, tool)
			}
			continue
		}
		tool := cloneMap(value)
		delete(tool, "type")
		if len(tool) > 0 {
			out = append(out, tool)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func anthropicCompatibleToolChoice(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		switch strings.TrimSpace(typed) {
		case "auto":
			return map[string]any{"type": "auto"}
		case "required", "any":
			return map[string]any{"type": "any"}
		case "none":
			return map[string]any{"type": "none"}
		default:
			return nil
		}
	case map[string]any:
		if choiceType, ok := typed["type"].(string); ok {
			switch strings.TrimSpace(choiceType) {
			case "auto", "any", "none":
				return cloneMap(typed)
			case "function":
				if function, ok := typed["function"].(map[string]any); ok {
					if name := strings.TrimSpace(fmt.Sprint(function["name"])); name != "" && name != "<nil>" {
						return map[string]any{"type": "tool", "name": name}
					}
				}
			}
		}
		return cloneMap(typed)
	default:
		return cloneAny(value)
	}
}

type openAIChatCompletionRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIChatMessage  `json:"messages"`
	Stream         bool                 `json:"stream"`
	StreamOptions  *openAIStreamOptions `json:"stream_options,omitempty"`
	Temperature    *float32             `json:"temperature,omitempty"`
	TopP           *float32             `json:"top_p,omitempty"`
	MaxTokens      *int                 `json:"max_tokens,omitempty"`
	Stop           []string             `json:"stop,omitempty"`
	Tools          []map[string]any     `json:"tools,omitempty"`
	ToolChoice     any                  `json:"tool_choice,omitempty"`
	ResponseFormat map[string]any       `json:"response_format,omitempty"`
}

type openAIEmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
	Dimensions     *int     `json:"dimensions,omitempty"`
	User           string   `json:"user,omitempty"`
}

type openAIImageGenerationRequest struct {
	Model          string         `json:"model"`
	Prompt         string         `json:"prompt"`
	N              int            `json:"n,omitempty"`
	Size           string         `json:"size,omitempty"`
	Quality        string         `json:"quality,omitempty"`
	Style          string         `json:"style,omitempty"`
	ResponseFormat string         `json:"response_format,omitempty"`
	User           string         `json:"user,omitempty"`
	Extra          map[string]any `json:"-"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func openAICompatiblePayload(req contract.TextRequest) openAIChatCompletionRequest {
	payload := openAIChatCompletionRequest{
		Model:          req.Mapping.UpstreamModelName,
		Messages:       openAICompatibleMessages(req),
		Stream:         req.Stream,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxOutputTokens,
		Stop:           cloneStrings(req.Stop),
		Tools:          cloneMapSlice(req.Tools),
		ToolChoice:     cloneAny(req.ToolChoice),
		ResponseFormat: cloneMap(req.ResponseFormat),
	}
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	return payload
}

func openAIEmbeddingPayload(req contract.EmbeddingRequest) openAIEmbeddingRequest {
	encoding := strings.TrimSpace(req.EncodingFormat)
	if encoding == "" {
		encoding = "float"
	}
	return openAIEmbeddingRequest{
		Model:          req.Mapping.UpstreamModelName,
		Input:          append([]string(nil), req.Input...),
		EncodingFormat: encoding,
		Dimensions:     cloneIntPtr(req.Dimensions),
		User:           strings.TrimSpace(req.User),
	}
}

func openAIImageGenerationPayload(req contract.ImageGenerationRequest) openAIImageGenerationRequest {
	return openAIImageGenerationRequest{
		Model:          req.Mapping.UpstreamModelName,
		Prompt:         strings.TrimSpace(req.Prompt),
		N:              req.Count,
		Size:           strings.TrimSpace(req.Size),
		Quality:        strings.TrimSpace(req.Quality),
		Style:          strings.TrimSpace(req.Style),
		ResponseFormat: strings.TrimSpace(req.ResponseFormat),
		User:           strings.TrimSpace(req.User),
		Extra:          cloneMap(req.Extra),
	}
}

func (r openAIImageGenerationRequest) MarshalJSON() ([]byte, error) {
	type alias openAIImageGenerationRequest
	raw, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	delete(payload, "Extra")
	for key, value := range r.Extra {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	return json.Marshal(payload)
}

func openAICompatibleMessages(req contract.TextRequest) []openAIChatMessage {
	out := make([]openAIChatMessage, 0, len(req.Messages)+2)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		out = append(out, openAIChatMessage{Role: "system", Content: instructions})
	}
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		out = append(out, openAIChatMessage{Role: role, Content: content})
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		out = append(out, openAIChatMessage{Role: "user", Content: strings.TrimSpace(req.Prompt)})
	}
	return out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, len(values))
	for idx, value := range values {
		out[idx] = cloneMap(value)
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneAny(item)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []map[string]any:
		return cloneMapSlice(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAny(item)
		}
		return out
	default:
		return typed
	}
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
	Usage openAIUsage `json:"usage"`
}

type openAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string          `json:"object"`
		Embedding json.RawMessage `json:"embedding"`
		Index     int             `json:"index"`
	} `json:"data"`
	Model string      `json:"model"`
	Usage openAIUsage `json:"usage"`
}

type openAIImageGenerationResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string         `json:"url"`
		Base64JSON    string         `json:"b64_json"`
		RevisedPrompt string         `json:"revised_prompt"`
		Extra         map[string]any `json:"-"`
	} `json:"data"`
	Model string      `json:"model"`
	Usage openAIUsage `json:"usage"`
}

func (r openAIChatCompletionResponse) FirstText() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}

func parseOpenAICompatibleEmbeddings(body []byte, statusCode int, fallbackModel string, input []string) (contract.EmbeddingResponse, error) {
	var decoded openAIEmbeddingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	data := make([]contract.Embedding, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		embedding, err := parseOpenAIEmbeddingValue(item.Embedding)
		if err != nil {
			return contract.EmbeddingResponse{}, err
		}
		embedding.Index = item.Index
		data = append(data, embedding)
	}
	if len(data) == 0 {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no embeddings"}
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	return contract.EmbeddingResponse{
		Data:       data,
		Model:      model,
		StatusCode: statusCode,
		Usage:      decoded.Usage.ToEmbeddingUsage(input),
	}, nil
}

func parseOpenAIEmbeddingValue(raw json.RawMessage) (contract.Embedding, error) {
	var vector []float32
	if err := json.Unmarshal(raw, &vector); err == nil && len(vector) > 0 {
		return contract.Embedding{Vector: vector}, nil
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil && strings.TrimSpace(encoded) != "" {
		return contract.Embedding{Base64Vector: strings.TrimSpace(encoded)}, nil
	}
	return contract.Embedding{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained invalid embedding vector"}
}

func parseOpenAICompatibleImages(body []byte, statusCode int, fallbackModel string, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	var decoded openAIImageGenerationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	data := make([]contract.Image, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		image := contract.Image{
			URL:           strings.TrimSpace(item.URL),
			Base64JSON:    strings.TrimSpace(item.Base64JSON),
			RevisedPrompt: strings.TrimSpace(item.RevisedPrompt),
			Metadata:      cloneMap(item.Extra),
		}
		if image.URL == "" && image.Base64JSON == "" {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained image without url or b64_json"}
		}
		data = append(data, image)
	}
	if len(data) == 0 {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no images"}
	}
	created := decoded.Created
	if created == 0 {
		created = time.Now().Unix()
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	return contract.ImageGenerationResponse{
		Created:    created,
		Data:       data,
		Model:      model,
		StatusCode: statusCode,
		Usage:      decoded.Usage.ToImageUsage(req),
	}, nil
}

func (r *openAIImageGenerationResponse) UnmarshalJSON(body []byte) error {
	var raw struct {
		Created int64            `json:"created"`
		Data    []map[string]any `json:"data"`
		Model   string           `json:"model"`
		Usage   openAIUsage      `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	r.Created = raw.Created
	r.Model = raw.Model
	r.Usage = raw.Usage
	r.Data = make([]struct {
		URL           string         `json:"url"`
		Base64JSON    string         `json:"b64_json"`
		RevisedPrompt string         `json:"revised_prompt"`
		Extra         map[string]any `json:"-"`
	}, 0, len(raw.Data))
	for _, item := range raw.Data {
		image := struct {
			URL           string         `json:"url"`
			Base64JSON    string         `json:"b64_json"`
			RevisedPrompt string         `json:"revised_prompt"`
			Extra         map[string]any `json:"-"`
		}{
			URL:           mapString(item, "url"),
			Base64JSON:    mapString(item, "b64_json"),
			RevisedPrompt: mapString(item, "revised_prompt"),
			Extra:         cloneMap(item),
		}
		delete(image.Extra, "url")
		delete(image.Extra, "b64_json")
		delete(image.Extra, "revised_prompt")
		r.Data = append(r.Data, image)
	}
	return nil
}

type openAIChatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage"`
}

type openAIUsage struct {
	PromptTokens        *int `json:"prompt_tokens"`
	CompletionTokens    *int `json:"completion_tokens"`
	TotalTokens         *int `json:"total_tokens"`
	InputTokens         *int `json:"input_tokens"`
	OutputTokens        *int `json:"output_tokens"`
	CachedTokens        *int `json:"cached_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.InputTokens)
	if input == 0 {
		input = valueOrZero(u.PromptTokens)
	}
	output := valueOrZero(u.OutputTokens)
	if output == 0 {
		output = valueOrZero(u.CompletionTokens)
	}
	cached := valueOrZero(u.CachedTokens)
	if cached == 0 && u.PromptTokensDetails != nil {
		cached = valueOrZero(u.PromptTokensDetails.CachedTokens)
	}
	total := input + output + cached
	if u.TotalTokens != nil && *u.TotalTokens > 0 && total == 0 {
		total = *u.TotalTokens
	}
	if total > 0 && output == 0 {
		output = max(0, total-input-cached)
	}
	if input == 0 && output == 0 && cached == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		Estimated:    false,
	}
}

func (u openAIUsage) ToEmbeddingUsage(input []string) contract.Usage {
	usage := u.ToUsage(strings.Join(input, "\n"))
	usage.OutputTokens = 0
	return usage
}

func (u openAIUsage) ToImageUsage(req contract.ImageGenerationRequest) contract.Usage {
	if !u.HasTokenUsage() {
		return estimatedImageUsage(req)
	}
	usage := u.ToUsage(req.Prompt)
	return usage
}

func (u openAIUsage) ToModerationUsage(input []string) contract.Usage {
	usage := u.ToUsage(strings.Join(input, "\n"))
	usage.OutputTokens = 0
	return usage
}

func (u openAIUsage) HasTokenUsage() bool {
	return u.PromptTokens != nil ||
		u.CompletionTokens != nil ||
		u.TotalTokens != nil ||
		u.InputTokens != nil ||
		u.OutputTokens != nil ||
		u.CachedTokens != nil ||
		(u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens != nil)
}

func parseOpenAICompatibleStream(body []byte, statusCode int) (contract.TextResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var builder strings.Builder
	var usage *openAIUsage
	done := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			done = true
			break
		}
		var chunk openAIChatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		if chunk.Usage != nil {
			copied := *chunk.Usage
			usage = &copied
		}
		for _, choice := range chunk.Choices {
			builder.WriteString(choice.Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	if !done {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	parsedUsage := estimatedUsage(text)
	if usage != nil {
		parsedUsage = usage.ToUsage(text)
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: statusCode,
		Usage:      parsedUsage,
	}, nil
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

func (r geminiGenerateContentResponse) FirstText() string {
	parts := make([]string, 0, len(r.Candidates))
	for _, candidate := range r.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func (r geminiGenerateContentResponse) StreamText() string {
	var builder strings.Builder
	for _, candidate := range r.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				builder.WriteString(part.Text)
			}
		}
	}
	return builder.String()
}

type geminiUsageMetadata struct {
	PromptTokenCount        *int `json:"promptTokenCount"`
	CandidatesTokenCount    *int `json:"candidatesTokenCount"`
	TotalTokenCount         *int `json:"totalTokenCount"`
	CachedContentTokenCount *int `json:"cachedContentTokenCount"`
}

func (u geminiUsageMetadata) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.PromptTokenCount)
	output := valueOrZero(u.CandidatesTokenCount)
	cached := valueOrZero(u.CachedContentTokenCount)
	total := input + output + cached
	if u.TotalTokenCount != nil && *u.TotalTokenCount > 0 && total == 0 {
		total = *u.TotalTokenCount
	}
	if total > 0 && output == 0 {
		output = max(0, total-input-cached)
	}
	if input == 0 && output == 0 && cached == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		Estimated:    false,
	}
}

func (u *geminiUsageMetadata) Merge(next geminiUsageMetadata) {
	if u == nil {
		return
	}
	if next.PromptTokenCount != nil {
		u.PromptTokenCount = cloneIntPtr(next.PromptTokenCount)
	}
	if next.CandidatesTokenCount != nil {
		u.CandidatesTokenCount = cloneIntPtr(next.CandidatesTokenCount)
	}
	if next.TotalTokenCount != nil {
		u.TotalTokenCount = cloneIntPtr(next.TotalTokenCount)
	}
	if next.CachedContentTokenCount != nil {
		u.CachedContentTokenCount = cloneIntPtr(next.CachedContentTokenCount)
	}
}

func parseGeminiCompatibleStream(body []byte, statusCode int) (contract.TextResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var builder strings.Builder
	var usage geminiUsageMetadata
	seenChunk := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var chunk geminiGenerateContentResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		seenChunk = true
		if text := chunk.StreamText(); strings.TrimSpace(text) != "" {
			builder.WriteString(text)
		}
		usage.Merge(chunk.UsageMetadata)
	}
	if err := scanner.Err(); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	if !seenChunk {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: statusCode,
		Usage:      usage.ToUsage(text),
	}, nil
}

type anthropicMessagesResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
}

func (r anthropicMessagesResponse) FirstText() string {
	parts := make([]string, 0, len(r.Content))
	for _, block := range r.Content {
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n")
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicStreamChunk struct {
	Type         string                 `json:"type"`
	ContentBlock *anthropicContentBlock `json:"content_block"`
	Delta        struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Message *struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.InputTokens)
	output := valueOrZero(u.OutputTokens)
	cached := valueOrZero(u.CacheCreationInputTokens) + valueOrZero(u.CacheReadInputTokens)
	if input == 0 && output == 0 && cached == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		Estimated:    false,
	}
}

func (u *anthropicUsage) Merge(next anthropicUsage) {
	if u == nil {
		return
	}
	if next.InputTokens != nil {
		u.InputTokens = cloneIntPtr(next.InputTokens)
	}
	if next.OutputTokens != nil {
		u.OutputTokens = cloneIntPtr(next.OutputTokens)
	}
	if next.CacheCreationInputTokens != nil {
		u.CacheCreationInputTokens = cloneIntPtr(next.CacheCreationInputTokens)
	}
	if next.CacheReadInputTokens != nil {
		u.CacheReadInputTokens = cloneIntPtr(next.CacheReadInputTokens)
	}
}

func parseAnthropicCompatibleStream(body []byte, statusCode int) (contract.TextResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var builder strings.Builder
	var usage anthropicUsage
	done := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			done = true
			break
		}
		var chunk anthropicStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		switch strings.TrimSpace(chunk.Type) {
		case "content_block_start":
			if chunk.ContentBlock != nil && strings.TrimSpace(chunk.ContentBlock.Text) != "" {
				builder.WriteString(chunk.ContentBlock.Text)
			}
		case "content_block_delta":
			builder.WriteString(chunk.Delta.Text)
		case "message_start":
			if chunk.Message != nil {
				usage.Merge(chunk.Message.Usage)
			}
		case "message_delta":
			if chunk.Usage != nil {
				usage.Merge(*chunk.Usage)
			}
		case "message_stop":
			done = true
		}
	}
	if err := scanner.Err(); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	if !done {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: statusCode,
		Usage:      usage.ToUsage(text),
	}, nil
}

func upstreamBaseURL(req contract.TextRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url", "anthropic_base_url", "gemini_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func upstreamBaseURLRealtime(req contract.RealtimeRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url", "anthropic_base_url", "gemini_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func upstreamBaseURLEmbeddings(req contract.EmbeddingRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func upstreamBaseURLImages(req contract.ImageGenerationRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url", "images_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func isGeminiCompatible(req contract.TextRequest) bool {
	for _, value := range []string{req.Provider.Protocol, req.Provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "gemini-compatible", "native-gemini", "reverse-proxy-gemini-cli":
			return true
		}
	}
	return false
}

func isAnthropicCompatible(req contract.TextRequest) bool {
	for _, value := range []string{req.Provider.Protocol, req.Provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "anthropic-compatible", "reverse-proxy-claude-code-cli":
			return true
		}
	}
	return false
}

func isCodexReverseProxy(req contract.TextRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-codex-cli")
}

func isCodexRealtimeReverseProxy(req contract.RealtimeRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-codex-cli")
}

func isReverseProxyRuntime(req contract.TextRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func isReverseProxyEmbeddingRuntime(req contract.EmbeddingRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func isReverseProxyImageRuntime(req contract.ImageGenerationRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func geminiAPIKey(req contract.TextRequest) string {
	for _, key := range []string{"api_key", "gemini_api_key", "google_api_key", "access_token"} {
		if value := credentialString(req.Credential, key); value != "" {
			return value
		}
	}
	return ""
}

func geminiCompatibleHeaders(req contract.TextRequest, apiKey string, endpoint *string) http.Header {
	headers := http.Header{
		"Content-Type": {"application/json"},
	}
	if apiKey == "" {
		return headers
	}
	switch strings.ToLower(requestSetting(req, "auth_mode")) {
	case "bearer":
		headers.Set("Authorization", "Bearer "+apiKey)
	case "custom_header":
		headerName := requestSetting(req, "custom_header_name", "auth_header", "api_key_header")
		if headerName == "" {
			headerName = "x-goog-api-key"
		}
		headers.Set(headerName, apiKey)
	case "api_key_header", "x_goog_api_key":
		headers.Set("x-goog-api-key", apiKey)
	case "api_key_query", "":
		if endpoint != nil {
			*endpoint = appendAPIKeyQuery(*endpoint, apiKey, requestSetting(req, "api_key_query_param", "query_param"))
		}
	default:
		if endpoint != nil {
			*endpoint = appendAPIKeyQuery(*endpoint, apiKey, "key")
		}
	}
	return headers
}

func geminiEndpoint(baseURL string, model string, stream bool) string {
	action := ":generateContent"
	if stream {
		action = ":streamGenerateContent"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	model = strings.Trim(strings.TrimSpace(model), "/")
	model = strings.TrimPrefix(model, "models/")
	escapedModel := strings.ReplaceAll(url.PathEscape(model), "%2F", "/")
	if strings.Contains(baseURL, "/models/") || strings.HasSuffix(baseURL, "/models") || strings.HasSuffix(baseURL, ":generateContent") || strings.HasSuffix(baseURL, ":streamGenerateContent") {
		if strings.HasSuffix(baseURL, ":generateContent") || strings.HasSuffix(baseURL, ":streamGenerateContent") {
			idx := strings.LastIndex(baseURL, ":")
			return baseURL[:idx] + action
		}
		return baseURL + "/" + escapedModel + action
	}
	return baseURL + "/models/" + escapedModel + action
}

func anthropicAPIKey(req contract.TextRequest) string {
	for _, key := range []string{"api_key", "x_api_key", "anthropic_api_key", "access_token"} {
		if value := credentialString(req.Credential, key); value != "" {
			return value
		}
	}
	return ""
}

func anthropicCompatibleHeaders(req contract.TextRequest, apiKey string, endpoint *string) http.Header {
	headers := http.Header{
		"Content-Type": {"application/json"},
	}
	version := requestSetting(req, "anthropic_version", "anthropic-version")
	if version == "" {
		version = "2023-06-01"
	}
	headers.Set("anthropic-version", version)
	if apiKey == "" {
		return headers
	}
	switch strings.ToLower(requestSetting(req, "auth_mode")) {
	case "bearer":
		headers.Set("Authorization", "Bearer "+apiKey)
	case "api_key_query":
		if endpoint != nil {
			*endpoint = appendAPIKeyQuery(*endpoint, apiKey, requestSetting(req, "api_key_query_param", "query_param"))
		}
	case "custom_header":
		headerName := requestSetting(req, "custom_header_name", "auth_header", "api_key_header")
		if headerName == "" {
			headerName = "x-api-key"
		}
		headers.Set(headerName, apiKey)
	case "x_api_key", "":
		headers.Set("x-api-key", apiKey)
	default:
		headers.Set("x-api-key", apiKey)
	}
	return headers
}

func appendAPIKeyQuery(rawURL string, apiKey string, queryParam string) string {
	queryParam = strings.TrimSpace(queryParam)
	if queryParam == "" {
		queryParam = "key"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		separator := "?"
		if strings.Contains(rawURL, "?") {
			separator = "&"
		}
		return rawURL + separator + url.QueryEscape(queryParam) + "=" + url.QueryEscape(apiKey)
	}
	query := parsed.Query()
	query.Set(queryParam, apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func requestSetting(req contract.TextRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func credentialString(values map[string]any, key string) string {
	return mapString(values, key)
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func classifyProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return contract.ProviderError{Class: providerClassForHTTPStatus(statusCode), StatusCode: statusCode, Message: message}
}

func classifyAnthropicProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	var decoded struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	class := providerClassForHTTPStatus(statusCode)
	if err := json.Unmarshal(body, &decoded); err == nil {
		if decoded.Error.Message != "" {
			message = strings.TrimSpace(decoded.Error.Message)
		}
		if decoded.Error.Type != "" {
			class = providerClassForAnthropicError(decoded.Error.Type, statusCode)
		}
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
}

func classifyGeminiProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	var decoded struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	class := providerClassForHTTPStatus(statusCode)
	if err := json.Unmarshal(body, &decoded); err == nil {
		if decoded.Error.Message != "" {
			message = strings.TrimSpace(decoded.Error.Message)
		}
		if decoded.Error.Status != "" {
			class = providerClassForGeminiStatus(decoded.Error.Status, statusCode)
		}
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
}

func providerClassForGeminiStatus(status string, statusCode int) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "INVALID_ARGUMENT", "FAILED_PRECONDITION", "OUT_OF_RANGE":
		return "invalid_request"
	case "UNAUTHENTICATED", "PERMISSION_DENIED":
		return "auth_failed"
	case "RESOURCE_EXHAUSTED":
		return "rate_limit"
	case "NOT_FOUND":
		return "model_unavailable"
	case "DEADLINE_EXCEEDED":
		return "timeout"
	case "UNAVAILABLE", "INTERNAL", "UNKNOWN":
		return "provider_5xx"
	}
	return providerClassForHTTPStatus(statusCode)
}

func providerClassForAnthropicError(errorType string, statusCode int) string {
	switch strings.ToLower(strings.TrimSpace(errorType)) {
	case "authentication_error", "permission_error":
		return "auth_failed"
	case "rate_limit_error":
		return "rate_limit"
	case "invalid_request_error":
		return "invalid_request"
	case "not_found_error":
		return "model_unavailable"
	case "overloaded_error", "api_error":
		return "provider_5xx"
	}
	return providerClassForHTTPStatus(statusCode)
}

func providerClassForHTTPStatus(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "auth_failed"
	case http.StatusNotFound:
		return "model_unavailable"
	case http.StatusTooManyRequests:
		return "rate_limit"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "timeout"
	default:
		if statusCode >= 500 {
			return "provider_5xx"
		}
	}
	return "unknown"
}

func providerErrorFromReverseProxy(err error) error {
	var runtimeErr reverseproxycontract.RuntimeError
	if errors.As(err, &runtimeErr) {
		statusCode := runtimeErr.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		class := strings.TrimSpace(runtimeErr.Class)
		if class == "" {
			class = "unknown"
		}
		if class == "upstream_error" {
			class = providerClassForHTTPStatus(statusCode)
		}
		message := strings.TrimSpace(runtimeErr.Message)
		if message == "" {
			message = runtimeErr.Error()
		}
		return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
	}
	return contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy request failed"}
}

func synthesizeLocalText(model, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "SRapi local response for " + model
	}
	return "SRapi local response for " + model + ": " + prompt
}

func synthesizeLocalEmbeddings(req contract.EmbeddingRequest) contract.EmbeddingResponse {
	data := make([]contract.Embedding, 0, len(req.Input))
	for idx, value := range req.Input {
		vector := deterministicEmbeddingVector(value, idx)
		data = append(data, contract.Embedding{Index: idx, Vector: vector})
	}
	return contract.EmbeddingResponse{
		Data:       data,
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: http.StatusOK,
		Usage:      estimatedEmbeddingUsage(req.Input),
	}
}

func synthesizeLocalImages(req contract.ImageGenerationRequest) contract.ImageGenerationResponse {
	count := req.Count
	if count <= 0 {
		count = 1
	}
	data := make([]contract.Image, 0, count)
	for idx := 0; idx < count; idx++ {
		data = append(data, contract.Image{
			URL:           fmt.Sprintf("srapi://local-image/%s/%d", url.PathEscape(req.Mapping.UpstreamModelName), idx),
			RevisedPrompt: strings.TrimSpace(req.Prompt),
		})
	}
	return contract.ImageGenerationResponse{
		Created:    time.Now().Unix(),
		Data:       data,
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: http.StatusOK,
		Usage:      estimatedImageUsage(req),
	}
}

func deterministicEmbeddingVector(value string, index int) []float32 {
	total := 0
	for _, r := range value {
		total += int(r)
	}
	base := float32((total % 997) + index + 1)
	return []float32{base / 997, float32(len(value)+1) / 997, float32(index+1) / 997}
}

func estimatedEmbeddingUsage(input []string) contract.Usage {
	return contract.Usage{
		InputTokens: estimateTokens(strings.Join(input, "\n")),
		Estimated:   true,
	}
}

func estimatedImageUsage(req contract.ImageGenerationRequest) contract.Usage {
	output := req.Count
	if output <= 0 {
		output = 1
	}
	return contract.Usage{
		InputTokens:  estimateTokens(req.Prompt),
		OutputTokens: output,
		Estimated:    true,
	}
}

func estimatedUsage(text string) contract.Usage {
	total := estimateTokens(text)
	input := total / 2
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: total - input,
		CachedTokens: 0,
		Estimated:    true,
	}
}

func estimateTokens(text string) int {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		if text == "" {
			return 1
		}
		return max(1, len(text)/4)
	}
	return max(1, len(fields)*2)
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func uniqueTrimmedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
