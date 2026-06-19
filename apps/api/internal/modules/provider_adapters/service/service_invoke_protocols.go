package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

// transportErrorPersistentMetadataKey marks a ProviderError whose transport-level
// failure is durable (proxy/DNS/routing/credential fault) rather than a transient
// blip. The class stays "network_error" (which the gateway's cooldown path already
// treats as cooldown-eligible), so a persistent marker tells that path to park the
// account instead of repeatedly scheduling it into the same hard failure. Transient
// transport errors carry no marker and remain freely reschedulable.
//
// Ported from sub2api's classifyOpenAITransportError
// (internal/service/openai_upstream_transport_error.go): the typed-error checks
// (syscall ECONNREFUSED/EHOSTUNREACH/ENETUNREACH and *net.DNSError.IsNotFound) are
// preferred and portable; the string-marker list is a cross-platform safety net for
// errors with no typed form (e.g. SOCKS5 RFC1929 credential rejection).
const transportErrorPersistentMetadataKey = "transport_error_persistent"

// persistentTransportErrorMarkers are substrings (matched case-insensitively
// against the raw transport error) that indicate a durable proxy/network fault.
// Matched signals are intentionally specific failure *reasons*, not the operation,
// so that a transient failure of the same operation (e.g. a proxy timeout) is NOT
// misclassified as durable. Mirrors sub2api openAIPersistentTransportErrorMarkers.
var persistentTransportErrorMarkers = []string{
	"authentication failed",         // SOCKS5 RFC1929 / proxy credentials rejected (expired account)
	"proxy authentication required", // HTTP proxy 407
	"connection refused",            // proxy/upstream endpoint down
	"no route to host",
	"network is unreachable",
	"no such host", // DNS resolution failure (bad/expired proxy hostname)
}

// transportErrorIsPersistent decides whether a transport-level upstream error is
// durable (retrying the same proxy/account is pointless) or a transient blip.
// Typed checks are tried first (portable, unambiguous), then a string-marker
// fallback. Mirrors sub2api classifyOpenAITransportError.
func transportErrorIsPersistent(err error) bool {
	if err == nil {
		return false
	}
	// Typed checks (preferred).
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}
	// String-marker fallback for errors with no typed form.
	msg := strings.ToLower(err.Error())
	for _, marker := range persistentTransportErrorMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// classifyTransportError converts a transport-level upstream failure (the HTTP
// round-trip never completed: proxy/DNS/TCP/TLS error, no status code received)
// into a ProviderError. The class is always "network_error" so the gateway's
// existing cooldown path applies; persistent faults additionally carry a distinct
// metadata marker so that path parks the account rather than rescheduling it into
// the same hard failure. Mirrors sub2api's transport-error classification.
func classifyTransportError(err error) contract.ProviderError {
	provErr := contract.ProviderError{
		Class:      "network_error",
		StatusCode: http.StatusBadGateway,
		Message:    "provider request failed",
	}
	if transportErrorIsPersistent(err) {
		provErr.Metadata = map[string]any{transportErrorPersistentMetadataKey: true}
	}
	return provErr
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
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.EmbeddingResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.EmbeddingResponse{}, classifyTransportError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.EmbeddingResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	parsed, err := parseOpenAICompatibleEmbeddings(body, resp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
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
			Metadata:       req.Account.Metadata,
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
		return contract.EmbeddingResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAICompatibleEmbeddings(runtimeResp.Body, runtimeResp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
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
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ImageGenerationResponse{}, classifyTransportError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	parsed, err := parseOpenAICompatibleImages(body, resp.StatusCode, req.Mapping.UpstreamModelName, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
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
			Metadata:       req.Account.Metadata,
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
		return contract.ImageGenerationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAICompatibleImages(runtimeResp.Body, runtimeResp.StatusCode, req.Mapping.UpstreamModelName, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
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
	send := func(body []byte) ([]byte, int, http.Header, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, 0, nil, err
		}
		httpReq.Header = headers
		s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

		resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, 0, nil, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
			}
			return nil, 0, nil, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			if req.Stream {
				return nil, resp.StatusCode, resp.Header, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
			}
			return nil, resp.StatusCode, resp.Header, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
		}
		return respBody, resp.StatusCode, resp.Header, nil
	}
	body, statusCode, respHeaders, err := send(raw)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	signatureDowngraded := false
	if statusCode < 200 || statusCode >= 300 {
		if retryReq, ok := geminiSignatureRetryRequest(req, statusCode, body, geminiSignatureRetryThinking); ok {
			if retryRaw, err := geminiCompatibleRequestBody(retryReq); err != nil {
				return contract.ConversationResponse{}, err
			} else if retryBody, retryStatusCode, retryHeaders, err := send(retryRaw); err != nil {
				return contract.ConversationResponse{}, err
			} else {
				signatureDowngraded = true
				body = retryBody
				statusCode = retryStatusCode
				respHeaders = retryHeaders
			}
		}
		if statusCode < 200 || statusCode >= 300 {
			if retryReq, ok := geminiSignatureRetryRequest(req, statusCode, body, geminiSignatureRetryTools); ok {
				if retryRaw, err := geminiCompatibleRequestBody(retryReq); err != nil {
					return contract.ConversationResponse{}, err
				} else if retryBody, retryStatusCode, retryHeaders, err := send(retryRaw); err != nil {
					return contract.ConversationResponse{}, err
				} else {
					signatureDowngraded = true
					body = retryBody
					statusCode = retryStatusCode
					respHeaders = retryHeaders
				}
			}
		}
	}
	if statusCode < 200 || statusCode >= 300 {
		return contract.ConversationResponse{}, classifyGeminiProviderHTTPErrorWithHeaders(statusCode, respHeaders, body)
	}
	if req.Stream {
		parsed, err := parseGeminiCompatibleStream(body, statusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		parsed = withConversationResponseHeaders(parsed, respHeaders)
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
	parsed = withConversationResponseHeaders(parsed, respHeaders)
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
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
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
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	if req.Stream {
		parsed, err := parseAnthropicCompatibleStream(body, resp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		parsed = withConversationResponseHeaders(parsed, resp.Header)
		return withAnthropicQuotaSignals(parsed, resp.Header), nil
	}

	parsed, err := parseAnthropicCompatibleJSON(body, resp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	parsed = withConversationResponseHeaders(parsed, resp.Header)
	return withAnthropicQuotaSignals(parsed, resp.Header), nil
}

func (s *Service) invokeOpenAICompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	if openAIResponsesCompactRequest(req) {
		return s.invokeOpenAICompatibleResponsesCompact(ctx, req, baseURL, apiKey)
	}
	if openAIResponsesRequest(req) {
		return s.invokeOpenAICompatibleResponses(ctx, req, baseURL, apiKey)
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
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
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
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	if req.Stream {
		parsed, err := parseOpenAICompatibleStream(body, resp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		return withConversationResponseHeaders(parsed, resp.Header), nil
	}

	parsed, err := parseOpenAICompatibleJSON(body, resp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, resp.Header), nil
}

func (s *Service) invokeOpenAICompatibleResponses(ctx context.Context, req contract.ConversationRequest, baseURL string, apiKey string) (contract.ConversationResponse, error) {
	raw, err := openAIResponsesBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/responses", bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header.Set("Accept", openAIResponsesAccept(req.Stream))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
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
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	parsed, err := parseOpenAIResponsesBodyWithOptions(body, resp.StatusCode, openAIResponsesRequireTerminalEvent(req))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, resp.Header), nil
}

func (s *Service) invokeOpenAIResponseInputItems(ctx context.Context, req contract.ResponseInputItemsRequest, baseURL string) (contract.ResponseInputItemsResponse, error) {
	apiKey := responseInputItemsAPIKey(req)
	if apiKey == "" {
		return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, responseInputItemsEndpoint(baseURL, req.ResponseID, req.Query), nil)
	if err != nil {
		return contract.ResponseInputItemsResponse{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ResponseInputItemsResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	return contract.ResponseInputItemsResponse{
		Raw:        append([]byte(nil), bytes.TrimSpace(body)...),
		StatusCode: resp.StatusCode,
		Headers:    cloneGenericHeaders(resp.Header),
	}, nil
}

func (s *Service) invokeReverseProxyOpenAIResponseInputItems(ctx context.Context, req contract.ResponseInputItemsRequest, baseURL string) (contract.ResponseInputItemsResponse, error) {
	if s.reverseProxy == nil {
		return contract.ResponseInputItemsResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: responseInputItemsReverseProxyAccount(req),
		Method:  http.MethodGet,
		URL:     responseInputItemsEndpoint(baseURL, req.ResponseID, req.Query),
		Headers: http.Header{
			"Accept": {"application/json"},
		},
	})
	if err != nil {
		return contract.ResponseInputItemsResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ResponseInputItemsResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	return contract.ResponseInputItemsResponse{
		Raw:        append([]byte(nil), bytes.TrimSpace(runtimeResp.Body)...),
		StatusCode: runtimeResp.StatusCode,
		Headers:    cloneGenericHeaders(runtimeResp.Headers),
	}, nil
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
				Metadata:       req.Account.Metadata,
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
		return contract.ConversationResponse{}, classifyGeminiProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	if req.Stream {
		parsed, err := parseGeminiCompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		parsed = withConversationResponseHeaders(parsed, runtimeResp.Headers)
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
	parsed = withConversationResponseHeaders(parsed, runtimeResp.Headers)
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
			Metadata:       req.Account.Metadata,
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
		parsed, err := parseAnthropicCompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		parsed = withConversationResponseHeaders(parsed, runtimeResp.Headers)
		return withAnthropicQuotaSignals(parsed, runtimeResp.Headers), nil
	}
	parsed, err := parseAnthropicCompatibleJSON(runtimeResp.Body, runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	parsed = withConversationResponseHeaders(parsed, runtimeResp.Headers)
	return withAnthropicQuotaSignals(parsed, runtimeResp.Headers), nil
}

func (s *Service) invokeReverseProxyOpenAICompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if openAIResponsesCompactRequest(req) {
		return s.invokeReverseProxyOpenAICompatibleResponsesCompact(ctx, req, baseURL)
	}
	if isChatGPTWebReverseProxy(req) {
		return s.invokeReverseProxyChatGPTWebConversation(ctx, req, baseURL)
	}
	if openAIResponsesRequest(req) {
		return s.invokeReverseProxyOpenAICompatibleResponses(ctx, req, baseURL)
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
			Metadata:       req.Account.Metadata,
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
		parsed, err := parseOpenAICompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
	}
	parsed, err := parseOpenAICompatibleJSON(runtimeResp.Body, runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
}

func (s *Service) invokeOpenAICompatibleResponsesCompact(ctx context.Context, req contract.ConversationRequest, baseURL string, apiKey string) (contract.ConversationResponse, error) {
	raw, err := openAIResponsesCompactBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/responses/compact", bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	parsed, err := parseOpenAIResponsesCompactJSON(body, resp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, resp.Header), nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleResponsesCompact(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	raw, err := openAIResponsesCompactBody(req)
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
			Metadata:       req.Account.Metadata,
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/responses/compact",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAIResponsesCompactJSON(runtimeResp.Body, runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleResponses(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	raw, err := openAIResponsesBody(req)
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
			Metadata:       req.Account.Metadata,
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/responses",
		Headers: http.Header{
			"Accept":       {openAIResponsesAccept(req.Stream)},
			"Content-Type": {"application/json"},
		},
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAIResponsesBodyWithOptions(runtimeResp.Body, runtimeResp.StatusCode, openAIResponsesRequireTerminalEvent(req))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
}

func openAIResponsesCompactRequest(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.SourceProtocol), "openai-compatible") &&
		strings.HasSuffix(strings.ToLower(strings.TrimSpace(req.SourceEndpoint)), "/responses/compact")
}

func openAIResponsesRequest(req contract.ConversationRequest) bool {
	sourceEndpoint := strings.ToLower(strings.TrimSpace(req.SourceEndpoint))
	return strings.EqualFold(strings.TrimSpace(req.SourceProtocol), "openai-compatible") &&
		strings.HasSuffix(sourceEndpoint, "/responses") &&
		!strings.HasSuffix(sourceEndpoint, "/responses/compact") &&
		openAIResponsesNativeEnabled(req)
}

func openAIResponsesNativeEnabled(req contract.ConversationRequest) bool {
	adapterType := strings.ToLower(strings.TrimSpace(req.Provider.AdapterType))
	if adapterType == "native-openai" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(req.Provider.Name), "openai") {
		return true
	}
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		if mapBool(values, "native_responses") ||
			mapBool(values, "responses_native") ||
			mapBool(values, "responses_passthrough") ||
			mapBool(values, "openai_responses_passthrough") {
			return true
		}
	}
	return false
}

func openAIResponsesRequireTerminalEvent(req contract.ConversationRequest) bool {
	if strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "native-openai") ||
		strings.EqualFold(strings.TrimSpace(req.Provider.Name), "openai") {
		return true
	}
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		if mapBool(values, "responses_require_terminal_event") ||
			mapBool(values, "openai_responses_require_terminal_event") ||
			mapBool(values, "strict_responses_stream_terminal") {
			return true
		}
	}
	return false
}

func openAIResponsesBody(req contract.ConversationRequest) ([]byte, error) {
	raw := bytes.TrimSpace(req.RawBody)
	if len(raw) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid raw responses payload"}
		}
		openAIApplyResponsesPayloadDefaults(req, payload)
		if err := normalizeOpenAIResponsesImageOnlyModel(req, payload); err != nil {
			return nil, err
		}
		normalizeOpenAIResponsesServiceTier(payload)
		normalizeOpenAIResponsesImageGenerationTools(payload)
		applyDisableImageGenerationToResponsesPayload(req, payload)
		return json.Marshal(payload)
	}
	payload := openAICanonicalResponsesPayload(req)
	openAIApplyResponsesPayloadDefaults(req, payload)
	if err := normalizeOpenAIResponsesImageOnlyModel(req, payload); err != nil {
		return nil, err
	}
	normalizeOpenAIResponsesServiceTier(payload)
	normalizeOpenAIResponsesImageGenerationTools(payload)
	applyDisableImageGenerationToResponsesPayload(req, payload)
	return json.Marshal(payload)
}

func openAICanonicalResponsesPayload(req contract.ConversationRequest) map[string]any {
	payload := map[string]any{
		"model":  req.Mapping.UpstreamModelName,
		"input":  codexResponsesInput(req),
		"stream": req.Stream,
	}
	if instructions := codexResponsesInstructions(req); instructions != "" {
		payload["instructions"] = instructions
	}
	if req.MaxOutputTokens != nil {
		payload["max_output_tokens"] = *req.MaxOutputTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
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
	return payload
}

func openAIApplyResponsesPayloadDefaults(req contract.ConversationRequest, payload map[string]any) {
	if payload == nil {
		return
	}
	if model := strings.TrimSpace(req.Mapping.UpstreamModelName); model != "" {
		payload["model"] = model
	}
	payload["stream"] = req.Stream
}

func normalizeOpenAIResponsesServiceTier(payload map[string]any) {
	if payload == nil {
		return
	}
	raw, ok := payload["service_tier"].(string)
	if !ok {
		return
	}
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		delete(payload, "service_tier")
		return
	}
	if value == "fast" {
		value = "priority"
	}
	switch value {
	case "priority", "flex", "auto", "default", "scale":
		payload["service_tier"] = value
	default:
		delete(payload, "service_tier")
	}
}

func normalizeOpenAIResponsesImageGenerationTools(payload map[string]any) {
	rawTools, ok := payload["tools"]
	if !ok || rawTools == nil {
		return
	}
	switch tools := rawTools.(type) {
	case []any:
		for _, rawTool := range tools {
			if tool, ok := rawTool.(map[string]any); ok {
				normalizeOpenAIResponsesImageGenerationTool(tool)
			}
		}
	case []map[string]any:
		for _, tool := range tools {
			normalizeOpenAIResponsesImageGenerationTool(tool)
		}
	}
}

func normalizeOpenAIResponsesImageGenerationTool(tool map[string]any) {
	if strings.TrimSpace(mapString(tool, "type")) != "image_generation" {
		return
	}
	if _, exists := tool["output_format"]; !exists {
		if value := strings.TrimSpace(mapString(tool, "format")); value != "" {
			tool["output_format"] = value
		}
	}
	if _, exists := tool["output_compression"]; !exists {
		if value, ok := tool["compression"]; ok && value != nil {
			tool["output_compression"] = cloneAny(value)
		}
	}
	delete(tool, "format")
	delete(tool, "compression")
}

func normalizeOpenAIResponsesImageOnlyModel(req contract.ConversationRequest, payload map[string]any) error {
	imageModel := strings.TrimSpace(mapString(payload, "model"))
	if !openAIResponsesImageGenerationModel(imageModel) {
		return nil
	}
	mainModel := openAIResponsesMainModel(req)
	if mainModel == "" {
		return contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "responses image_generation requests require a configured responses main model"}
	}
	tool := ensureOpenAIResponsesImageGenerationTool(payload)
	if strings.TrimSpace(mapString(tool, "model")) == "" {
		tool["model"] = imageModel
	}
	for _, key := range []string{"size", "quality", "background", "output_format", "output_compression", "moderation", "style", "partial_images"} {
		moveOpenAIResponsesImageToolField(payload, tool, key, key)
	}
	moveOpenAIResponsesImageToolField(payload, tool, "format", "output_format")
	moveOpenAIResponsesImageToolField(payload, tool, "compression", "output_compression")
	if prompt := strings.TrimSpace(mapString(payload, "prompt")); prompt != "" {
		if _, exists := payload["input"]; !exists {
			payload["input"] = prompt
		}
		delete(payload, "prompt")
	}
	if _, exists := payload["tool_choice"]; !exists {
		payload["tool_choice"] = map[string]any{"type": "image_generation"}
	}
	payload["model"] = mainModel
	return nil
}

func moveOpenAIResponsesImageToolField(payload map[string]any, tool map[string]any, fromKey string, toKey string) {
	value, ok := payload[fromKey]
	if !ok || value == nil {
		return
	}
	if _, exists := tool[toKey]; !exists {
		tool[toKey] = cloneAny(value)
	}
	delete(payload, fromKey)
}

func ensureOpenAIResponsesImageGenerationTool(payload map[string]any) map[string]any {
	switch tools := payload["tools"].(type) {
	case []any:
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if ok && strings.TrimSpace(mapString(tool, "type")) == "image_generation" {
				return tool
			}
		}
		tool := map[string]any{"type": "image_generation"}
		payload["tools"] = append(tools, tool)
		return tool
	case []map[string]any:
		for _, tool := range tools {
			if strings.TrimSpace(mapString(tool, "type")) == "image_generation" {
				return tool
			}
		}
		tool := map[string]any{"type": "image_generation"}
		payload["tools"] = append(tools, tool)
		return tool
	default:
		tool := map[string]any{"type": "image_generation"}
		payload["tools"] = []any{tool}
		return tool
	}
}

func openAIResponsesImageGenerationModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-")
}

func openAIResponsesMainModel(req contract.ConversationRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"responses_main_model", "openai_responses_main_model", "image_generation_responses_model"} {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func openAIResponsesAccept(stream bool) string {
	if stream {
		return "text/event-stream"
	}
	return "application/json"
}

func parseOpenAIResponsesBody(body []byte, statusCode int) (contract.ConversationResponse, error) {
	return parseOpenAIResponsesBodyWithOptions(body, statusCode, false)
}

func parseOpenAIResponsesBodyWithOptions(body []byte, statusCode int, requireTerminalEvent bool) (contract.ConversationResponse, error) {
	return parseCodexResponsesBodyWithOptions(body, statusCode, codexResponsesParseOptions{RequireTerminalEvent: requireTerminalEvent})
}

func openAIResponsesCompactBody(req contract.ConversationRequest) ([]byte, error) {
	raw := bytes.TrimSpace(req.RawBody)
	if len(raw) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid raw responses compact payload"}
		}
		if model := strings.TrimSpace(req.Mapping.UpstreamModelName); model != "" {
			payload["model"] = model
		}
		normalizeOpenAIResponsesServiceTier(payload)
		return json.Marshal(payload)
	}
	prompt := strings.TrimSpace(conversationPrompt(req))
	if prompt == "" {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "responses compact request body missing"}
	}
	return json.Marshal(map[string]any{
		"model": req.Mapping.UpstreamModelName,
		"input": prompt,
	})
}

type openAIResponsesCompactResponse struct {
	Object       string      `json:"object"`
	InputTokens  *int        `json:"input_tokens"`
	OutputTokens *int        `json:"output_tokens"`
	Usage        openAIUsage `json:"usage"`
}

func parseOpenAIResponsesCompactJSON(body []byte, statusCode int) (contract.ConversationResponse, error) {
	var decoded openAIResponsesCompactResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	if !strings.EqualFold(strings.TrimSpace(decoded.Object), "response.compaction") {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned non-compact response"}
	}
	usage := decoded.Usage
	if usage.InputTokens == nil && decoded.InputTokens != nil {
		usage.InputTokens = decoded.InputTokens
	}
	if usage.OutputTokens == nil && decoded.OutputTokens != nil {
		usage.OutputTokens = decoded.OutputTokens
	}
	respUsage := contract.Usage{}
	if usage.HasTokenUsage() {
		respUsage = usage.ToUsage("")
	}
	return contract.ConversationResponse{
		StopReason: contract.StopReasonEndTurn,
		StatusCode: statusCode,
		Usage:      respUsage,
		Raw:        append(json.RawMessage(nil), bytes.TrimSpace(body)...),
	}, nil
}
