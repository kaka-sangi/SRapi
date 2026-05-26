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

func (s *Service) InvokeConversation(ctx context.Context, req contract.ConversationRequest) (contract.ConversationResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.ConversationResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURL(req); baseURL != "" {
		if isGenericReverseProxy(req) {
			return s.invokeGenericReverseProxyText(ctx, req, baseURL)
		}
		if isCodexReverseProxy(req) {
			return s.invokeReverseProxyCodexResponses(ctx, req, baseURL)
		}
		if isAntigravityReverseProxy(req) {
			return s.invokeReverseProxyAntigravity(ctx, req, baseURL)
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
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	text := synthesizeLocalText(req.Model, conversationPrompt(req))
	return conversationTextResponse(text, http.StatusOK, estimatedUsage(text)), nil
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
	if isOpenAIRealtimeCompatible(req) {
		return s.prepareOpenAIRealtime(ctx, req, baseURL)
	}
	return contract.RealtimeSession{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "realtime reverse proxy adapter unsupported"}
}

func (s *Service) InvokeEmbeddings(ctx context.Context, req contract.EmbeddingRequest) (contract.EmbeddingResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || len(req.Input) == 0 {
		return contract.EmbeddingResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLEmbeddings(req); baseURL != "" {
		if isGenericReverseProxyEmbeddings(req) {
			return s.invokeGenericReverseProxyEmbeddings(ctx, req, baseURL)
		}
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

func (s *Service) invokeGeminiCompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := geminiAPIKey(req)
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := geminiCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := geminiEndpoint(baseURL, req.Mapping.UpstreamModelName, req.Stream)
	headers := geminiCompatibleHeaders(req, apiKey, &endpoint)
	send := func(body []byte) ([]byte, int, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, 0, err
		}
		httpReq.Header = headers

		resp, err := s.client.Do(httpReq)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, 0, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
			}
			return nil, 0, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			if req.Stream {
				return nil, resp.StatusCode, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
			}
			return nil, resp.StatusCode, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
		}
		return respBody, resp.StatusCode, nil
	}
	body, statusCode, err := send(raw)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	signatureDowngraded := false
	if statusCode < 200 || statusCode >= 300 {
		if retryReq, ok := geminiSignatureRetryRequest(req, statusCode, body, geminiSignatureRetryThinking); ok {
			if retryRaw, err := geminiCompatibleRequestBody(retryReq); err != nil {
				return contract.ConversationResponse{}, err
			} else if retryBody, retryStatusCode, err := send(retryRaw); err != nil {
				return contract.ConversationResponse{}, err
			} else {
				signatureDowngraded = true
				body = retryBody
				statusCode = retryStatusCode
			}
		}
		if statusCode < 200 || statusCode >= 300 {
			if retryReq, ok := geminiSignatureRetryRequest(req, statusCode, body, geminiSignatureRetryTools); ok {
				if retryRaw, err := geminiCompatibleRequestBody(retryReq); err != nil {
					return contract.ConversationResponse{}, err
				} else if retryBody, retryStatusCode, err := send(retryRaw); err != nil {
					return contract.ConversationResponse{}, err
				} else {
					signatureDowngraded = true
					body = retryBody
					statusCode = retryStatusCode
				}
			}
		}
	}
	if statusCode < 200 || statusCode >= 300 {
		return contract.ConversationResponse{}, classifyGeminiProviderHTTPError(statusCode, body)
	}
	if req.Stream {
		parsed, err := parseGeminiCompatibleStream(body, statusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		if signatureDowngraded {
			parsed = appendGeminiSignatureDowngradeWarning(parsed)
		}
		return parsed, nil
	}

	var decoded geminiGenerateContentResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	parsed, err := decoded.ConversationResponse(statusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	parsed.Raw = append(json.RawMessage(nil), body...)
	if signatureDowngraded {
		parsed = appendGeminiSignatureDowngradeWarning(parsed)
	}
	return parsed, nil
}

func (s *Service) invokeAnthropicCompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := anthropicAPIKey(req)
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := anthropicCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	headers := anthropicCompatibleHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header = headers

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if req.Stream {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyAnthropicProviderHTTPError(resp.StatusCode, body)
	}
	if req.Stream {
		return parseAnthropicCompatibleStream(body, resp.StatusCode)
	}

	return parseAnthropicCompatibleJSON(body, resp.StatusCode)
}

func (s *Service) invokeOpenAICompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := openAICompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if req.Stream {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPError(resp.StatusCode, body)
	}
	if req.Stream {
		return parseOpenAICompatibleStream(body, resp.StatusCode)
	}

	return parseOpenAICompatibleJSON(body, resp.StatusCode)
}

func (s *Service) invokeReverseProxyGeminiCompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	raw, err := geminiCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := geminiEndpoint(baseURL, req.Mapping.UpstreamModelName, req.Stream)
	headers := geminiCompatibleHeaders(req, "", &endpoint)
	send := func(body []byte) (reverseproxycontract.Response, error) {
		return s.reverseProxy.Do(ctx, reverseproxycontract.Request{
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
			Body:         body,
			ExpectStream: req.Stream,
		})
	}
	runtimeResp, err := send(raw)
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	signatureDowngraded := false
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		if retryReq, ok := geminiSignatureRetryRequest(req, runtimeResp.StatusCode, runtimeResp.Body, geminiSignatureRetryThinking); ok {
			if retryRaw, err := geminiCompatibleRequestBody(retryReq); err != nil {
				return contract.ConversationResponse{}, err
			} else {
				runtimeResp, err = send(retryRaw)
				if err != nil {
					return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
				}
				signatureDowngraded = true
			}
		}
		if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
			if retryReq, ok := geminiSignatureRetryRequest(req, runtimeResp.StatusCode, runtimeResp.Body, geminiSignatureRetryTools); ok {
				if retryRaw, err := geminiCompatibleRequestBody(retryReq); err != nil {
					return contract.ConversationResponse{}, err
				} else {
					runtimeResp, err = send(retryRaw)
					if err != nil {
						return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
					}
					signatureDowngraded = true
				}
			}
		}
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyGeminiProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	if req.Stream {
		parsed, err := parseGeminiCompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		if signatureDowngraded {
			parsed = appendGeminiSignatureDowngradeWarning(parsed)
		}
		return parsed, nil
	}
	var decoded geminiGenerateContentResponse
	if err := json.Unmarshal(runtimeResp.Body, &decoded); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	parsed, err := decoded.ConversationResponse(runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	parsed.Raw = append(json.RawMessage(nil), runtimeResp.Body...)
	if signatureDowngraded {
		parsed = appendGeminiSignatureDowngradeWarning(parsed)
	}
	return parsed, nil
}

func (s *Service) invokeReverseProxyAnthropicCompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if isClaudeCodeReverseProxy(req) && claudeCodeReverseProxyRuntimeIsAPIKey(req) {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "Claude Code reverse proxy requires OAuth or client-token runtime"}
	}
	raw, err := anthropicCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	headers := anthropicCompatibleHeaders(req, "", &endpoint)
	if isClaudeCodeReverseProxy(req) {
		raw, err = claudeCodeMessagesPayload(req, raw)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		endpoint = claudeCodeMessagesEndpoint(baseURL)
		headers = claudeCodeMessagesHeaders(req)
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
		Method:       http.MethodPost,
		URL:          endpoint,
		Headers:      headers,
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if req.Stream {
		return parseAnthropicCompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
	}
	return parseAnthropicCompatibleJSON(runtimeResp.Body, runtimeResp.StatusCode)
}

func (s *Service) invokeReverseProxyOpenAICompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if isChatGPTWebReverseProxy(req) {
		return s.invokeReverseProxyChatGPTWebConversation(ctx, req, baseURL)
	}
	raw, err := openAICompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
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
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if req.Stream {
		return parseOpenAICompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
	}
	return parseOpenAICompatibleJSON(runtimeResp.Body, runtimeResp.StatusCode)
}

func upstreamBaseURL(req contract.ConversationRequest) string {
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

func isGeminiCompatible(req contract.ConversationRequest) bool {
	for _, value := range []string{req.Provider.Protocol, req.Provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "gemini-compatible", "native-gemini", "reverse-proxy-gemini-cli":
			return true
		}
	}
	return false
}

func isAnthropicCompatible(req contract.ConversationRequest) bool {
	for _, value := range []string{req.Provider.Protocol, req.Provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "anthropic-compatible", "reverse-proxy-claude-code-cli":
			return true
		}
	}
	return false
}

func isCodexReverseProxy(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-codex-cli")
}

func isCodexRealtimeReverseProxy(req contract.RealtimeRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-codex-cli")
}

func isReverseProxyRuntime(req contract.ConversationRequest) bool {
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

func geminiAPIKey(req contract.ConversationRequest) string {
	for _, key := range []string{"api_key", "gemini_api_key", "google_api_key", "access_token"} {
		if value := credentialString(req.Credential, key); value != "" {
			return value
		}
	}
	return ""
}

func geminiCompatibleHeaders(req contract.ConversationRequest, apiKey string, endpoint *string) http.Header {
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

func anthropicAPIKey(req contract.ConversationRequest) string {
	for _, key := range []string{"api_key", "x_api_key", "anthropic_api_key", "access_token"} {
		if value := credentialString(req.Credential, key); value != "" {
			return value
		}
	}
	return ""
}

func anthropicCompatibleHeaders(req contract.ConversationRequest, apiKey string, endpoint *string) http.Header {
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

func requestSetting(req contract.ConversationRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

type rawEndpointKind string

const (
	rawEndpointOpenAIChatCompletions rawEndpointKind = "openai_chat_completions"
	rawEndpointAnthropicMessages     rawEndpointKind = "anthropic_messages"
	rawEndpointGeminiGenerateContent rawEndpointKind = "gemini_generate_content"
)

func rawSameProtocolPayload(req contract.ConversationRequest, kind rawEndpointKind) (map[string]any, bool, error) {
	if !rawEndpointMatches(req, kind) {
		return nil, false, nil
	}
	raw := bytes.TrimSpace(req.RawBody)
	if len(raw) == 0 {
		return nil, false, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, true, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid raw conversation payload"}
	}
	if len(payload) == 0 {
		return nil, false, nil
	}
	switch kind {
	case rawEndpointOpenAIChatCompletions, rawEndpointAnthropicMessages:
		if model := strings.TrimSpace(req.Mapping.UpstreamModelName); model != "" {
			payload["model"] = model
		}
		payload["stream"] = req.Stream
	}
	return payload, true, nil
}

func rawEndpointMatches(req contract.ConversationRequest, kind rawEndpointKind) bool {
	sourceEndpoint := strings.ToLower(strings.TrimSpace(req.SourceEndpoint))
	sourceProtocol := strings.ToLower(strings.TrimSpace(req.SourceProtocol))
	targetProtocol := strings.ToLower(strings.TrimSpace(req.TargetProtocol))
	if targetProtocol == "" {
		targetProtocol = strings.ToLower(strings.TrimSpace(req.Provider.Protocol))
	}
	switch kind {
	case rawEndpointOpenAIChatCompletions:
		return sourceProtocol == "openai-compatible" &&
			targetProtocol == "openai-compatible" &&
			strings.HasSuffix(sourceEndpoint, "/chat/completions")
	case rawEndpointAnthropicMessages:
		return sourceProtocol == "anthropic-compatible" &&
			targetProtocol == "anthropic-compatible" &&
			strings.HasSuffix(sourceEndpoint, "/messages")
	case rawEndpointGeminiGenerateContent:
		return sourceProtocol == "gemini-compatible" &&
			targetProtocol == "gemini-compatible" &&
			(strings.Contains(sourceEndpoint, ":generatecontent") || strings.Contains(sourceEndpoint, ":streamgeneratecontent"))
	default:
		return false
	}
}

func ensureOpenAIStreamOptions(payload map[string]any) {
	streamOptions, _ := payload["stream_options"].(map[string]any)
	if streamOptions == nil {
		streamOptions = map[string]any{}
		payload["stream_options"] = streamOptions
	}
	streamOptions["include_usage"] = true
}

func conversationTextResponse(text string, statusCode int, usage contract.Usage) contract.ConversationResponse {
	return contract.ConversationResponse{
		Parts:      []contract.ContentPart{textContentPart(text)},
		StopReason: contract.StopReasonEndTurn,
		StatusCode: statusCode,
		Usage:      usage,
	}
}

func textContentPart(text string) contract.ContentPart {
	return contract.ContentPart{
		Kind: contract.ContentPartText,
		Text: strings.TrimSpace(text),
	}
}

func textContentDelta(text string) contract.ContentPart {
	return contract.ContentPart{
		Kind: contract.ContentPartText,
		Text: text,
	}
}

func conversationText(resp contract.ConversationResponse) string {
	return contentPartsText(resp.Parts)
}

func conversationPrompt(req contract.ConversationRequest) string {
	parts := make([]string, 0, len(req.Messages)+len(req.InputParts)+1)
	for _, message := range req.Messages {
		if text := conversationMessageText(message); text != "" {
			parts = append(parts, text)
		}
	}
	if text := contentPartsText(req.InputParts); text != "" {
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		if text := contentPartsText(req.System); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(uniqueTrimmedStrings(parts), "\n")
}

func conversationMessageText(message contract.ConversationMessage) string {
	return contentPartsText(message.Parts)
}

func contentPartsText(parts []contract.ContentPart) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Kind {
		case "", contract.ContentPartText, contract.ContentPartThinking, contract.ContentPartRefusal:
			if text := strings.TrimSpace(part.Text); text != "" {
				values = append(values, text)
			}
		case contract.ContentPartImage:
			if text := strings.TrimSpace(part.Text); text != "" {
				values = append(values, text)
			}
		case contract.ContentPartToolResult:
			if text := strings.TrimSpace(part.Text); text != "" {
				values = append(values, text)
			}
		}
	}
	return strings.Join(values, "\n")
}

func openAIContentParts(content any) []contract.ContentPart {
	switch value := content.(type) {
	case nil:
		return nil
	case string:
		if text := strings.TrimSpace(value); text != "" {
			return []contract.ContentPart{textContentPart(text)}
		}
	case []any:
		parts := make([]contract.ContentPart, 0, len(value))
		for _, item := range value {
			part, ok := openAIContentPart(item)
			if ok {
				parts = append(parts, part)
			}
		}
		return parts
	case []map[string]any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return openAIContentParts(items)
	}
	return nil
}

func openAIContentPart(value any) (contract.ContentPart, bool) {
	item, ok := value.(map[string]any)
	if !ok {
		return contract.ContentPart{}, false
	}
	blockType := strings.ToLower(mapString(item, "type"))
	switch blockType {
	case "", "text", "input_text", "output_text":
		text := mapString(item, "text")
		if text == "" {
			return contract.ContentPart{}, false
		}
		return textContentPart(text), true
	case "image_url":
		imageURL, _ := item["image_url"].(map[string]any)
		url := mapString(imageURL, "url")
		if url == "" {
			url = mapString(item, "url")
		}
		if url == "" {
			return contract.ContentPart{}, false
		}
		return contract.ContentPart{Kind: contract.ContentPartImage, MediaURL: url}, true
	case "input_audio":
		inputAudio, _ := item["input_audio"].(map[string]any)
		data := mapString(inputAudio, "data")
		if data == "" {
			return contract.ContentPart{}, false
		}
		mimeType := mapString(inputAudio, "format")
		if mimeType != "" && !strings.Contains(mimeType, "/") {
			mimeType = "audio/" + mimeType
		}
		return contract.ContentPart{Kind: contract.ContentPartAudio, MediaBase64: data, MIMEType: mimeType}, true
	case "file":
		file, _ := item["file"].(map[string]any)
		fileID := mapString(file, "file_id")
		fileData := mapString(file, "file_data")
		if fileID == "" && fileData == "" {
			return contract.ContentPart{}, false
		}
		return contract.ContentPart{Kind: contract.ContentPartFile, FileID: fileID, MediaURL: fileData}, true
	}
	return contract.ContentPart{}, false
}

func jsonObjectValue(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func setMapString(values map[string]any, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values[key] = value
}

func metadataString(values map[string]any, key string) string {
	return mapString(values, key)
}

func mediaURLValue(part contract.ContentPart) string {
	if url := strings.TrimSpace(part.MediaURL); url != "" {
		return url
	}
	if data := strings.TrimSpace(part.MediaBase64); data != "" {
		mimeType := strings.TrimSpace(part.MIMEType)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		return "data:" + mimeType + ";base64," + data
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
